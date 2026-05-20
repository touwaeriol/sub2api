package service

import (
	"context"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/protocol"
)

type Protocol struct {
	ID              int64
	Name            string
	DisplayName     string
	Platform        string
	GatewayEndpoint string
	IconSvg         *string
	ThemeColor      string
	SortOrder       int
}

type ProtocolService struct {
	client *dbent.Client
}

func NewProtocolService(client *dbent.Client) *ProtocolService {
	return &ProtocolService{client: client}
}

func (s *ProtocolService) List(ctx context.Context, platform string) ([]*Protocol, error) {
	query := s.client.Protocol.Query().
		Where(protocol.StatusEQ(StatusActive)).
		Order(protocol.BySortOrder())

	if platform != "" {
		query = query.Where(protocol.PlatformEQ(platform))
	}

	entities, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list protocols: %w", err)
	}

	return protocolEntitiesToService(entities), nil
}

func (s *ProtocolService) GetByID(ctx context.Context, id int64) (*Protocol, error) {
	entity, err := s.client.Protocol.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get protocol: %w", err)
	}
	return protocolEntityToService(entity), nil
}

func protocolEntityToService(e *dbent.Protocol) *Protocol {
	if e == nil {
		return nil
	}
	return &Protocol{
		ID:              e.ID,
		Name:            e.Name,
		DisplayName:     e.DisplayName,
		Platform:        e.Platform,
		GatewayEndpoint: e.GatewayEndpoint,
		IconSvg:         e.IconSvg,
		ThemeColor:      e.ThemeColor,
		SortOrder:       e.SortOrder,
	}
}

func protocolEntitiesToService(entities []*dbent.Protocol) []*Protocol {
	out := make([]*Protocol, 0, len(entities))
	for _, e := range entities {
		out = append(out, protocolEntityToService(e))
	}
	return out
}
