package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/myronsi/messenger-moderation/internal/config"
	"github.com/myronsi/messenger-moderation/internal/status"
)

func statusProbes(cfg config.Config, pingDB func(ctx context.Context) error) []status.Probe {
	client := &http.Client{Timeout: 8 * time.Second}
	probes := []status.Probe{status.FuncProbe("moderation_db", pingDB)}
	if cfg.Status.APIReadyURL != "" {
		probes = append(probes, status.ReadinessProbe(client, cfg.Status.APIReadyURL))
	}
	if cfg.Status.WorkerHealthURL != "" {
		probes = append(probes, status.HTTPProbe(client, "worker", cfg.Status.WorkerHealthURL, ""))
	}
	if cfg.Status.LiveKitURL != "" {
		probes = append(probes, status.HTTPProbe(client, "livekit", cfg.Status.LiveKitURL, ""))
	}
	if cfg.Messenger.InternalURL != "" {
		probes = append(probes, status.HTTPProbe(client, "internal_api", strings.TrimRight(cfg.Messenger.InternalURL, "/")+"/internal/v1/ping", cfg.Messenger.InternalToken))
	}
	return probes
}

func alertNotifier(cfg config.Config) status.Notifier {
	if cfg.Mail.AlertEmail == "" {
		return nil
	}
	return status.NewMailNotifier(status.MailConfig{
		Host: cfg.Mail.Host, Port: cfg.Mail.Port, Username: cfg.Mail.Username, Password: cfg.Mail.Password,
		From: cfg.Mail.From, To: cfg.Mail.AlertEmail, Label: "messenger " + cfg.Environment,
	})
}
