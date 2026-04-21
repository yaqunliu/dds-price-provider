package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dds/dds-price-provider/internal/client"
	"github.com/dds/dds-price-provider/internal/config"
	"github.com/dds/dds-price-provider/internal/types"
)

type PricingService struct {
	cfg     *config.Config
	sub2api *client.Sub2APIClient
	litellm *client.LiteLLMLoader

	mu       sync.Mutex
	cached   *types.PricingData
	cachedAt time.Time
}

func NewPricingService(cfg *config.Config, sub *client.Sub2APIClient, lite *client.LiteLLMLoader) *PricingService {
	return &PricingService{cfg: cfg, sub2api: sub, litellm: lite}
}

func (s *PricingService) BuildPricing(ctx context.Context) (*types.PricingData, error) {
	s.mu.Lock()
	if s.cached != nil && time.Since(s.cachedAt) < time.Duration(s.cfg.Cache.TTLSeconds)*time.Second {
		data := s.cached
		s.mu.Unlock()
		return data, nil
	}
	s.mu.Unlock()

	data, err := s.buildFresh(ctx)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cached = data
	s.cachedAt = time.Now()
	s.mu.Unlock()
	return data, nil
}

func (s *PricingService) buildFresh(ctx context.Context) (*types.PricingData, error) {
	var (
		groups   []client.Group
		pricing  map[string]client.LiteLLMEntry
		gErr     error
		pErr     error
		wg       sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		groups, gErr = s.sub2api.ListGroups(ctx)
	}()
	go func() {
		defer wg.Done()
		pricing, pErr = s.litellm.LoadPricing(ctx)
	}()
	wg.Wait()

	if gErr != nil {
		return nil, fmt.Errorf("list groups: %w", gErr)
	}
	if pErr != nil {
		return nil, fmt.Errorf("load pricing: %w", pErr)
	}

	models := make([]types.ModelPrice, 0, 64)
	for _, g := range groups {
		if g.IsExclusive {
			continue
		}
		models = append(models, s.pricesForGroup(g, pricing)...)
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].GroupName != models[j].GroupName {
			return models[i].GroupName < models[j].GroupName
		}
		return models[i].ModelName < models[j].ModelName
	})

	return &types.PricingData{
		Currency:   s.cfg.Site.Currency,
		PriceUnit:  s.cfg.Site.PriceUnit,
		SiteName:   s.cfg.Site.Name,
		SiteDomain: s.cfg.Site.Domain,
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
		Models:     models,
	}, nil
}

func (s *PricingService) pricesForGroup(g client.Group, pricing map[string]client.LiteLLMEntry) []types.ModelPrice {
	factor := 1e6 * g.RateMultiplier
	decimals := s.cfg.Pricing.PriceDecimals

	out := make([]types.ModelPrice, 0, 32)
	for name, entry := range pricing {
		if !platformMatches(g.Platform, name, entry) {
			continue
		}
		if entry.InputCostPerToken == 0 && entry.OutputCostPerToken == 0 {
			continue
		}
		out = append(out, types.ModelPrice{
			ModelName:       normalizeModelName(name),
			GroupName:       g.Name,
			InputPrice:      round(entry.InputCostPerToken*factor, decimals),
			OutputPrice:     round(entry.OutputCostPerToken*factor, decimals),
			CacheInputPrice: round(entry.CacheReadInputTokenCost*factor, decimals),
			Enabled:         true,
			Note:            "",
		})
	}
	return out
}

func platformMatches(platform, modelName string, entry client.LiteLLMEntry) bool {
	p := strings.ToLower(platform)
	name := strings.ToLower(modelName)
	provider := strings.ToLower(entry.LiteLLMProvider)

	switch p {
	case "anthropic":
		return provider == "anthropic" || strings.HasPrefix(name, "claude") || strings.Contains(name, "/claude")
	case "openai":
		return provider == "openai" || strings.HasPrefix(name, "gpt-") || strings.HasPrefix(name, "o1") || strings.HasPrefix(name, "o3") || strings.HasPrefix(name, "o4")
	case "gemini":
		return provider == "gemini" || provider == "vertex_ai-language-models" || strings.Contains(name, "gemini")
	case "antigravity":
		return strings.Contains(name, "gemini") || strings.HasPrefix(name, "claude")
	case "sora":
		return strings.Contains(name, "sora") || strings.HasPrefix(name, "gpt-image")
	}
	return false
}

func normalizeModelName(name string) string {
	return strings.TrimPrefix(name, "models/")
}

func round(val float64, decimals int) float64 {
	if decimals < 0 {
		decimals = 0
	}
	p := math.Pow(10, float64(decimals))
	return math.Round(val*p) / p
}
