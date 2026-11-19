package stats_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/myronsi/messenger-moderation/internal/stats"
)

func TestUsageClient(t *testing.T) {
	var seen *http.Request
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"day":"2026-11-18","active_day":7,"active_30d":30,"storage_bytes":9000}`))
	}))
	defer srv.Close()
	c := stats.NewUsageClient(srv.URL+"/", "0123456789abcdef0123456789abcdef")
	u, err := c.Usage(context.Background(), time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if u != (stats.Usage{ActiveDay: 7, Active30d: 30, StorageBytes: 9000}) {
		t.Fatalf("usage %+v", u)
	}
	if seen.URL.Path != "/internal/v1/stats/usage" || seen.URL.Query().Get("day") != "2026-11-18" ||
		seen.Header.Get("Authorization") != "Bearer 0123456789abcdef0123456789abcdef" {
		t.Fatalf("request %s %v", seen.URL, seen.Header)
	}
	status = http.StatusServiceUnavailable
	if _, err := c.Usage(context.Background(), time.Now()); err == nil {
		t.Fatal("an error answer is an error")
	}
}
