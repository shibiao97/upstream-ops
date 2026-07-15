package notify

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bejix/upstream-ops/backend/crypto"
	"github.com/bejix/upstream-ops/backend/storage"
)

func TestFailureNotificationThresholdAndRecovery(t *testing.T) {
	db, err := storage.Open(storage.DBConfig{Driver: storage.DBDriverSQLite, Path: filepath.Join(t.TempDir(), "notify-test.db")})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := storage.AutoMigrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	channels := storage.NewChannels(db)
	upstream := &storage.Channel{Name: "demo", Type: storage.ChannelTypeNewAPI, SiteURL: "https://example.com", Username: "u", PasswordCipher: "x"}
	if err := channels.Create(upstream); err != nil {
		t.Fatalf("create upstream: %v", err)
	}

	cipher, err := crypto.NewCipher("secret")
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	configCipher, err := cipher.Encrypt(fmt.Sprintf(`{"url":%q}`, server.URL))
	if err != nil {
		t.Fatalf("encrypt config: %v", err)
	}

	notifications := storage.NewNotifications(db)
	if err := notifications.CreateChannel(&storage.NotificationChannel{Name: "webhook", Type: storage.NotifyWebhook, ConfigCipher: configCipher, Enabled: true}); err != nil {
		t.Fatalf("create notification channel: %v", err)
	}
	dispatcher := NewDispatcher(notifications, cipher, slog.New(slog.NewTextHandler(io.Discard, nil)), Policy{SendMaxAttempts: 1})
	msg := Message{Event: storage.EventMonitorFailed, ChannelID: upstream.ID, Subject: "demo 余额采集失败", Body: "upstream unavailable"}

	for i := 0; i < 3; i++ {
		if err := dispatcher.DispatchFailure(context.Background(), "balance", msg); err != nil {
			t.Fatalf("dispatch failure %d: %v", i+1, err)
		}
	}
	// 重新创建 Dispatcher，确认连续失败状态不是只保存在内存里。
	dispatcher = NewDispatcher(notifications, cipher, slog.New(slog.NewTextHandler(io.Discard, nil)), Policy{SendMaxAttempts: 1})
	if err := dispatcher.DispatchFailure(context.Background(), "balance", msg); err != nil {
		t.Fatalf("dispatch failure 4: %v", err)
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("failure notifications = %d, want 3", got)
	}

	recovery := Message{Event: storage.EventMonitorFailed, ChannelID: upstream.ID, Subject: "demo 余额采集恢复正常"}
	if err := dispatcher.DispatchRecovery(context.Background(), "balance", recovery); err != nil {
		t.Fatalf("dispatch recovery: %v", err)
	}
	if err := dispatcher.DispatchRecovery(context.Background(), "balance", recovery); err != nil {
		t.Fatalf("dispatch duplicate recovery: %v", err)
	}
	if got := hits.Load(); got != 4 {
		t.Fatalf("notifications after recovery = %d, want 4", got)
	}
	logs, err := notifications.ListLogs(10)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 4 || !strings.Contains(logs[0].Subject, "恢复正常") || !strings.Contains(logs[0].Body, "连续失败：4 次") {
		t.Fatalf("unexpected recovery log: %#v", logs)
	}
}
