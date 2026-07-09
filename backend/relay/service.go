package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bejix/upstream-ops/backend/crypto"
	"github.com/bejix/upstream-ops/backend/storage"
)

const defaultPageSize = 200
const defaultPullIntervalMinutes = 5

type Service struct {
	Repo   *storage.Relays
	Cipher *crypto.Cipher
	HTTP   *http.Client
}

func NewService(repo *storage.Relays, cipher *crypto.Cipher) *Service {
	return &Service{Repo: repo, Cipher: cipher, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

type AccountMultiplierInput struct {
	AccountID  int64   `json:"account_id"`
	Name       string  `json:"name"`
	Multiplier float64 `json:"multiplier"`
}

type ConfigInput struct {
	Name                string                   `json:"name"`
	SiteURL             string                   `json:"site_url"`
	AdminEmail          string                   `json:"admin_email"`
	Password            string                   `json:"password"`
	Enabled             bool                     `json:"enabled"`
	PullIntervalMinutes int                      `json:"pull_interval_minutes"`
	AccountMultipliers  []AccountMultiplierInput `json:"account_multipliers"`
}

type ConfigOutput struct {
	Configured          bool                     `json:"configured"`
	ID                  uint                     `json:"id,omitempty"`
	Name                string                   `json:"name,omitempty"`
	SiteURL             string                   `json:"site_url,omitempty"`
	AdminEmail          string                   `json:"admin_email,omitempty"`
	Enabled             bool                     `json:"enabled"`
	PullIntervalMinutes int                      `json:"pull_interval_minutes"`
	LastCheckedAt       *time.Time               `json:"last_checked_at,omitempty"`
	LastError           string                   `json:"last_error,omitempty"`
	AccountMultipliers  []AccountMultiplierInput `json:"account_multipliers"`
}

type TestResult struct {
	OK       bool           `json:"ok"`
	Message  string         `json:"message"`
	Accounts []RelayAccount `json:"accounts,omitempty"`
}

type RelayAccount struct {
	ID             int64   `json:"id"`
	Name           string  `json:"name"`
	RateMultiplier float64 `json:"rate_multiplier,omitempty"`
}

type Summary struct {
	Configured    bool       `json:"configured"`
	Enabled       bool       `json:"enabled"`
	ActualCost    float64    `json:"actual_cost"`
	Cost          float64    `json:"cost"`
	RequestCount  int        `json:"request_count"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

type UsersPage struct {
	Items      []UserUsage `json:"items"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	Pages      int         `json:"pages"`
	ActualCost float64     `json:"actual_cost"`
	Cost       float64     `json:"cost"`
}

type UserUsage struct {
	UserID       int64                `json:"user_id"`
	Username     string               `json:"username"`
	ActualCost   float64              `json:"actual_cost"`
	Cost         float64              `json:"cost"`
	RequestCount int                  `json:"request_count"`
	MainAccount  string               `json:"main_account"`
	Accounts     []AccountUsageDetail `json:"accounts"`
}

type AccountUsageDetail struct {
	AccountID    int64   `json:"account_id"`
	AccountName  string  `json:"account_name"`
	ActualCost   float64 `json:"actual_cost"`
	Cost         float64 `json:"cost"`
	Multiplier   float64 `json:"multiplier"`
	RequestCount int     `json:"request_count"`
}

type usageLog struct {
	UserID                int64
	Username              string
	AccountID             int64
	AccountName           string
	ActualCost            float64
	AccountCost           float64
	AccountRateMultiplier float64
	RequestCount          int
}

type sub2Resp struct {
	Code    any             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (s *Service) GetConfig() (*ConfigOutput, error) {
	cfg, err := s.Repo.FindConfig()
	if err != nil || cfg == nil {
		return &ConfigOutput{Enabled: true, PullIntervalMinutes: defaultPullIntervalMinutes}, err
	}
	multipliers, err := s.Repo.ListMultipliers(cfg.ID)
	if err != nil {
		return nil, err
	}
	out := configOutput(cfg)
	out.AccountMultipliers = multiplierOutput(multipliers)
	return out, nil
}

func (s *Service) Test(ctx context.Context, in ConfigInput) (*TestResult, error) {
	cfg, password, err := s.resolveInput(in)
	if err != nil {
		return nil, err
	}
	token, err := s.login(ctx, cfg.SiteURL, cfg.AdminEmail, password)
	if err != nil {
		return nil, err
	}
	accounts, err := s.listAccounts(ctx, cfg.SiteURL, token, 1, 200)
	if err != nil {
		return nil, fmt.Errorf("admin permission check failed: %w", err)
	}
	return &TestResult{OK: true, Message: "管理员权限校验通过", Accounts: accounts}, nil
}

func (s *Service) SaveConfig(ctx context.Context, in ConfigInput) (*ConfigOutput, error) {
	cfg, password, err := s.resolveInput(in)
	if err != nil {
		return nil, err
	}
	token, err := s.login(ctx, cfg.SiteURL, cfg.AdminEmail, password)
	if err != nil {
		return nil, err
	}
	accounts, err := s.listAccounts(ctx, cfg.SiteURL, token, 1, 200)
	if err != nil {
		return nil, fmt.Errorf("admin permission check failed: %w", err)
	}
	enc, err := s.Cipher.Encrypt(password)
	if err != nil {
		return nil, fmt.Errorf("encrypt relay password: %w", err)
	}
	cfg.PasswordCipher = enc
	multipliers := mergeMultipliers(cfg.ID, in.AccountMultipliers, accounts)
	if err := s.Repo.SaveConfig(cfg, multipliers); err != nil {
		return nil, err
	}
	_ = s.Repo.SetCheckResult(cfg.ID, "")
	return s.GetConfig()
}

func (s *Service) Accounts(ctx context.Context) ([]RelayAccount, error) {
	cfg, password, err := s.configWithPassword()
	if err != nil {
		return nil, err
	}
	token, err := s.login(ctx, cfg.SiteURL, cfg.AdminEmail, password)
	if err != nil {
		return nil, err
	}
	return s.listAccounts(ctx, cfg.SiteURL, token, 1, 500)
}

func (s *Service) Summary(ctx context.Context, date string) (*Summary, error) {
	cfg, password, err := s.configWithPassword()
	if err != nil {
		if errors.Is(err, errNotConfigured) {
			return &Summary{Configured: false}, nil
		}
		return nil, err
	}
	out := &Summary{Configured: true, Enabled: cfg.Enabled, LastCheckedAt: cfg.LastCheckedAt, LastError: cfg.LastError}
	if !cfg.Enabled {
		return out, nil
	}
	stats, _ := s.fetchUsageStats(ctx, cfg, password, date)
	logs, err := s.fetchUsageLogs(ctx, cfg, password, date)
	if err != nil {
		_ = s.Repo.SetCheckResult(cfg.ID, err.Error())
		return nil, err
	}
	for _, log := range logs {
		out.ActualCost += log.ActualCost
		out.Cost += log.AccountCost
		out.RequestCount += log.RequestCount
	}
	if stats != nil && stats.ActualCost > 0 {
		out.ActualCost = stats.ActualCost
	}
	if stats != nil && stats.Cost > 0 {
		out.Cost = stats.Cost
	}
	if stats != nil && stats.RequestCount > 0 {
		out.RequestCount = stats.RequestCount
	}
	out.ActualCost = round4(out.ActualCost)
	out.Cost = round4(out.Cost)
	_ = s.Repo.SetCheckResult(cfg.ID, "")
	return out, nil
}

func (s *Service) fetchUsageStats(ctx context.Context, cfg *storage.RelayConfig, password, date string) (*Summary, error) {
	token, err := s.login(ctx, cfg.SiteURL, cfg.AdminEmail, password)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(date) == "" {
		date = time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02")
	}
	q := url.Values{}
	q.Set("start_date", date)
	q.Set("end_date", date)
	q.Set("timezone", "Asia/Shanghai")
	var wrapped sub2Resp
	if err := s.doJSON(ctx, http.MethodGet, cfg.SiteURL+"/api/v1/admin/usage/stats?"+q.Encode(), token, nil, &wrapped); err != nil {
		return nil, err
	}
	if !respOK(wrapped.Code) {
		return nil, errors.New(wrapped.Message)
	}
	var data map[string]any
	if err := json.Unmarshal(wrapped.Data, &data); err != nil {
		return nil, err
	}
	return &Summary{
		ActualCost:   floatValue(data, "total_actual_cost", "actual_cost", "today_actual_cost"),
		Cost:         floatValue(data, "total_account_cost", "account_cost", "today_account_cost"),
		RequestCount: int(anyInt(data["total_requests"]) + anyInt(data["request_count"])),
	}, nil
}

func (s *Service) Users(ctx context.Context, date string, page, pageSize int) (*UsersPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	cfg, password, err := s.configWithPassword()
	if err != nil {
		return nil, err
	}
	logs, err := s.fetchUsageLogs(ctx, cfg, password, date)
	if err != nil {
		_ = s.Repo.SetCheckResult(cfg.ID, err.Error())
		return nil, err
	}
	mults, err := s.multiplierMap(cfg.ID)
	if err != nil {
		return nil, err
	}
	items := aggregateUsers(logs, mults)
	var actualCost, cost float64
	for _, item := range items {
		actualCost += item.ActualCost
		cost += item.Cost
	}
	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	pages := 1
	if total > 0 {
		pages = int(math.Ceil(float64(total) / float64(pageSize)))
	}
	return &UsersPage{Items: items[start:end], Total: total, Page: page, PageSize: pageSize, Pages: pages, ActualCost: round4(actualCost), Cost: round4(cost)}, nil
}

var errNotConfigured = errors.New("relay is not configured")

func (s *Service) configWithPassword() (*storage.RelayConfig, string, error) {
	cfg, err := s.Repo.FindConfig()
	if err != nil {
		return nil, "", err
	}
	if cfg == nil {
		return nil, "", errNotConfigured
	}
	password, err := s.Cipher.Decrypt(cfg.PasswordCipher)
	if err != nil {
		return nil, "", fmt.Errorf("decrypt relay password: %w", err)
	}
	return cfg, password, nil
}

func (s *Service) resolveInput(in ConfigInput) (*storage.RelayConfig, string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "自有中转站"
	}
	site := strings.TrimRight(strings.TrimSpace(in.SiteURL), "/")
	email := strings.TrimSpace(in.AdminEmail)
	if site == "" || email == "" {
		return nil, "", errors.New("站点地址和管理员邮箱必填")
	}
	password := in.Password
	existing, err := s.Repo.FindConfig()
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(password) == "" && existing != nil {
		password, err = s.Cipher.Decrypt(existing.PasswordCipher)
		if err != nil {
			return nil, "", fmt.Errorf("decrypt relay password: %w", err)
		}
	}
	if strings.TrimSpace(password) == "" {
		return nil, "", errors.New("管理员密码必填")
	}
	id := uint(1)
	if existing != nil {
		id = existing.ID
	}
	return &storage.RelayConfig{ID: id, Name: name, SiteURL: site, AdminEmail: email, Enabled: in.Enabled, PullIntervalMinutes: normalizePullInterval(in.PullIntervalMinutes)}, password, nil
}

func (s *Service) login(ctx context.Context, site, email, password string) (string, error) {
	body := map[string]string{"email": email, "password": password}
	var wrapped sub2Resp
	if err := s.doJSON(ctx, http.MethodPost, site+"/api/v1/auth/login", "", body, &wrapped); err != nil {
		return "", fmt.Errorf("sub2api login: %w", err)
	}
	if !respOK(wrapped.Code) {
		return "", fmt.Errorf("sub2api login: %s", wrapped.Message)
	}
	var data struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(wrapped.Data, &data); err != nil {
		return "", fmt.Errorf("sub2api login data: %w", err)
	}
	if strings.TrimSpace(data.AccessToken) == "" {
		return "", errors.New("sub2api login returned empty access_token")
	}
	return data.AccessToken, nil
}

func (s *Service) listAccounts(ctx context.Context, site, token string, page, pageSize int) ([]RelayAccount, error) {
	u := site + "/api/v1/admin/accounts?page=" + strconv.Itoa(page) + "&page_size=" + strconv.Itoa(pageSize) + "&lite=1"
	var wrapped sub2Resp
	if err := s.doJSON(ctx, http.MethodGet, u, token, nil, &wrapped); err != nil {
		return nil, err
	}
	if !respOK(wrapped.Code) {
		return nil, errors.New(wrapped.Message)
	}
	items := extractItems(wrapped.Data)
	accounts := make([]RelayAccount, 0, len(items))
	for _, item := range items {
		id := intValue(item, "id", "account_id")
		if id == 0 {
			continue
		}
		accounts = append(accounts, RelayAccount{
			ID:             id,
			Name:           firstString(item, "name", "email", "username"),
			RateMultiplier: floatValue(item, "rate_multiplier", "account_rate_multiplier"),
		})
	}
	return accounts, nil
}

func (s *Service) fetchUsageLogs(ctx context.Context, cfg *storage.RelayConfig, password, date string) ([]usageLog, error) {
	token, err := s.login(ctx, cfg.SiteURL, cfg.AdminEmail, password)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(date) == "" {
		date = time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02")
	}
	var out []usageLog
	for page := 1; page <= 50; page++ {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("page_size", strconv.Itoa(defaultPageSize))
		q.Set("sort_by", "actual_cost")
		q.Set("sort_order", "desc")
		q.Set("start_date", date)
		q.Set("end_date", date)
		q.Set("timezone", "Asia/Shanghai")
		var wrapped sub2Resp
		if err := s.doJSON(ctx, http.MethodGet, cfg.SiteURL+"/api/v1/admin/usage?"+q.Encode(), token, nil, &wrapped); err != nil {
			return nil, err
		}
		if !respOK(wrapped.Code) {
			return nil, errors.New(wrapped.Message)
		}
		items := extractItems(wrapped.Data)
		for _, item := range items {
			actual := floatValue(item, "actual_cost", "actualCost")
			if actual <= 0 {
				continue
			}
			accountCost := usageAccountCost(item)
			accountRateMultiplier := usageAccountRateMultiplier(item)
			out = append(out, usageLog{
				UserID:                intValue(item, "user_id", "userId"),
				Username:              firstString(item, "username", "user_email", "email", "user_name"),
				AccountID:             intValue(item, "account_id", "accountId", "channel_id"),
				AccountName:           firstString(item, "account_name", "account", "channel_name", "name"),
				ActualCost:            actual,
				AccountCost:           accountCost,
				AccountRateMultiplier: accountRateMultiplier,
				RequestCount:          1,
			})
		}
		if len(items) < defaultPageSize {
			break
		}
	}
	return out, nil
}

func usageAccountCost(item map[string]any) float64 {
	base := floatValue(item, "account_stats_cost", "accountStatsCost")
	if base <= 0 {
		base = floatValue(item, "total_cost", "totalCost", "cost")
	}
	return base * usageAccountRateMultiplier(item)
}

func usageAccountRateMultiplier(item map[string]any) float64 {
	mult := floatValue(item, "account_rate_multiplier", "accountRateMultiplier")
	if mult <= 0 {
		return 1
	}
	return mult
}

func (s *Service) doJSON(ctx context.Context, method, url, token string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := s.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return err
	}
	return nil
}

func normalizePullInterval(minutes int) int {
	if minutes <= 0 {
		return defaultPullIntervalMinutes
	}
	if minutes > 1440 {
		return 1440
	}
	return minutes
}

func configOutput(cfg *storage.RelayConfig) *ConfigOutput {
	return &ConfigOutput{
		Configured:          true,
		ID:                  cfg.ID,
		Name:                cfg.Name,
		SiteURL:             cfg.SiteURL,
		AdminEmail:          cfg.AdminEmail,
		Enabled:             cfg.Enabled,
		PullIntervalMinutes: normalizePullInterval(cfg.PullIntervalMinutes),
		LastCheckedAt:       cfg.LastCheckedAt,
		LastError:           cfg.LastError,
	}
}

func multiplierOutput(list []storage.RelayAccountMultiplier) []AccountMultiplierInput {
	out := make([]AccountMultiplierInput, 0, len(list))
	for _, m := range list {
		out = append(out, AccountMultiplierInput{AccountID: m.AccountID, Name: m.Name, Multiplier: m.Multiplier})
	}
	return out
}

func mergeMultipliers(configID uint, input []AccountMultiplierInput, accounts []RelayAccount) []storage.RelayAccountMultiplier {
	byID := map[int64]AccountMultiplierInput{}
	for _, m := range input {
		if m.AccountID != 0 {
			byID[m.AccountID] = m
		}
	}
	out := make([]storage.RelayAccountMultiplier, 0, len(accounts)+len(input))
	seen := map[int64]bool{}
	for _, acc := range accounts {
		mult := acc.RateMultiplier
		if mult <= 0 {
			mult = 1
		}
		name := acc.Name
		if in, ok := byID[acc.ID]; ok {
			if in.Multiplier > 0 {
				mult = in.Multiplier
			}
			if strings.TrimSpace(in.Name) != "" {
				name = in.Name
			}
		}
		out = append(out, storage.RelayAccountMultiplier{ConfigID: configID, AccountID: acc.ID, Name: name, Multiplier: mult})
		seen[acc.ID] = true
	}
	for _, in := range input {
		if in.AccountID == 0 || seen[in.AccountID] {
			continue
		}
		out = append(out, storage.RelayAccountMultiplier{ConfigID: configID, AccountID: in.AccountID, Name: in.Name, Multiplier: in.Multiplier})
	}
	return out
}

func (s *Service) multiplierMap(configID uint) (map[int64]float64, error) {
	list, err := s.Repo.ListMultipliers(configID)
	if err != nil {
		return nil, err
	}
	out := map[int64]float64{}
	for _, m := range list {
		out[m.AccountID] = m.Multiplier
	}
	return out, nil
}

func aggregateUsers(logs []usageLog, multipliers map[int64]float64) []UserUsage {
	users := map[string]*UserUsage{}
	accountMaps := map[string]map[int64]*AccountUsageDetail{}
	for _, log := range logs {
		key := strconv.FormatInt(log.UserID, 10) + ":" + log.Username
		user := users[key]
		if user == nil {
			user = &UserUsage{UserID: log.UserID, Username: displayName(log.Username, log.UserID)}
			users[key] = user
			accountMaps[key] = map[int64]*AccountUsageDetail{}
		}
		mult := log.AccountRateMultiplier
		if mult <= 0 {
			mult = multiplierFor(multipliers, log.AccountID, 0)
		}
		cost := log.AccountCost
		if cost <= 0 {
			cost = log.ActualCost * mult
		}
		user.ActualCost += log.ActualCost
		user.Cost += cost
		user.RequestCount += log.RequestCount
		account := accountMaps[key][log.AccountID]
		if account == nil {
			account = &AccountUsageDetail{AccountID: log.AccountID, AccountName: displayAccount(log.AccountName, log.AccountID), Multiplier: mult}
			accountMaps[key][log.AccountID] = account
		}
		account.ActualCost += log.ActualCost
		account.Cost += cost
		account.RequestCount += log.RequestCount
	}
	out := make([]UserUsage, 0, len(users))
	for key, user := range users {
		for _, account := range accountMaps[key] {
			account.ActualCost = round4(account.ActualCost)
			account.Cost = round4(account.Cost)
			user.Accounts = append(user.Accounts, *account)
		}
		sort.Slice(user.Accounts, func(i, j int) bool { return user.Accounts[i].ActualCost > user.Accounts[j].ActualCost })
		if len(user.Accounts) > 0 {
			user.MainAccount = user.Accounts[0].AccountName
		}
		user.ActualCost = round4(user.ActualCost)
		user.Cost = round4(user.Cost)
		out = append(out, *user)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ActualCost > out[j].ActualCost })
	return out
}

func extractItems(raw json.RawMessage) []map[string]any {
	var arr []map[string]any
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) != nil {
		return nil
	}
	for _, key := range []string{"items", "records", "list", "data"} {
		if v, ok := obj[key]; ok {
			raw, _ := json.Marshal(v)
			if json.Unmarshal(raw, &arr) == nil {
				return arr
			}
		}
	}
	return nil
}

func respOK(code any) bool {
	switch v := code.(type) {
	case nil:
		return true
	case float64:
		return v == 0
	case string:
		return v == "0" || strings.EqualFold(v, "ok")
	default:
		return false
	}
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch x := v.(type) {
			case string:
				if strings.TrimSpace(x) != "" {
					return strings.TrimSpace(x)
				}
			case map[string]any:
				if s := firstString(x, "name", "email", "username"); s != "" {
					return s
				}
			}
		}
	}
	for _, nested := range []string{"user", "account", "channel"} {
		if v, ok := m[nested].(map[string]any); ok {
			if s := firstString(v, keys...); s != "" {
				return s
			}
		}
	}
	return ""
}

func intValue(m map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if n := anyInt(v); n != 0 {
				return n
			}
		}
	}
	for _, nested := range []string{"user", "account", "channel"} {
		if v, ok := m[nested].(map[string]any); ok {
			if n := intValue(v, "id"); n != 0 {
				return n
			}
		}
	}
	return 0
}

func anyInt(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	case json.Number:
		n, _ := x.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	default:
		return 0
	}
}

func floatValue(m map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if n := anyFloat(v); n != 0 {
				return n
			}
		}
	}
	return 0
}

func anyFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		n, _ := x.Float64()
		return n
	case string:
		n, _ := strconv.ParseFloat(x, 64)
		return n
	default:
		return 0
	}
}

func multiplierFor(m map[int64]float64, accountID int64, fallback float64) float64 {
	if v := m[accountID]; v > 0 {
		return v
	}
	if fallback > 0 {
		return fallback
	}
	return 1
}

func displayName(name string, id int64) string {
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	if id != 0 {
		return "用户 #" + strconv.FormatInt(id, 10)
	}
	return "未知用户"
}

func displayAccount(name string, id int64) string {
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	if id != 0 {
		return "账号 #" + strconv.FormatInt(id, 10)
	}
	return "未知账号"
}

func round4(v float64) float64 { return math.Round(v*10000) / 10000 }
