package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ProtocolHandler struct {
	protocolService *service.ProtocolService
}

func NewProtocolHandler(protocolService *service.ProtocolService) *ProtocolHandler {
	return &ProtocolHandler{protocolService: protocolService}
}

func (h *ProtocolHandler) List(c *gin.Context) {
	platform := c.Query("platform")

	protocols, err := h.protocolService.List(c.Request.Context(), platform)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.Protocol, 0, len(protocols))
	for _, p := range protocols {
		out = append(out, *dto.ProtocolFromService(p))
	}
	response.Success(c, out)
}
