package demo

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// hello is the public /api/v1/plugin/demo/hello handler. It returns the
// current greeting (falling back to defaultGreeting) and the plugin id
// so integration tests can confirm the plugin is wired end to end.
func (p *Plugin) hello(c *gin.Context) {
	greeting := defaultGreeting
	if v, err := p.core.Settings().Get(c.Request.Context(), settingKeyGreeting); err == nil {
		if s, ok := v.(string); ok && s != "" {
			greeting = s
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"plugin":   PluginID,
		"greeting": greeting,
	})
}

// listNotes returns up to 50 recent notes written by the plugin. Admin
// only; the auth middleware is installed by the host based on the route's
// AuthRequirement.
func (p *Plugin) listNotes(c *gin.Context) {
	if p.client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "demo: ent client unavailable (host driver not wired)",
		})
		return
	}
	notes, err := p.client.Note.Query().
		Limit(50).
		All(c.Request.Context())
	if err != nil {
		p.core.Logger().Error("demo: list notes failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]noteView, 0, len(notes))
	for _, n := range notes {
		out = append(out, noteView{
			ID:        n.ID,
			AccountID: n.AccountID,
			Content:   n.Content,
			CreatedAt: n.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	c.JSON(http.StatusOK, gin.H{"notes": out})
}

// noteView is the wire shape for the listNotes endpoint. Defined here
// rather than in api/exports.go because it is an HTTP contract, not a
// cross-plugin contract.
type noteView struct {
	ID        int64  `json:"id"`
	AccountID int64  `json:"account_id"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}
