package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	OpusMaxBaseURL = "https://api.opusmax.pro"
)

// OpusMaxAccountLister defines the interface for listing accounts by base URL
type OpusMaxAccountLister interface {
	ListActiveByBaseURL(ctx context.Context, baseURL string) ([]service.Account, error)
}

// OpusMaxHandler handles OpusMax-specific monitoring endpoints
type OpusMaxHandler struct {
	accountLister OpusMaxAccountLister
	httpClient    *http.Client
}

// NewOpusMaxHandler creates a new OpusMax handler
func NewOpusMaxHandler(accountLister OpusMaxAccountLister) *OpusMaxHandler {
	return &OpusMaxHandler{
		accountLister: accountLister,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// OpusMaxKeyStatus represents the actual response from OpusMax /api/key-status endpoint
type OpusMaxKeyStatus struct {
	Status         string `json:"status"`
	Name           string `json:"name"`
	KeyPrefix     string `json:"keyPrefix"`
	IsActive      bool   `json:"isActive"`
	RateLimit     int    `json:"rateLimit"`
	Unlimited     bool   `json:"unlimited"`
	UsagePercent  float64 `json:"usagePercent"`
	WindowStartedAt string `json:"windowStartedAt"`
	WindowResetAt string `json:"windowResetAt"`
	WindowActive bool   `json:"windowActive"`
	PlanName     string `json:"planName"`
	ExpiresAt    string `json:"expiresAt"`
	LastUsedAt   string `json:"lastUsedAt"`
	CreatedAt    string `json:"createdAt"`
	IsExpired    bool   `json:"isExpired"`
	IsOverLimit  bool   `json:"isOverLimit"`
	TotalRequests int    `json:"totalRequests"`
	Last24H      struct {
		Requests int `json:"requests"`
	} `json:"last24h"`
}

// OpusMaxAccount represents an account with its key status
type OpusMaxAccount struct {
	ID               int64   `json:"id"`
	Name             string  `json:"name"`
	Status           string  `json:"status"`
	Schedulable      bool    `json:"schedulable"`
	Concurrency      int     `json:"concurrency"`
	RPM              int     `json:"rpm"`
	WindowTokensLimit string  `json:"window_tokens_limit"`
	WindowTokensUsed string  `json:"window_tokens_used"`
	UsagePercent     float64 `json:"usage_percent"`
	PlanName         string  `json:"plan_name"`
	ExpiresAt        string  `json:"expires_at"`
	TotalRequests    int     `json:"total_requests"`
	Last24HRequests  int     `json:"last_24h_requests"`
}

// OpusMaxAccountsResponse is the response for the OpusMax accounts list
type OpusMaxAccountsResponse struct {
	Accounts   []OpusMaxAccount `json:"accounts"`
	UpdatedAt  time.Time        `json:"updated_at"`
	TotalCount int              `json:"total_count"`
}

// GetOpusMaxAccounts handles GET /api/v1/opusmax/accounts
// Returns accounts with base_url = https://api.opusmax.pro and their key status
func (h *OpusMaxHandler) GetOpusMaxAccounts(c *gin.Context) {
	ctx := c.Request.Context()

	accounts, err := h.accountLister.ListActiveByBaseURL(ctx, OpusMaxBaseURL)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	result := make([]OpusMaxAccount, 0, len(accounts))
	for _, account := range accounts {
		apiKey := account.GetCredential("api_key")
		if apiKey == "" {
			continue
		}

		status, err := h.fetchKeyStatus(apiKey)
		if err != nil {
			status = &OpusMaxKeyStatus{
				Status: "error",
				Name:   account.Name,
			}
		}

		// Determine display status
		displayStatus := account.Status
		if status.Status != "" && status.Status != "found" {
			displayStatus = status.Status
		} else if !status.IsActive || status.IsExpired {
			displayStatus = "disabled"
		} else if status.IsOverLimit {
			displayStatus = "error"
		}

		accountInfo := OpusMaxAccount{
			ID:               account.ID,
			Name:             account.Name,
			Status:           displayStatus,
			Schedulable:      account.Schedulable,
			Concurrency:      account.Concurrency,
			RPM:              status.RateLimit,
			WindowTokensLimit: fmt.Sprintf("%d", status.RateLimit),
			WindowTokensUsed: fmt.Sprintf("%.0f", float64(status.RateLimit)*status.UsagePercent/100),
			UsagePercent:     status.UsagePercent,
			PlanName:         status.PlanName,
			ExpiresAt:        status.ExpiresAt,
			TotalRequests:    status.TotalRequests,
			Last24HRequests:  status.Last24H.Requests,
		}
		result = append(result, accountInfo)
	}

	response.Success(c, OpusMaxAccountsResponse{
		Accounts:   result,
		UpdatedAt:  time.Now(),
		TotalCount: len(result),
	})
}

// fetchKeyStatus calls the OpusMax /api/key-status endpoint
func (h *OpusMaxHandler) fetchKeyStatus(apiKey string) (*OpusMaxKeyStatus, error) {
	url := fmt.Sprintf("%s/api/key-status?key=%s", OpusMaxBaseURL, apiKey)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var status OpusMaxKeyStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, err
	}

	return &status, nil
}
