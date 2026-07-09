package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bejix/upstream-ops/backend/crypto"
	"github.com/bejix/upstream-ops/backend/relay"
	"github.com/bejix/upstream-ops/backend/storage"
	"github.com/gin-gonic/gin"
)

func newRelayTestDeps(t *testing.T, siteURL string) *Deps {
	t.Helper()
	db := openTestDB(t)
	cipher, err := crypto.NewCipher("relay-test-secret")
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	return &Deps{Relay: relay.NewService(storage.NewRelays(db), cipher)}
}

func TestRelaySaveRequiresSub2APIAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			var body struct {
				Email string `json:"email"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			token := "user-token"
			if body.Email == "admin@example.com" {
				token = "admin-token"
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"` + token + `"}}`))
		case "/api/v1/admin/accounts":
			if r.Header.Get("Authorization") != "Bearer admin-token" {
				http.Error(w, `{"code":"FORBIDDEN"}`, http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":1,"name":"acc-a","rate_multiplier":2}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer fake.Close()

	r := gin.New()
	deps := newRelayTestDeps(t, fake.URL)
	registerRelay(r.Group("/api"), deps)

	badReq := httptest.NewRequest(http.MethodPut, "/api/relay/config", strings.NewReader(`{"site_url":"`+fake.URL+`","admin_email":"user@example.com","password":"p","enabled":true}`))
	badReq.Header.Set("Content-Type", "application/json")
	badRec := httptest.NewRecorder()
	r.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("non-admin save status = %d body = %s", badRec.Code, badRec.Body.String())
	}

	goodReq := httptest.NewRequest(http.MethodPut, "/api/relay/config", strings.NewReader(`{"site_url":"`+fake.URL+`","admin_email":"admin@example.com","password":"p","enabled":true,"pull_interval_minutes":15,"account_multipliers":[{"account_id":1,"name":"acc-a","multiplier":3}]}`))
	goodReq.Header.Set("Content-Type", "application/json")
	goodRec := httptest.NewRecorder()
	r.ServeHTTP(goodRec, goodReq)
	if goodRec.Code != http.StatusOK {
		t.Fatalf("admin save status = %d body = %s", goodRec.Code, goodRec.Body.String())
	}
	var goodResp struct {
		Data relay.ConfigOutput `json:"data"`
	}
	if err := json.Unmarshal(goodRec.Body.Bytes(), &goodResp); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if goodResp.Data.PullIntervalMinutes != 15 {
		t.Fatalf("pull interval = %d, want 15", goodResp.Data.PullIntervalMinutes)
	}
}

func TestRelayUsersAggregateAndSortByActualCost(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"admin-token"}}`))
		case "/api/v1/admin/accounts":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":1,"name":"acc-a","rate_multiplier":2},{"id":2,"name":"acc-b","rate_multiplier":4}]}}`))
		case "/api/v1/admin/usage":
			if got := r.URL.Query().Get("start_date"); got != "2026-07-09" {
				t.Errorf("usage start_date = %q, want 2026-07-09", got)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[
				{"user_id":1,"username":"admin@example.com","account_id":1,"account_name":"acc-a","actual_cost":100,"total_cost":100,"account_rate_multiplier":1},
				{"user_id":10,"username":"u10","account_id":1,"account_name":"acc-a","actual_cost":6,"total_cost":10,"account_rate_multiplier":0.1},
				{"user_id":20,"username":"u20","account_id":2,"account_name":"acc-b","actual_cost":10,"total_cost":20,"account_rate_multiplier":0.1},
				{"user_id":10,"username":"u10","account_id":2,"account_name":"acc-b","actual_cost":2,"total_cost":99,"account_stats_cost":5,"account_rate_multiplier":0.2}
			]}}`))
		case "/api/v1/admin/usage/stats":
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_actual_cost":118,"total_account_cost":104,"total_requests":4}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer fake.Close()

	r := gin.New()
	deps := newRelayTestDeps(t, fake.URL)
	registerRelay(r.Group("/api"), deps)

	saveReq := httptest.NewRequest(http.MethodPut, "/api/relay/config", strings.NewReader(`{"site_url":"`+fake.URL+`","admin_email":"admin@example.com","password":"p","enabled":true,"account_multipliers":[{"account_id":1,"name":"acc-a","multiplier":0.1},{"account_id":2,"name":"acc-b","multiplier":0.2}]}`))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	r.ServeHTTP(saveRec, saveReq)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("save status = %d body = %s", saveRec.Code, saveRec.Body.String())
	}

	summaryReq := httptest.NewRequest(http.MethodGet, "/api/relay/summary?date=2026-07-09", nil)
	summaryRec := httptest.NewRecorder()
	r.ServeHTTP(summaryRec, summaryReq)
	if summaryRec.Code != http.StatusOK {
		t.Fatalf("summary status = %d body = %s", summaryRec.Code, summaryRec.Body.String())
	}
	var summaryResp struct {
		Data relay.Summary `json:"data"`
	}
	if err := json.Unmarshal(summaryRec.Body.Bytes(), &summaryResp); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summaryResp.Data.ActualCost != 18 || summaryResp.Data.Cost != 4.0 {
		t.Fatalf("summary = %#v, want actual 18 cost 4", summaryResp.Data)
	}

	usersReq := httptest.NewRequest(http.MethodGet, "/api/relay/users?date=2026-07-09&page=1&page_size=20", nil)
	usersRec := httptest.NewRecorder()
	r.ServeHTTP(usersRec, usersReq)
	if usersRec.Code != http.StatusOK {
		t.Fatalf("users status = %d body = %s", usersRec.Code, usersRec.Body.String())
	}
	var usersResp struct {
		Data relay.UsersPage `json:"data"`
	}
	if err := json.Unmarshal(usersRec.Body.Bytes(), &usersResp); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	if len(usersResp.Data.Items) != 2 || usersResp.Data.Items[0].Username != "u20" || usersResp.Data.Items[0].ActualCost != 10 {
		t.Fatalf("users not sorted by actual cost desc: %#v", usersResp.Data.Items)
	}
	if usersResp.Data.ActualCost != 18 || usersResp.Data.Cost != 4.0 {
		t.Fatalf("users totals = %#v, want actual 18 cost 4", usersResp.Data)
	}
	if usersResp.Data.Items[1].Cost != 2.0 || usersResp.Data.Items[1].MainAccount != "acc-a" {
		t.Fatalf("user aggregate = %#v", usersResp.Data.Items[1])
	}
}
