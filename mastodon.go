package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	server string
	token  string
	http   *http.Client
}

func NewClient(server, token string) *Client {
	return &Client{
		server: strings.TrimRight(server, "/"),
		token:  token,
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

type Account struct {
	ID          string `json:"id"`
	Acct        string `json:"acct"`
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar"`
	URL         string `json:"url"`
}

type MediaAttachment struct {
	Type        string `json:"type"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

type Status struct {
	ID               string            `json:"id"`
	CreatedAt        time.Time         `json:"created_at"`
	InReplyToID      *string           `json:"in_reply_to_id"`
	Content          string            `json:"content"`
	Visibility       string            `json:"visibility"`
	URL              string            `json:"url"`
	Account          Account           `json:"account"`
	MediaAttachments []MediaAttachment `json:"media_attachments"`
}

type Notification struct {
	ID     string  `json:"id"`
	Type   string  `json:"type"`
	Status *Status `json:"status"`
}

type StatusContext struct {
	Ancestors   []Status `json:"ancestors"`
	Descendants []Status `json:"descendants"`
}

func (c *Client) do(ctx context.Context, method, path string, query, form url.Values, out any) error {
	u := c.server + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(b))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) VerifyCredentials(ctx context.Context) (Account, error) {
	var a Account
	err := c.do(ctx, http.MethodGet, "/api/v1/accounts/verify_credentials", nil, nil, &a)
	return a, err
}

func (c *Client) Mentions(ctx context.Context, sinceID string) ([]Notification, error) {
	q := url.Values{"types[]": {"mention"}}
	if sinceID != "" {
		q.Set("since_id", sinceID)
	}
	var ns []Notification
	err := c.do(ctx, http.MethodGet, "/api/v1/notifications", q, nil, &ns)
	return ns, err
}

func (c *Client) Context(ctx context.Context, statusID string) (StatusContext, error) {
	var sc StatusContext
	err := c.do(ctx, http.MethodGet, "/api/v1/statuses/"+statusID+"/context", nil, nil, &sc)
	return sc, err
}

func (c *Client) Reblog(ctx context.Context, statusID string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/statuses/"+statusID+"/reblog", nil, nil, nil)
}

func (c *Client) Reply(ctx context.Context, to *Status, text string) error {
	visibility := to.Visibility
	if visibility == "public" {
		visibility = "unlisted"
	}
	form := url.Values{
		"status":         {text},
		"in_reply_to_id": {to.ID},
		"visibility":     {visibility},
	}
	return c.do(ctx, http.MethodPost, "/api/v1/statuses", nil, form, nil)
}
