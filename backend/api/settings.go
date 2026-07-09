package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/bejix/upstream-ops/backend/config"
	"github.com/bejix/upstream-ops/backend/scheduler"
	"github.com/bejix/upstream-ops/backend/storage"
	"github.com/gin-gonic/gin"
)

type settingsConfigView struct {
	App           config.AppConfig           `json:"app"`
	Auth          config.AuthConfig          `json:"auth"`
	Scheduler     config.SchedulerConfig     `json:"scheduler"`
	Notifications config.NotificationsConfig `json:"notifications"`
	Proxy         config.ProxyConfig         `json:"proxy"`
	Upstream      config.UpstreamConfig      `json:"upstream"`
}

type settingsConfigInput struct {
	App           config.AppConfig           `json:"app" binding:"required"`
	Auth          config.AuthConfig          `json:"auth" binding:"required"`
	Scheduler     config.SchedulerConfig     `json:"scheduler" binding:"required"`
	Notifications config.NotificationsConfig `json:"notifications" binding:"required"`
	Proxy         config.ProxyConfig         `json:"proxy"`
	Upstream      config.UpstreamConfig      `json:"upstream"`
}

func registerSettings(g *gin.RouterGroup, d *Deps) {
	gs := g.Group("/settings")
	gs.GET("/config", func(c *gin.Context) { getSettingsConfig(c, d) })
	gs.PUT("/config", func(c *gin.Context) { saveSettingsConfig(c, d) })
	gs.POST("/apply", func(c *gin.Context) { applySettingsConfig(c, d) })
	gs.GET("/user-scheduler", func(c *gin.Context) { getUserSchedulerConfig(c, d) })
	gs.PUT("/user-scheduler", func(c *gin.Context) { saveUserSchedulerConfig(c, d) })
	gs.POST("/proxy/test", func(c *gin.Context) {
		if !requireSuper(c, d) {
			return
		}
		testProxy(c)
	})
}

func getSettingsConfig(c *gin.Context, d *Deps) {
	if !requireSuper(c, d) {
		return
	}
	cfg, err := config.LoadFile(d.Runtime.ConfigPath())
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"config_path": d.Runtime.ConfigPath(),
			"config": settingsConfigView{
				App:           cfg.App,
				Auth:          cfg.Auth,
				Scheduler:     cfg.Scheduler,
				Notifications: cfg.Notifications,
				Proxy:         cfg.Proxy,
				Upstream:      cfg.Upstream,
			},
		},
	})
}

func saveSettingsConfig(c *gin.Context, d *Deps) {
	if !requireSuper(c, d) {
		return
	}
	var in settingsConfigInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}

	path := d.Runtime.ConfigPath()
	cfg, err := config.LoadFile(path)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}

	cfg.App.Title = in.App.Title
	cfg.App.NotificationPrefix = in.App.NotificationPrefix
	cfg.Auth = in.Auth
	cfg.Scheduler = in.Scheduler
	cfg.Notifications = in.Notifications
	cfg.Proxy = in.Proxy
	cfg.Upstream = in.Upstream.WithDefaults()

	if err := config.Save(path, cfg); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"config_path": path,
			"message":     "已写入配置文件",
		},
	})
}

func applySettingsConfig(c *gin.Context, d *Deps) {
	if !requireSuper(c, d) {
		return
	}
	result, err := d.Runtime.ApplyFromFile()
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func getUserSchedulerConfig(c *gin.Context, d *Deps) {
	u, ok := currentUser(c, d)
	if !ok {
		fail(c, http.StatusUnauthorized, errors.New("missing user"))
		return
	}
	cfg := config.SchedulerConfig{}
	inherited := true
	if d.Runtime != nil {
		if fileCfg, err := config.LoadFile(d.Runtime.ConfigPath()); err == nil {
			cfg = fileCfg.Scheduler
		}
	}
	if d.UserSchedulers != nil {
		row, err := d.UserSchedulers.Get(u.ID)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		if row != nil && row.ConfigJSON != "" {
			_ = json.Unmarshal([]byte(row.ConfigJSON), &cfg)
			inherited = false
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"scheduler": cfg, "inherited": inherited}})
}

func saveUserSchedulerConfig(c *gin.Context, d *Deps) {
	u, ok := currentUser(c, d)
	if !ok {
		fail(c, http.StatusUnauthorized, errors.New("missing user"))
		return
	}
	if isSuper(u) {
		fail(c, http.StatusBadRequest, errors.New("超级管理员请使用系统设置"))
		return
	}
	var in struct {
		Scheduler config.SchedulerConfig `json:"scheduler" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	raw, err := json.Marshal(in.Scheduler)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if err := scheduler.ValidateConfig(in.Scheduler); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if d.UserSchedulers == nil {
		fail(c, http.StatusServiceUnavailable, errors.New("user scheduler repo unavailable"))
		return
	}
	if err := d.UserSchedulers.Upsert(u.ID, string(raw)); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"message": "已保存个人调度，下一轮轮询生效", "scheduler": in.Scheduler}})
}

func schedulerConfigFromSetting(row storage.UserSchedulerSetting) (config.SchedulerConfig, error) {
	var cfg config.SchedulerConfig
	if row.ConfigJSON == "" {
		return cfg, nil
	}
	return cfg, json.Unmarshal([]byte(row.ConfigJSON), &cfg)
}
