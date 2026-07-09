package api

import (
	"net/http"
	"strconv"

	"github.com/bejix/upstream-ops/backend/relay"
	"github.com/gin-gonic/gin"
)

func registerRelay(g *gin.RouterGroup, d *Deps) {
	gp := g.Group("/relay")
	gp.GET("/config", func(c *gin.Context) { relayConfig(c, d) })
	gp.PUT("/config", func(c *gin.Context) { saveRelayConfig(c, d) })
	gp.POST("/test", func(c *gin.Context) { testRelay(c, d) })
	gp.GET("/accounts", func(c *gin.Context) { relayAccounts(c, d) })
	gp.GET("/summary", func(c *gin.Context) { relaySummary(c, d) })
	gp.GET("/users", func(c *gin.Context) { relayUsers(c, d) })
}

func relayService(c *gin.Context, d *Deps) *relay.Service {
	if d == nil || d.Relay == nil {
		fail(c, http.StatusServiceUnavailable, errRelayUnavailable{})
		return nil
	}
	return d.Relay
}

type errRelayUnavailable struct{}

func (errRelayUnavailable) Error() string { return "relay service unavailable" }

func relayConfig(c *gin.Context, d *Deps) {
	svc := relayService(c, d)
	if svc == nil {
		return
	}
	out, err := svc.GetConfig()
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func saveRelayConfig(c *gin.Context, d *Deps) {
	svc := relayService(c, d)
	if svc == nil {
		return
	}
	var in relay.ConfigInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	out, err := svc.SaveConfig(c.Request.Context(), in)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func testRelay(c *gin.Context, d *Deps) {
	svc := relayService(c, d)
	if svc == nil {
		return
	}
	var in relay.ConfigInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	out, err := svc.Test(c.Request.Context(), in)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func relayAccounts(c *gin.Context, d *Deps) {
	svc := relayService(c, d)
	if svc == nil {
		return
	}
	out, err := svc.Accounts(c.Request.Context())
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func relaySummary(c *gin.Context, d *Deps) {
	svc := relayService(c, d)
	if svc == nil {
		return
	}
	out, err := svc.Summary(c.Request.Context(), c.Query("date"))
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func relayUsers(c *gin.Context, d *Deps) {
	svc := relayService(c, d)
	if svc == nil {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	out, err := svc.Users(c.Request.Context(), c.Query("date"), page, pageSize)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}
