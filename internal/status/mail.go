package status

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type MailConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       string
	Label    string
}

type MailNotifier struct {
	cfg  MailConfig
	send func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error
}

func NewMailNotifier(cfg MailConfig) *MailNotifier {
	return &MailNotifier{cfg: cfg, send: smtp.SendMail}
}

func (n *MailNotifier) IncidentOpened(_ context.Context, in Incident) error {
	subject := fmt.Sprintf("[%s] %s is down", n.cfg.Label, in.Component)
	body := fmt.Sprintf("%s failed its checks since %s (UTC).\n\nLast error: %s\n", in.Component, in.StartedAt.UTC().Format(time.DateTime), orNone(in.Detail))
	return n.mail(subject, body)
}

func (n *MailNotifier) IncidentClosed(_ context.Context, in Incident) error {
	end := time.Now()
	if in.EndedAt != nil {
		end = *in.EndedAt
	}
	duration := end.Sub(in.StartedAt).Round(time.Minute)
	subject := fmt.Sprintf("[%s] %s works again", n.cfg.Label, in.Component)
	if in.Kind == "monitoring_gap" {
		subject = fmt.Sprintf("[%s] monitoring was interrupted", n.cfg.Label)
	}
	body := fmt.Sprintf("%s\n\nFrom %s to %s (UTC), %s.\n", orNone(in.Detail), in.StartedAt.UTC().Format(time.DateTime), end.UTC().Format(time.DateTime), duration)
	return n.mail(subject, body)
}

func (n *MailNotifier) mail(subject, body string) error {
	headers := []string{
		"From: " + n.cfg.From,
		"To: " + n.cfg.To,
		"Subject: " + strings.ReplaceAll(subject, "\n", " "),
		"Date: " + time.Now().UTC().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
	}
	msg := strings.Join(headers, "\r\n") + "\r\n\r\n" + strings.ReplaceAll(body, "\n", "\r\n")
	var auth smtp.Auth
	if n.cfg.Username != "" {
		auth = smtp.PlainAuth("", n.cfg.Username, n.cfg.Password, n.cfg.Host)
	}
	addr := net.JoinHostPort(n.cfg.Host, strconv.Itoa(n.cfg.Port))
	return n.send(addr, auth, n.cfg.From, []string{n.cfg.To}, []byte(msg))
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
