package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/bejix/upstream-ops/backend/storage"
	"github.com/gin-gonic/gin"
)

func registerAuth(g *gin.RouterGroup, d *Deps) {
	g.POST("/auth/login", func(c *gin.Context) { login(c, d) })
	g.POST("/auth/register", func(c *gin.Context) { register(c, d) })
	g.GET("/auth/me", func(c *gin.Context) { whoami(c, d) })
	g.POST("/auth/logout", func(c *gin.Context) {
		// 无状态 token，客户端丢弃即可；这个接口仅作语义存在。
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
}

type loginInput struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type registerInput struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func login(c *gin.Context, d *Deps) {
	// 鉴权关闭：任何登录请求都直接成功；前端在 /auth/me 已经知道无需登录。
	authSvc := d.Runtime.CurrentAuth()
	if authSvc == nil {
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"auth_disabled": true,
				"username":      "anonymous",
			},
		})
		return
	}
	var in loginInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	token, exp, u, err := authSvc.Login(in.Username, in.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"token":      token,
			"expires_at": exp.Unix(),
			"user_id":    u.ID,
			"username":   u.Username,
			"role":       u.Role,
		},
	})
}

func register(c *gin.Context, d *Deps) {
	if d.Users == nil {
		fail(c, http.StatusServiceUnavailable, errors.New("users repo unavailable"))
		return
	}
	var in registerInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	username := strings.TrimSpace(in.Username)
	if username == "" || len(in.Password) < 6 {
		fail(c, http.StatusBadRequest, errors.New("账号不能为空，密码至少 6 位"))
		return
	}
	if username == storage.SuperAdminUsername {
		fail(c, http.StatusBadRequest, errors.New("该账号已保留"))
		return
	}
	u, err := d.Users.Create(username, in.Password)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": userOutput(u)})
}

// whoami 既是"前端启动探测"接口也是"已登录信息"接口。
//
//   - 鉴权关闭 → 返回 {auth_disabled: true}，前端据此跳过登录页
//   - 鉴权开启但未带 token → 中间件已经在前面 401 拦截，根本走不到这里
//   - 鉴权开启 + 有效 token → 返回 username
func whoami(c *gin.Context, d *Deps) {
	if d.Runtime.CurrentAuth() == nil {
		u, _ := superUser(d)
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"auth_disabled": true,
				"user_id":       u.ID,
				"username":      u.Username,
				"role":          u.Role,
			},
		})
		return
	}
	u, ok := currentUser(c, d)
	if !ok {
		fail(c, http.StatusUnauthorized, errors.New("missing user"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": userOutput(u)})
}

func userOutput(u *storage.SystemUser) gin.H {
	if u == nil {
		return gin.H{"username": "anonymous", "role": storage.UserRoleSuperAdmin}
	}
	return gin.H{
		"id":       u.ID,
		"user_id":  u.ID,
		"username": u.Username,
		"role":     u.Role,
		"enabled":  u.Enabled,
	}
}
