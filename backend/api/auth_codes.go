package api

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const emailCodeTTL = 10 * time.Minute

type emailCodeEntry struct {
	Code   string
	Expiry time.Time
	SentAt time.Time
}

var emailCodes = struct {
	sync.Mutex
	m map[string]emailCodeEntry
}{m: map[string]emailCodeEntry{}}

type sendCodeInput struct {
	Username string `json:"username" binding:"required"`
	Action   string `json:"action" binding:"required"`
}

func sendAuthCode(c *gin.Context, d *Deps) {
	var in sendCodeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, 400, err)
		return
	}
	email, err := normalizeAllowedEmail(in.Username)
	if err != nil {
		fail(c, 400, err)
		return
	}
	action, err := normalizeCodeAction(in.Action)
	if err != nil {
		fail(c, 400, err)
		return
	}
	key := action + ":" + email
	now := time.Now()
	emailCodes.Lock()
	if old, ok := emailCodes.m[key]; ok && now.Sub(old.SentAt) < time.Minute {
		emailCodes.Unlock()
		fail(c, 429, errors.New("验证码发送太频繁，请 1 分钟后再试"))
		return
	}
	code := randomCode()
	emailCodes.m[key] = emailCodeEntry{Code: code, Expiry: now.Add(emailCodeTTL), SentAt: now}
	emailCodes.Unlock()

	if err := sendVerificationEmail(email, code, action); err != nil {
		fail(c, 500, err)
		return
	}
	c.JSON(200, gin.H{"ok": true, "data": gin.H{"expires_in": int(emailCodeTTL.Seconds())}})
}

func normalizeAllowedEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" {
		return "", errors.New("请输入有效邮箱")
	}
	switch parts[1] {
	case "qq.com", "163.com", "gmail.com":
		return email, nil
	default:
		return "", errors.New("仅支持 qq.com、163.com、gmail.com 邮箱")
	}
}

func normalizeCodeAction(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "register":
		return strings.ToLower(strings.TrimSpace(raw)), nil
	default:
		return "", errors.New("验证码类型无效")
	}
}

func verifyEmailCode(email, action, code string) error {
	email, err := normalizeAllowedEmail(email)
	if err != nil {
		return err
	}
	action, err = normalizeCodeAction(action)
	if err != nil {
		return err
	}
	code = strings.TrimSpace(code)
	key := action + ":" + email
	emailCodes.Lock()
	defer emailCodes.Unlock()
	entry, ok := emailCodes.m[key]
	if !ok || time.Now().After(entry.Expiry) || entry.Code != code {
		return errors.New("验证码错误或已过期")
	}
	delete(emailCodes.m, key)
	return nil
}

func randomCode() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	n := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if n < 0 {
		n = -n
	}
	return fmt.Sprintf("%06d", n%1000000)
}

func sendVerificationEmail(to, code, action string) error {
	cfg := smtpConfigFromEnv()
	if cfg.Password == "" {
		return errors.New("SMTP_PASSWORD 未配置")
	}
	title := "登录验证码"
	if action == "register" {
		title = "注册验证码"
	}
	subject := "UpstreamOps " + title
	html := fmt.Sprintf(`<!doctype html><html><body style="margin:0;background:#f6f7fb;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#111827;">
<table width="100%%" cellpadding="0" cellspacing="0" style="padding:32px 12px;background:#f6f7fb;"><tr><td align="center">
<table width="100%%" cellpadding="0" cellspacing="0" style="max-width:520px;background:#ffffff;border:1px solid #e5e7eb;border-radius:18px;overflow:hidden;box-shadow:0 12px 30px rgba(15,23,42,.08);">
<tr><td style="padding:28px 30px 10px;"><div style="font-size:13px;color:#6b7280;letter-spacing:.08em;text-transform:uppercase;">UpstreamOps</div><h1 style="margin:8px 0 0;font-size:22px;line-height:1.3;">%s</h1></td></tr>
<tr><td style="padding:18px 30px 8px;"><div style="border-radius:16px;background:linear-gradient(135deg,#111827,#2563eb);padding:24px;text-align:center;color:#fff;"><div style="font-size:13px;opacity:.78;">你的验证码</div><div style="margin-top:10px;font-size:40px;line-height:1;font-weight:800;letter-spacing:.18em;">%s</div></div></td></tr>
<tr><td style="padding:14px 30px 28px;font-size:14px;line-height:1.7;color:#4b5563;">验证码 10 分钟内有效。若不是你本人操作，可以忽略此邮件。</td></tr>
</table></td></tr></table></body></html>`, title, code)
	msg := strings.Join([]string{
		"From: UpstreamOps <" + cfg.Username + ">",
		"To: " + to,
		"Subject: " + mimeHeader(subject),
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"",
		html,
	}, "\r\n")
	return sendSMTP(cfg, to, []byte(msg))
}

type smtpConfig struct {
	Username string
	Password string
	Host     string
	Port     int
	TLS      bool
}

func smtpConfigFromEnv() smtpConfig {
	port, _ := strconv.Atoi(envDefault("SMTP_PORT", "465"))
	return smtpConfig{
		Username: envDefault("SMTP_USERNAME", "1550696493@qq.com"),
		Password: os.Getenv("SMTP_PASSWORD"),
		Host:     envDefault("SMTP_HOST", "smtp.qq.com"),
		Port:     port,
		TLS:      strings.ToLower(envDefault("SMTP_TLS", "true")) != "false",
	}
}

func envDefault(k, v string) string {
	if s := strings.TrimSpace(os.Getenv(k)); s != "" {
		return s
	}
	return v
}

func sendSMTP(cfg smtpConfig, to string, msg []byte) error {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	if !cfg.TLS {
		return smtp.SendMail(addr, auth, cfg.Username, []string{to}, msg)
	}
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	defer conn.Close()
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.Auth(auth); err != nil {
		return err
	}
	if err := client.Mail(cfg.Username); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func mimeHeader(s string) string {
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
}
