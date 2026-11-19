package status

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type State string

const (
	StateOK     State = "ok"
	StateSlow   State = "slow"
	StateFailed State = "failed"
)

type Result struct {
	Component string
	State     State
	Latency   time.Duration
	Detail    string
}

type Probe func(ctx context.Context) []Result

func ReadinessProbe(client *http.Client, url string) Probe {
	return func(ctx context.Context) []Result {
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return []Result{{Component: "api", State: StateFailed, Detail: err.Error()}}
		}
		resp, err := client.Do(req)
		latency := time.Since(start)
		if err != nil {
			return []Result{{Component: "api", State: StateFailed, Latency: latency, Detail: shortError(err)}}
		}
		defer func() { _ = resp.Body.Close() }()
		var body struct {
			Status string            `json:"status"`
			Checks map[string]string `json:"checks"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body); err != nil {
			return []Result{{Component: "api", State: StateFailed, Latency: latency, Detail: fmt.Sprintf("unreadable readiness answer (HTTP %d)", resp.StatusCode)}}
		}
		results := []Result{{Component: "api", State: StateOK, Latency: latency}}
		if body.Status == "shutting_down" {
			results[0] = Result{Component: "api", State: StateFailed, Latency: latency, Detail: "shutting down"}
		}
		for name, value := range body.Checks {
			r := Result{Component: name, State: StateOK, Latency: latency}
			if value != "ok" {
				r.State, r.Detail = StateFailed, value
			}
			results = append(results, r)
		}
		return results
	}
}

func HTTPProbe(client *http.Client, component, url, token string) Probe {
	return func(ctx context.Context) []Result {
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return []Result{{Component: component, State: StateFailed, Detail: err.Error()}}
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		latency := time.Since(start)
		if err != nil {
			return []Result{{Component: component, State: StateFailed, Latency: latency, Detail: shortError(err)}}
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return []Result{{Component: component, State: StateFailed, Latency: latency, Detail: fmt.Sprintf("HTTP %d", resp.StatusCode)}}
		}
		return []Result{{Component: component, State: StateOK, Latency: latency}}
	}
}

func FuncProbe(component string, ping func(ctx context.Context) error) Probe {
	return func(ctx context.Context) []Result {
		start := time.Now()
		err := ping(ctx)
		r := Result{Component: component, State: StateOK, Latency: time.Since(start)}
		if err != nil {
			r.State, r.Detail = StateFailed, shortError(err)
		}
		return []Result{r}
	}
}

func shortError(err error) string {
	msg := err.Error()
	if len(msg) > 200 {
		return msg[:200]
	}
	return msg
}
