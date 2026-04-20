# dds-price-provider 部署计划

## Context

`dds-price-provider` 是单接口 Go 服务 (`GET /api/provider/pricing`)，要部署到一台已有生产服务器。这台机器上：
- 已运行 `sub2api`、`dds-billing` 两个服务
- **Nginx 直接装在宿主机上**（非容器），统一管理同一个域名下的反代和 TLS
- 三个服务共用同一个域名，靠 nginx `location` 区分路径

目标：用最小改动把本服务挂到这台机器上，hvoy.ai 通过 HTTPS 抓 `https://<同一域名>/api/provider/pricing`。

**关键约束**：
- 服务器架构：linux/amd64
- 复用宿主机 nginx + 现有证书，不再单独搞域名/证书
- `configs/config.yaml` 含 sub2api admin token，不入库（已有 `.gitignore`）
- 修改 `include_groups` 后能一条命令立即生效

## 部署拓扑

```
hvoy.ai ──HTTPS──▶  宿主机 Nginx (:443, 系统服务)
                        │
                        │ location /api/provider/  ──▶  127.0.0.1:8085 (dds-price-provider 容器)
                        │ location /api/           ──▶  127.0.0.1:<dds-billing 端口>
                        │ location /...            ──▶  其他既有服务
                        │
                    dds-price-provider 容器
                    (network_mode: host, 监听 :8085)
                        │
                        ▼
                    sub2api (同机, HTTPS)
```

宿主机 nginx 直接 `proxy_pass http://127.0.0.1:8085`，不存在容器网络互通问题。

## 新增文件（全部在本项目）

| 文件 | 作用 |
|---|---|
| `Dockerfile` | 多阶段构建：golang:1.25-alpine → alpine:3.21，`CGO_ENABLED=0 GOOS=linux` |
| `docker-compose.yml` | 单服务 `backend`，`network_mode: host`，只读挂载 `./configs/config.yaml` 和 `./resources/`，`restart: unless-stopped` |
| `.dockerignore` | 排除 `bin/`、`*.log`、`.git/`、`configs/config.yaml` |
| `configs/config.example.yaml` | 模板文件（admin_token 留 `***REPLACE_ME***`），首次部署时 `cp` 成 `config.yaml` |
| `deploy.md` | 本文档 |

**不修改任何其他项目**。dds-billing 不动，sub2api 不动。宿主机 nginx 由运维侧追加一段 location（见下文），与本项目代码解耦。

## 核心实现

### Dockerfile

```dockerfile
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/app ./cmd/app

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/bin/app .
COPY resources /app/resources
EXPOSE 8085
CMD ["./app", "-c", "/app/configs/config.yaml"]
```

要点：
- `ca-certificates` 必须装（调用 sub2api HTTPS 和 raw.githubusercontent.com）
- `resources/model_pricing.json` 随镜像打包，作为远程拉取失败时的回退
- `configs/` 不 COPY 进镜像，完全靠挂载；镜像无机密

### docker-compose.yml

```yaml
services:
  backend:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: dds-price-provider
    restart: unless-stopped
    environment:
      TZ: Asia/Shanghai
    network_mode: host
    volumes:
      - ./configs/config.yaml:/app/configs/config.yaml:ro
      - ./resources:/app/resources:ro
```

### 宿主机 Nginx 追加片段

在现有 443 server 块（管理同一域名的那个）内追加：

```nginx
location /api/provider/ {
    proxy_pass http://127.0.0.1:8085;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

`location` 最长前缀优先：`/api/provider/` 比 `/api/` 长，会优先匹配，不会被 dds-billing 的 `/api/` 抢走。

操作：
```bash
sudo vim /etc/nginx/conf.d/<现有 conf 文件>
sudo nginx -t           # 语法检查
sudo systemctl reload nginx
```

### configs/config.example.yaml

```yaml
server:
  port: 8085

sub2api:
  base_url: "https://www.ddshub.cc"
  admin_token: "***REPLACE_ME***"
  timeout_seconds: 10

litellm:
  remote_url: "https://raw.githubusercontent.com/Wei-Shaw/claude-relay-service/price-mirror/model_prices_and_context_window.json"
  fallback_file: "./resources/model_pricing.json"

site:
  name: "hvoy"
  domain: "hvoy.ai"
  currency: "CNY"
  price_unit: "per_1m_tokens"

pricing:
  price_decimals: 4

cache:
  ttl_seconds: 600

include_groups:
  - "claude-max（仅 Claude Code）"
```

## 服务器操作手册

### 首次部署

```bash
ssh <服务器>
cd /opt
git clone <repo-url> dds-price-provider
cd dds-price-provider

# 1. 准备配置
cp configs/config.example.yaml configs/config.yaml
vim configs/config.yaml    # 填 admin_token、调整 include_groups

# 2. 启动容器
docker compose up -d --build

# 3. 本机自测
curl -s http://127.0.0.1:8085/api/provider/pricing | head

# 4. 宿主 nginx 追加 location（一次性）
sudo vim /etc/nginx/conf.d/<现有 conf>
sudo nginx -t && sudo systemctl reload nginx

# 5. 外网验证
curl -s https://<域名>/api/provider/pricing | head
```

### 日常修改 `include_groups`

```bash
cd /opt/dds-price-provider
vim configs/config.yaml
docker compose restart backend
```

### 更新代码版本

```bash
cd /opt/dds-price-provider
git pull
docker compose up -d --build
```

### 日志 / 健康

```bash
docker compose logs -f backend
docker compose ps
curl -s http://127.0.0.1:8085/api/provider/pricing | jq .success
```

## 验证清单

1. 本地 `docker build -t dds-price-provider:test .` 通过
2. 服务器 `docker compose ps` 显示 `Up`
3. `curl http://127.0.0.1:8085/api/provider/pricing` 返回 `"success": true`、`data.models` 非空
4. `curl https://<域名>/api/provider/pricing` 同样返回成功（证书+nginx 链路通）
5. 改 `include_groups` → restart → models 数量变化
6. `docker restart dds-price-provider` 或服务器重启后容器自动拉起
7. 把 `litellm.remote_url` 改错 → 重启 → 仍 200（走本地 `resources/model_pricing.json` 回退）
