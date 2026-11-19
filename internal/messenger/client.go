package messenger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	ErrNotConfigured = errors.New("the Messenger internal API is not configured")
	ErrNotFound      = errors.New("not found in Messenger")
	ErrForbidden     = errors.New("messenger refused the action")
)

type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string { return e.Field + ": " + e.Message }

type Client struct {
	base  string
	token string
	http  *http.Client
}

func New(base, token string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{base: strings.TrimRight(base, "/"), token: token, http: hc}
}

type StatusView struct {
	Status         string     `json:"status"`
	Since          *time.Time `json:"since"`
	Restrictions   []string   `json:"restrictions"`
	CanAppeal      bool       `json:"can_appeal"`
	ProbationUntil *time.Time `json:"probation_until"`
	DecidedBy      string     `json:"decided_by"`
	ReasonCode     *string    `json:"reason_code"`
	ReasonText     *string    `json:"reason_text"`
}

type Change struct {
	Before StatusView `json:"before"`
	After  StatusView `json:"after"`
}

type Dismissal struct {
	Dismissed int        `json:"dismissed"`
	Before    StatusView `json:"before"`
	After     StatusView `json:"after"`
}

type StatusRequest struct {
	Status     string `json:"status"`
	ReasonCode string `json:"reason_code"`
	ReasonText string `json:"reason_text"`
	StaffRef   string `json:"staff_ref,omitempty"`
}

func id(n int64) string { return strconv.FormatInt(n, 10) }

func (c *Client) SetStatus(ctx context.Context, userID int64, req StatusRequest) (Change, error) {
	var out Change
	err := c.do(ctx, http.MethodPost, "/users/"+id(userID)+"/status", req, &out)
	return out, err
}

func (c *Client) DismissReports(ctx context.Context, userID int64, reporterIDs []int64) (Dismissal, error) {
	var body any
	if reporterIDs != nil {
		ids := make([]string, len(reporterIDs))
		for i, r := range reporterIDs {
			ids[i] = id(r)
		}
		body = map[string][]string{"reporter_ids": ids}
	}
	var out Dismissal
	err := c.do(ctx, http.MethodPost, "/users/"+id(userID)+"/reports/dismiss", body, &out)
	return out, err
}

func (c *Client) SetPackHidden(ctx context.Context, kind string, packID int64, hidden bool) (bool, error) {
	action := "unhide"
	if hidden {
		action = "hide"
	}
	var out struct {
		Changed bool `json:"changed"`
	}
	err := c.do(ctx, http.MethodPost, "/packs/"+kind+"/"+id(packID)+"/"+action, nil, &out)
	return out.Changed, err
}

func (c *Client) DeleteAccount(ctx context.Context, userID int64) error {
	return c.do(ctx, http.MethodDelete, "/users/"+id(userID), nil, nil)
}

func (c *Client) PackImage(ctx context.Context, attachmentID string) (*http.Response, error) {
	resp, err := c.send(ctx, http.MethodGet, "/pack-images/"+attachmentID, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		return nil, failure(resp)
	}
	return resp, nil
}

func (c *Client) send(ctx context.Context, method, path string, body any) (*http.Response, error) {
	if c == nil || c.base == "" {
		return nil, ErrNotConfigured
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/internal/v1"+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("messenger %s %s: %w", method, path, err)
	}
	return resp, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	resp, err := c.send(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return failure(resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("messenger %s %s: decode: %w", method, path, err)
	}
	return nil
}

func failure(resp *http.Response) error {
	var p struct {
		Error   string `json:"error"`
		Field   string `json:"field"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&p)
	switch resp.StatusCode {
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusForbidden:
		return ErrForbidden
	case http.StatusBadRequest:
		return &FieldError{Field: p.Field, Message: p.Message}
	}
	return fmt.Errorf("messenger answered %d %s", resp.StatusCode, p.Error)
}
