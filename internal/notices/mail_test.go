package notices

import (
	"context"
	"net/smtp"
	"strings"
	"testing"
	"time"
)

func TestComposeNoViolation(t *testing.T) {
	subject, body := Compose(Reply{
		NotifierName: "Jane", ReceivedAt: time.Date(2026, 11, 18, 9, 30, 0, 0, time.UTC), Location: "Profile @someone",
		Outcome: OutcomeNoViolation, ReasonCode: "other", ReasonText: "The post is satire.",
	})
	if subject != "Decision on your notice to Messenger" {
		t.Fatalf("subject %q", subject)
	}
	for _, want := range []string{"Hello Jane,", "18 November 2026, 09:30 (UTC)", "Profile @someone", "We found no violation", "Rule: Other breach", "The post is satire.", "out-of-court dispute settlement body"} {
		if !strings.Contains(body, want) {
			t.Errorf("body misses %q:\n%s", want, body)
		}
	}
}

func TestSMTPSend(t *testing.T) {
	var got struct {
		addr, from string
		to         []string
		msg        string
		auth       bool
	}
	m := NewSMTP("mail.example.com", 587, "user", "secret", "moderation@example.com")
	m.send = func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
		got.addr, got.from, got.to, got.msg, got.auth = addr, from, to, string(msg), auth != nil
		return nil
	}
	if err := m.Send(context.Background(), "jane@example.com", "Decision", "Line one\nLine two"); err != nil {
		t.Fatal(err)
	}
	if got.addr != "mail.example.com:587" || got.from != "moderation@example.com" || len(got.to) != 1 || got.to[0] != "jane@example.com" || !got.auth {
		t.Fatalf("envelope %+v", got)
	}
	for _, want := range []string{"From: moderation@example.com\r\n", "To: jane@example.com\r\n", "Subject: Decision\r\n", "charset=utf-8", "\r\n\r\nLine one\r\nLine two"} {
		if !strings.Contains(got.msg, want) {
			t.Errorf("message misses %q", want)
		}
	}
}

func TestSMTPRefusesInjectedHeaders(t *testing.T) {
	m := NewSMTP("mail.example.com", 587, "", "", "moderation@example.com")
	m.send = func(string, smtp.Auth, string, []string, []byte) error {
		t.Fatal("sent")
		return nil
	}
	for _, c := range []struct{ to, subject string }{
		{"jane@example.com\r\nBcc: other@example.com", "Decision"},
		{"jane@example.com", "Decision\nBcc: other@example.com"},
		{"Jane <jane@example.com>", "Decision"},
	} {
		if err := m.Send(context.Background(), c.to, c.subject, "body"); err == nil {
			t.Fatalf("accepted %q / %q", c.to, c.subject)
		}
	}
}

func TestCleanEmail(t *testing.T) {
	for in, ok := range map[string]bool{"": true, " jane@example.com ": true, "not an address": false, "Jane <jane@example.com>": false} {
		if _, err := CleanEmail(in); (err == nil) != ok {
			t.Errorf("%q: %v", in, err)
		}
	}
}
