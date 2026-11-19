package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Usage struct {
	ActiveDay    int64 `json:"active_day"`
	Active30d    int64 `json:"active_30d"`
	StorageBytes int64 `json:"storage_bytes"`
}

type UsageSource interface {
	Usage(ctx context.Context, day time.Time) (Usage, error)
}

type UsageFunc func(ctx context.Context, day time.Time) (Usage, error)

func (f UsageFunc) Usage(ctx context.Context, day time.Time) (Usage, error) { return f(ctx, day) }

func NewUsageClient(baseURL, token string) UsageFunc {
	return usageClient(&http.Client{Timeout: 20 * time.Second}, strings.TrimRight(baseURL, "/")+"/internal/v1/stats/usage", token)
}

func usageClient(client *http.Client, target, token string) UsageFunc {
	return func(ctx context.Context, day time.Time) (Usage, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return Usage{}, err
		}
		req.URL.RawQuery = url.Values{"day": {day.UTC().Format(time.DateOnly)}}.Encode()
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err != nil {
			return Usage{}, fmt.Errorf("usage figures: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return Usage{}, fmt.Errorf("usage figures: messenger answered %d", resp.StatusCode)
		}
		var u Usage
		if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&u); err != nil {
			return Usage{}, fmt.Errorf("usage figures: %w", err)
		}
		return u, nil
	}
}
