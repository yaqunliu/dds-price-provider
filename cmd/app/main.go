package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/dds/dds-price-provider/internal/client"
	"github.com/dds/dds-price-provider/internal/config"
	"github.com/dds/dds-price-provider/internal/handler"
	"github.com/dds/dds-price-provider/internal/service"
	"github.com/gin-gonic/gin"
)

func main() {
	configPath := flag.String("c", "configs/config.yaml", "config file path")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	sub := client.NewSub2APIClient(cfg.Sub2API.BaseURL, cfg.Sub2API.AdminToken, cfg.Sub2API.TimeoutSeconds)
	lite := client.NewLiteLLMLoader(cfg.LiteLLM.RemoteURL, cfg.LiteLLM.FallbackFile)
	svc := service.NewPricingService(cfg, sub, lite)
	h := handler.NewPricingHandler(svc)

	r := gin.Default()
	r.GET("/api/provider/pricing", h.GetPricing)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("dds-price-provider listening on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("run server: %v", err)
	}
}
