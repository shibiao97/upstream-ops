package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bejix/upstream-ops/backend/storage"
	"github.com/gin-gonic/gin"
)

type Service struct {
	users    *storage.Users
	secret   []byte
	tokenTTL time.Duration
}

func New(users *storage.Users, secret string, ttl time.Duration) (*Service, error) {
	if users == nil {
		return nil, errors.New("users repo is nil")
	}
	if secret == "" {
		return nil, errors.New("auth token secret is empty")
	}
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	return &Service{users: users, secret: []byte(secret), tokenTTL: ttl}, nil
}

type claims struct {
	Sub  string `json:"sub"`
	UID  uint   `json:"uid"`
	Role string `json:"role"`
	Exp  int64  `json:"exp"`
}

func (s *Service) Login(username, password string) (string, time.Time, *storage.SystemUser, error) {
	u, err := s.users.FindByUsername(username)
	if err != nil || !u.Enabled || !storage.CheckPassword(u.PasswordHash, password) {
		return "", time.Time{}, nil, errors.New("invalid username or password")
	}
	expiresAt := time.Now().Add(s.tokenTTL)
	tok, err := s.sign(claims{Sub: u.Username, UID: u.ID, Role: string(u.Role), Exp: expiresAt.Unix()})
	return tok, expiresAt, u, err
}

func (s *Service) Verify(token string) (*storage.SystemUser, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, errors.New("malformed token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode sig: %w", err)
	}
	if subtle.ConstantTimeCompare(sig, s.mac(payload)) != 1 {
		return nil, errors.New("bad signature")
	}
	var c claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, fmt.Errorf("decode claims: %w", err)
	}
	if time.Now().Unix() > c.Exp {
		return nil, errors.New("token expired")
	}
	u, err := s.users.FindByID(c.UID)
	if err != nil || !u.Enabled || u.Username != c.Sub || string(u.Role) != c.Role {
		return nil, errors.New("unknown subject")
	}
	return u, nil
}

func (s *Service) sign(c claims) (string, error) {
	body, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	sig := s.mac(body)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (s *Service) mac(payload []byte) []byte {
	m := hmac.New(sha256.New, s.secret)
	m.Write(payload)
	return m.Sum(nil)
}

func (s *Service) TokenTTL() time.Duration { return s.tokenTTL }

func (s *Service) Middleware() gin.HandlerFunc {
	whitelist := map[string]struct{}{
		"/healthz":            {},
		"/api/version":        {},
		"/api/auth/login":     {},
		"/api/auth/register":  {},
		"/api/auth/send-code": {},
	}
	return func(c *gin.Context) {
		if _, ok := whitelist[c.FullPath()]; ok {
			c.Next()
			return
		}
		if _, ok := whitelist[c.Request.URL.Path]; ok {
			c.Next()
			return
		}
		token := extractToken(c.Request)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		u, err := s.Verify(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.Set("authUser", u)
		c.Next()
	}
}

func extractToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	if c, err := r.Cookie("uh_token"); err == nil {
		return c.Value
	}
	return ""
}
