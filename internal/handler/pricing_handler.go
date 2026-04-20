package handler

import (
	"net/http"

	"github.com/dds/dds-price-provider/internal/service"
	"github.com/dds/dds-price-provider/internal/types"
	"github.com/gin-gonic/gin"
)

type PricingHandler struct {
	svc *service.PricingService
}

func NewPricingHandler(svc *service.PricingService) *PricingHandler {
	return &PricingHandler{svc: svc}
}

func (h *PricingHandler) GetPricing(c *gin.Context) {
	data, err := h.svc.BuildPricing(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.PricingResponse{
			SchemaVersion: "1.0",
			Success:       false,
			Message:       err.Error(),
			Data:          nil,
		})
		return
	}
	c.JSON(http.StatusOK, types.PricingResponse{
		SchemaVersion: "1.0",
		Success:       true,
		Message:       "",
		Data:          data,
	})
}
