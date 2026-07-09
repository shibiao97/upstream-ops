package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/bejix/upstream-ops/backend/storage"
	"github.com/gin-gonic/gin"
)

func registerUsers(g *gin.RouterGroup, d *Deps) {
	gu := g.Group("/users")
	gu.GET("", func(c *gin.Context) {
		if !requireSuper(c, d) {
			return
		}
		list, err := d.Users.List(c.Query("search"))
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		out := make([]gin.H, 0, len(list))
		for i := range list {
			out = append(out, userOutput(&list[i]))
		}
		c.JSON(http.StatusOK, gin.H{"data": out})
	})
	gu.POST("/:id/enable", func(c *gin.Context) { setUserEnabled(c, d, true) })
	gu.POST("/:id/disable", func(c *gin.Context) { setUserEnabled(c, d, false) })
	gu.DELETE("/:id", func(c *gin.Context) {
		if !requireSuper(c, d) {
			return
		}
		id, err := uintParam(c, "id")
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		if err := d.Users.DeleteCascade(id); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
}

func setUserEnabled(c *gin.Context, d *Deps, enabled bool) {
	if !requireSuper(c, d) {
		return
	}
	id, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if err := d.Users.SetEnabled(id, enabled); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func currentUser(c *gin.Context, d *Deps) (*storage.SystemUser, bool) {
	if v, ok := c.Get("authUser"); ok {
		if u, ok := v.(*storage.SystemUser); ok && u != nil {
			return u, true
		}
	}
	if d != nil && (d.Runtime == nil || d.Runtime.CurrentAuth() == nil) {
		u, err := superUser(d)
		return u, err == nil
	}
	return nil, false
}

func superUser(d *Deps) (*storage.SystemUser, error) {
	if d == nil || d.Users == nil {
		return &storage.SystemUser{ID: 0, Username: storage.SuperAdminUsername, Role: storage.UserRoleSuperAdmin, Enabled: true}, nil
	}
	return d.Users.FindByUsername(storage.SuperAdminUsername)
}

func isSuper(u *storage.SystemUser) bool {
	return u != nil && u.Role == storage.UserRoleSuperAdmin && u.Username == storage.SuperAdminUsername
}

func requireSuper(c *gin.Context, d *Deps) bool {
	u, ok := currentUser(c, d)
	if !ok || !isSuper(u) {
		fail(c, http.StatusForbidden, errors.New("需要超级管理员权限"))
		return false
	}
	return true
}

func visibleOwnerParam(c *gin.Context, u *storage.SystemUser) uint {
	if !isSuper(u) {
		return u.ID
	}
	s := c.Query("owner_user_id")
	if s == "" {
		s = c.Query("user_id")
	}
	if s == "" || s == "all" {
		return 0
	}
	id, _ := strconv.ParseUint(s, 10, 64)
	return uint(id)
}

func canUseChannel(c *gin.Context, d *Deps, id uint) (*storage.Channel, bool) {
	u, ok := currentUser(c, d)
	if !ok {
		fail(c, http.StatusUnauthorized, errors.New("missing user"))
		return nil, false
	}
	if d == nil || d.Channels == nil {
		return &storage.Channel{ID: id, OwnerUserID: u.ID}, true
	}
	ch, err := d.Channels.FindByID(id)
	if err != nil {
		fail(c, http.StatusNotFound, err)
		return nil, false
	}
	if !isSuper(u) && ch.OwnerUserID != u.ID {
		fail(c, http.StatusForbidden, errors.New("无权访问该渠道"))
		return nil, false
	}
	return ch, true
}
