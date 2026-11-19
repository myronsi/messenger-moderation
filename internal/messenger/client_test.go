package messenger_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/myronsi/messenger-moderation/internal/messenger"
)

func TestClientCallsTheInternalAPI(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		seen = append(seen, r.Method+" "+r.URL.Path+" "+string(body))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/internal/v1/users/5/status":
			_, _ = w.Write([]byte(`{"before":{"status":"limited","decided_by":"automatic"},"after":{"status":"restricted","decided_by":"staff","reason_code":"spam"}}`))
		case "/internal/v1/users/5/reports/dismiss":
			_, _ = w.Write([]byte(`{"dismissed":3,"before":{"status":"limited"},"after":{"status":"good"}}`))
		case "/internal/v1/packs/emoji_pack/4/hide":
			_, _ = w.Write([]byte(`{"changed":true}`))
		case "/internal/v1/users/6":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not_found"}`))
		case "/internal/v1/users/7/status":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_request","field":"reason_text","message":"1 to 1000 characters"}`))
		case "/internal/v1/pack-images/abc":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png"))
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()
	c := messenger.New(srv.URL+"/", "secret", nil)
	ctx := context.Background()

	change, err := c.SetStatus(ctx, 5, messenger.StatusRequest{Status: "restricted", ReasonCode: "spam", ReasonText: "Links", StaffRef: "staff:1"})
	if err != nil || change.Before.Status != "limited" || change.After.DecidedBy != "staff" || *change.After.ReasonCode != "spam" {
		t.Fatalf("set status %+v %v", change, err)
	}
	var sent map[string]string
	if err := json.Unmarshal([]byte(seen[0][len("POST /internal/v1/users/5/status "):]), &sent); err != nil || sent["staff_ref"] != "staff:1" {
		t.Fatalf("request %q", seen[0])
	}
	if d, err := c.DismissReports(ctx, 5, []int64{8, 9}); err != nil || d.Dismissed != 3 || seen[1] != `POST /internal/v1/users/5/reports/dismiss {"reporter_ids":["8","9"]}` {
		t.Fatalf("dismiss %+v %v %q", d, err, seen[1])
	}
	if _, err := c.DismissReports(ctx, 5, nil); err != nil || seen[2] != "POST /internal/v1/users/5/reports/dismiss " {
		t.Fatalf("dismiss all %v %q", err, seen[2])
	}
	if changed, err := c.SetPackHidden(ctx, "emoji_pack", 4, true); err != nil || !changed {
		t.Fatalf("hide %v %v", changed, err)
	}
	if err := c.DeleteAccount(ctx, 6); !errors.Is(err, messenger.ErrNotFound) {
		t.Fatalf("unknown account %v", err)
	}
	var field *messenger.FieldError
	if _, err := c.SetStatus(ctx, 7, messenger.StatusRequest{}); !errors.As(err, &field) || field.Field != "reason_text" {
		t.Fatalf("field error %v", err)
	}
	resp, err := c.PackImage(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(b) != "png" {
		t.Fatalf("image %q", b)
	}
	if _, err := c.SetPackHidden(ctx, "sticker_pack", 1, false); err == nil {
		t.Fatal("a 503 is an error")
	}
	if _, err := messenger.New("", "", nil).SetStatus(ctx, 1, messenger.StatusRequest{}); !errors.Is(err, messenger.ErrNotConfigured) {
		t.Fatalf("not configured %v", err)
	}
	if _, err := messenger.New(srv.URL, "wrong", nil).SetPackHidden(ctx, "emoji_pack", 4, true); err == nil {
		t.Fatal("a wrong token is an error")
	}
}
