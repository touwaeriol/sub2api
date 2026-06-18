package handler

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type monitorGroupFinder interface {
	ListActiveByPlatform(ctx context.Context, platform string) ([]service.Group, error)
}

type monitorAccountLister interface {
	ListAccounts(ctx context.Context, page, pageSize int, platform, accountType, status, search string, groupID int64, privacyMode, sortBy, sortOrder string) ([]service.Account, int64, error)
}

type monitorUsageStats interface {
	GetTodayStatsBatch(ctx context.Context, accountIDs []int64) (map[int64]*service.WindowStats, error)
	GetStatsBatch(ctx context.Context, accountIDs []int64, startTime time.Time) (map[int64]*service.WindowStats, error)
}

type MonitorHandler struct {
	groupFinder   monitorGroupFinder
	accountLister monitorAccountLister
	usageStats    monitorUsageStats
}

func NewMonitorHandler(
	groupRepo service.GroupRepository,
	adminService service.AdminService,
	accountUsageService *service.AccountUsageService,
) *MonitorHandler {
	return &MonitorHandler{
		groupFinder:   groupRepo,
		accountLister: adminService,
		usageStats:    accountUsageService,
	}
}

type monitorAccountItem struct {
	Name              string  `json:"name"`
	Platform          string  `json:"platform"`
	Type              string  `json:"type"`
	Concurrency       int     `json:"concurrency"`
	Status            string  `json:"status"`
	Schedulable       bool    `json:"schedulable"`
	TotalMarkedCost   float64 `json:"total_marked_cost"`
	TodayStandardCost float64 `json:"today_standard_cost"`
}

type monitorResponse struct {
	GroupName string               `json:"group_name"`
	Platform  string               `json:"platform"`
	Accounts  []monitorAccountItem `json:"accounts"`
	UpdatedAt time.Time            `json:"updated_at"`
}

// GetGroupAccountMonitor GET /api/v1/monitor/group?platform=xxx&group_name=xxx
func (h *MonitorHandler) GetGroupAccountMonitor(c *gin.Context) {
	platform := strings.TrimSpace(c.Query("platform"))
	groupName := strings.TrimSpace(c.Query("group_name"))
	if platform == "" || groupName == "" {
		response.BadRequest(c, "platform and group_name are required")
		return
	}

	ctx := c.Request.Context()

	group, err := h.findGroup(ctx, platform, groupName)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if group == nil {
		response.NotFound(c, "group not found")
		return
	}

	const maxPageSize = 500
	accounts, _, err := h.accountLister.ListAccounts(ctx, 1, maxPageSize, "", "", "", "", group.ID, "", "name", "asc")
	if err != nil {
		// 与同函数 line76（findGroup）一致：用 ErrorFrom 透传 typed 错误码，
		// 不丢 err、不把上游/校验类错误硬编码成 500。
		response.ErrorFrom(c, err)
		return
	}

	accountIDs := make([]int64, len(accounts))
	for i, a := range accounts {
		accountIDs[i] = a.ID
	}

	todayStats, err := h.usageStats.GetTodayStatsBatch(ctx, accountIDs)
	if err != nil {
		todayStats = make(map[int64]*service.WindowStats)
	}

	allTimeStats, err := h.usageStats.GetStatsBatch(ctx, accountIDs, time.Time{})
	if err != nil {
		allTimeStats = make(map[int64]*service.WindowStats)
	}

	items := make([]monitorAccountItem, 0, len(accounts))
	for _, a := range accounts {
		item := monitorAccountItem{
			Name:        a.Name,
			Platform:    a.Platform,
			Type:        a.Type,
			Concurrency: a.Concurrency,
			Status:      a.Status,
			Schedulable: a.Schedulable,
		}
		if ts := todayStats[a.ID]; ts != nil {
			item.TodayStandardCost = ts.StandardCost
		}
		if as := allTimeStats[a.ID]; as != nil {
			item.TotalMarkedCost = as.Cost
		}
		items = append(items, item)
	}

	response.Success(c, monitorResponse{
		GroupName: group.Name,
		Platform:  group.Platform,
		Accounts:  items,
		UpdatedAt: time.Now(),
	})
}

func (h *MonitorHandler) findGroup(ctx context.Context, platform, name string) (*service.Group, error) {
	groups, err := h.groupFinder.ListActiveByPlatform(ctx, platform)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if strings.EqualFold(groups[i].Name, name) {
			return &groups[i], nil
		}
	}
	return nil, nil
}
