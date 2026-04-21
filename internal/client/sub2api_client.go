package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Group struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	Platform       string   `json:"platform"`
	RateMultiplier float64  `json:"rate_multiplier"`
	IsExclusive    bool     `json:"is_exclusive"`
	Status         string   `json:"status"`
	Scopes         []string `json:"supported_model_scopes"`
}

type Sub2APIClient struct {
	baseURL    string
	adminToken string
	http       *http.Client
}

func NewSub2APIClient(baseURL, adminToken string, timeoutSeconds int) *Sub2APIClient {
	return &Sub2APIClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		adminToken: adminToken,
		http:       &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
	}
}

type groupsResponse struct {
	Code    int     `json:"code"`
	Message string  `json:"message"`
	Data    []Group `json:"data"`
}

func (c *Sub2APIClient) ListGroups(ctx context.Context) ([]Group, error) {
	url := c.baseURL + "/api/v1/admin/groups/all"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.adminToken != "" {
		req.Header.Set("x-api-key", c.adminToken)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call sub2api: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sub2api status %d: %s", resp.StatusCode, string(body))
	}

	var parsed groupsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode groups: %w", err)
	}

	out := make([]Group, 0, len(parsed.Data))
	for _, g := range parsed.Data {
		if strings.EqualFold(g.Status, "active") {
			out = append(out, g)
		}
	}
	return out, nil
}
