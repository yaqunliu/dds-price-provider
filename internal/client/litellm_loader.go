package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type LiteLLMEntry struct {
	InputCostPerToken           float64 `json:"input_cost_per_token"`
	OutputCostPerToken          float64 `json:"output_cost_per_token"`
	CacheReadInputTokenCost     float64 `json:"cache_read_input_token_cost"`
	CacheCreationInputTokenCost float64 `json:"cache_creation_input_token_cost"`
	LiteLLMProvider             string  `json:"litellm_provider"`
	Mode                        string  `json:"mode"`
}

type LiteLLMLoader struct {
	remoteURL    string
	fallbackFile string
	http         *http.Client
}

func NewLiteLLMLoader(remoteURL, fallbackFile string) *LiteLLMLoader {
	return &LiteLLMLoader{
		remoteURL:    remoteURL,
		fallbackFile: fallbackFile,
		http:         &http.Client{Timeout: 15 * time.Second},
	}
}

func (l *LiteLLMLoader) LoadPricing(ctx context.Context) (map[string]LiteLLMEntry, error) {
	if l.remoteURL != "" {
		if entries, err := l.loadRemote(ctx); err == nil {
			return entries, nil
		}
	}
	if l.fallbackFile != "" {
		return l.loadLocal()
	}
	return nil, fmt.Errorf("no remote_url or fallback_file configured")
}

func (l *LiteLLMLoader) loadRemote(ctx context.Context) (map[string]LiteLLMEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.remoteURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := l.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return decodeLiteLLM(body)
}

func (l *LiteLLMLoader) loadLocal() (map[string]LiteLLMEntry, error) {
	data, err := os.ReadFile(l.fallbackFile)
	if err != nil {
		return nil, fmt.Errorf("read fallback file: %w", err)
	}
	return decodeLiteLLM(data)
}

func decodeLiteLLM(data []byte) (map[string]LiteLLMEntry, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode litellm: %w", err)
	}
	out := make(map[string]LiteLLMEntry, len(raw))
	for name, rv := range raw {
		if name == "sample_spec" {
			continue
		}
		var entry LiteLLMEntry
		if err := json.Unmarshal(rv, &entry); err != nil {
			continue
		}
		out[name] = entry
	}
	return out, nil
}
