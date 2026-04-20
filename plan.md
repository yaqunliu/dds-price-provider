# dds-price-provider: /api/provider/pricing 服务

## Context

hvoy.ai 需要抓取我们中转站的模型价格用于价格对比/核验。上级目录 `../sub2api` 是基于 GitHub LiteLLM 价格文件 + 分组 `rate_multiplier` 构建的中转站。本项目 `dds-price-provider` 是一个**极简** Go 后端，只暴露一个接口 `GET /api/provider/pricing`，按 hvoy.ai 约定的 schema（参见 `heweiData.json`）返回数据。

数据源取自 sub2api：
- 分组信息通过 `GET /api/v1/admin/groups/all`（需 Admin Token）获取，每组含 `name`、`platform`、`rate_multiplier`
- 基础价格 USD/token 从 sub2api 已缓存的 LiteLLM JSON 读取（默认远程 `https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/.../model_prices_and_context_window.json`；容灾退回 `./data/model_pricing.json`）
- 计算公式：`display_price_cny_per_1M = base_usd_price_per_token × 1e6 × rate_multiplier × fx_usd_to_cny`，其中 `fx_usd_to_cny` 可配置（默认 7.2）

## 响应格式（来自 heweiData.json）

```json
{
  "schema_version": "1.0",
  "success": true,
  "message": "",
  "data": {
    "currency": "CNY",
    "price_unit": "per_1m_tokens",
    "site_name": "hvoy",
    "site_domain": "hvoy.ai",
    "updated_at": "2026-04-20T12:00:00Z",
    "models": [
      { "model_name": "...", "group_name": "...", "input_price": 7.5,
        "output_price": 37.5, "cache_input_price": 0.75, "enabled": true, "note": "" }
    ]
  }
}
```

## 项目结构

遵循工作区 Go 项目的分层约定（精简版，仅 1 个接口）：

```
dds-price-provider/
├── go.mod                                 # module github.com/.../dds-price-provider
├── cmd/app/main.go                        # 入口（Gin 启动）
├── configs/config.yaml                    # 配置：端口、sub2api 地址、admin token、缓存 TTL、站点信息
├── internal/
│   ├── config/config.go                   # 读 yaml → struct
│   ├── handler/pricing_handler.go         # GET /api/provider/pricing
│   ├── service/pricing_service.go         # 聚合 groups + 基础价格 → 目标 schema
│   ├── client/sub2api_client.go           # 调 /api/v1/admin/groups/all
│   ├── client/litellm_loader.go           # 加载 LiteLLM 价格 JSON（远程 + 本地回退）
│   └── types/pricing.go                   # 请求/响应 DTO（schema_version 等）
└── README.md
```

## 核心实现

### 1. `client/sub2api_client.go`

- `ListGroups(ctx) ([]Group, error)`：`GET {sub2api_base}/api/v1/admin/groups/all`，请求头 `Authorization: Bearer {admin_token}`，解析 `data` 数组
- `Group` 字段：`ID, Name, Platform, RateMultiplier, Status, SupportedModelScopes, ModelRouting`
- 只保留 `status=="active"` 的分组

### 2. `client/litellm_loader.go`

- `LoadPricing(ctx) (map[string]LiteLLMEntry, error)`：
  - 首选：HTTP GET 配置的 `remote_url`（默认与 sub2api 的 `PricingConfig.RemoteURL` 一致）
  - 回退：读取本地 `fallback_file`（指向 `../sub2api/data/model_pricing.json` 或本项目 `./data/model_pricing.json`）
- `LiteLLMEntry` 至少含：`input_cost_per_token`、`output_cost_per_token`、`cache_read_input_token_cost`、`cache_creation_input_token_cost`、`litellm_provider`、`mode`

### 3. `service/pricing_service.go`

- `BuildPricing(ctx) (*PricingResponse, error)`：
  1. 并发调用 `ListGroups` + `LoadPricing`
  2. **白名单过滤**：仅保留 `group.name ∈ cfg.IncludeGroups` 且 `status=="active"` 的分组；若 `IncludeGroups` 为空则返回空列表
  3. 对每个白名单分组，按其 `platform` 过滤 LiteLLM 条目（`claude/*` → anthropic 组；`gpt-*`/`o*` → openai 组；`gemini-*` → gemini/antigravity 组；`sora-*` → sora 组）
  4. 每个匹配模型输出一条（`factor = 1e6 × group.RateMultiplier × cfg.FxUsdToCny`）：
     ```go
     {
       ModelName: normalizedName,                              // 去掉 models/ 前缀
       GroupName: group.Name,
       InputPrice:      round(entry.InputCostPerToken        * factor, 4),
       OutputPrice:     round(entry.OutputCostPerToken       * factor, 4),
       CacheInputPrice: round(entry.CacheReadInputTokenCost  * factor, 4),
       Enabled: true,
       Note: "",
     }
     ```
  5. 使用 `sync.Map` + `time.Time` 做内存缓存，TTL 可配置（默认 10 分钟），避免每次请求都打 sub2api
  6. `updated_at` = 最近一次成功构建的时间（RFC3339 UTC）

### 4. `handler/pricing_handler.go`

- `GET /api/provider/pricing` → 调 `BuildPricing` → 包装 `schema_version:"1.0"`, `success:true`, `message:""`, `data:{...}`
- 失败时返回 `success:false`, `message: err.Error()`, HTTP 500

### 5. `configs/config.yaml`

```yaml
server:
  port: 8085
sub2api:
  base_url: "http://127.0.0.1:8083"
  admin_token: "${SUB2API_ADMIN_TOKEN}"
  timeout_seconds: 10
litellm:
  remote_url: "https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/c7947e9871687e664180bc971d4837f1fc2784a9/model_prices_and_context_window.json"
  fallback_file: "../sub2api/data/model_pricing.json"
site:
  name: "hvoy"
  domain: "hvoy.ai"
  currency: "CNY"
  price_unit: "per_1m_tokens"
pricing:
  fx_usd_to_cny: 7.2     # 美元→人民币汇率
  price_decimals: 4       # 价格保留小数位
cache:
  ttl_seconds: 600
include_groups: ["cc"]    # 白名单：仅暴露列表中的 group.name；为空则全部拒绝（强制白名单）
```

支持环境变量覆盖 `SUB2API_ADMIN_TOKEN`、`SERVER_PORT`、`FX_USD_TO_CNY`。

## 关键文件（待创建）

- `/Users/user/Documents/work/code/dds-price-provider/cmd/app/main.go`
- `/Users/user/Documents/work/code/dds-price-provider/internal/handler/pricing_handler.go`
- `/Users/user/Documents/work/code/dds-price-provider/internal/service/pricing_service.go`
- `/Users/user/Documents/work/code/dds-price-provider/internal/client/sub2api_client.go`
- `/Users/user/Documents/work/code/dds-price-provider/internal/client/litellm_loader.go`
- `/Users/user/Documents/work/code/dds-price-provider/internal/config/config.go`
- `/Users/user/Documents/work/code/dds-price-provider/internal/types/pricing.go`
- `/Users/user/Documents/work/code/dds-price-provider/configs/config.yaml`
- `/Users/user/Documents/work/code/dds-price-provider/go.mod`

## 复用 sub2api 的约定

- LiteLLM JSON 格式 / URL 直接复用 sub2api 的 `PricingConfig` 默认值（`/Users/user/Documents/work/code/sub2api/backend/internal/config/config.go`）
- 分组实体字段参照 `/Users/user/Documents/work/code/sub2api/backend/ent/schema/group.go`
- 管理员接口路径参照 `/Users/user/Documents/work/code/sub2api/backend/internal/handler/admin/group_handler.go:192` 的 `GET /api/v1/admin/groups/all`

## 验证

1. 启动 sub2api 后端（上级目录），记录 admin token
2. 本项目：`go mod tidy && go run cmd/app/main.go -c configs/config.yaml`
3. `curl http://127.0.0.1:8085/api/provider/pricing | jq .` 校验：
   - `schema_version == "1.0"`、`success == true`
   - `data.currency == "CNY"`、`data.price_unit == "per_1m_tokens"`
   - `data.models[]` 每条具备 7 个必填字段
   - 手动抽查一条：`claude-sonnet-4-6` 在 `rate_multiplier=2.5` 的 `cc` 组下，若 `fx_usd_to_cny=7.2`、基础价 `$3/$15/$0.30` per 1M，应得 `input_price ≈ 54.0`、`output_price ≈ 270.0`、`cache_input_price ≈ 5.4`（若要对齐 heweiData.json 样例的 `7.5/37.5/0.75`，需把 `fx_usd_to_cny` 设为 `1.0` 或把 `rate_multiplier` 设为已含汇率的值——由部署方选择）
4. sub2api 离线时确认回退到本地 `fallback_file` 仍能返回（除非 groups 也取不到才 500）
5. 连续请求两次，观察第二次延迟明显更低，确认缓存生效
