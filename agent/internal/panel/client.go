// Package panel talks to the Pelican panel's client API.
package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

type Client struct {
	base string
	key  string
	http *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{base: strings.TrimRight(baseURL, "/"), key: apiKey, http: &http.Client{Timeout: 15 * time.Second}}
}

// Base returns the panel URL, for resolving its address.
func (c *Client) Base() string { return c.base }

func (c *Client) do(ctx context.Context, method, path string, body any, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return 0, err
	}

	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))

	if res.StatusCode < 200 || res.StatusCode > 299 {
		snippet := strings.TrimSpace(string(data))
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}

		return res.StatusCode, fmt.Errorf("%s %s: HTTP %d: %s", method, path, res.StatusCode, snippet)
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return res.StatusCode, fmt.Errorf("%s %s: unexpected response: %w", method, path, err)
		}
	}

	return res.StatusCode, nil
}

func serverPath(id, suffix string) string {
	return "/api/client/servers/" + url.PathEscape(id) + suffix
}

// State returns the server's current state as reported by Wings: offline, starting, running, stopping.
func (c *Client) State(ctx context.Context, id string) (string, error) {
	var res struct {
		Attributes struct {
			CurrentState string `json:"current_state"`
			IsSuspended  bool   `json:"is_suspended"`
		} `json:"attributes"`
		CurrentState string `json:"current_state"`
	}

	if _, err := c.do(ctx, http.MethodGet, serverPath(id, "/resources"), nil, &res); err != nil {
		return "", err
	}

	state := res.Attributes.CurrentState
	if state == "" {
		state = res.CurrentState
	}

	if state == "" {
		return "", fmt.Errorf("the panel did not report a state for server %s", id)
	}

	if res.Attributes.IsSuspended {
		return "suspended", nil
	}

	return state, nil
}

// Start asks the panel to start the server.
func (c *Client) Start(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodPost, serverPath(id, "/power"), map[string]string{"signal": "start"}, nil)

	return err
}

// Details is what doctor needs to know about a server.
type Details struct {
	Name            string
	Permissions     []string
	CanStart        bool
	IsServerOwner   bool
	AllocationPorts []int
}

// Details fetches a server and the calling user's permissions on it.
func (c *Client) Details(ctx context.Context, id string) (Details, error) {
	var res struct {
		Attributes struct {
			Name string `json:"name"`
		} `json:"attributes"`
		Meta struct {
			IsServerOwner   bool     `json:"is_server_owner"`
			UserPermissions []string `json:"user_permissions"`
		} `json:"meta"`
	}

	if _, err := c.do(ctx, http.MethodGet, serverPath(id, ""), nil, &res); err != nil {
		return Details{}, err
	}

	d := Details{Name: res.Attributes.Name, Permissions: res.Meta.UserPermissions, IsServerOwner: res.Meta.IsServerOwner}
	// "*" is how the panel reports an owner/admin; otherwise look for the specific permission.
	d.CanStart = slices.Contains(d.Permissions, "*") || slices.Contains(d.Permissions, "control.start")

	return d, nil
}

// Ping checks the key is accepted at all.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, "/api/client", nil, nil)

	return err
}
