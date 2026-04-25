package cloak

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

const defaultBaseURL = "http://localhost:8080"

type Client struct {
	baseURL string
	http    *http.Client
}

type ProfileResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CDPURL    string `json:"cdp_url"`
	UserData  string `json:"user_data_dir"`
	UpdatedAt string `json:"updated_at"`
}

type LaunchResponse struct {
	ProfileID string `json:"profile_id"`
	Status    string `json:"status"`
	CDPURL    string `json:"cdp_url"`
}

type VersionResponse struct {
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func NormalizeBaseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultBaseURL
	}
	return strings.TrimRight(trimmed, "/")
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: NormalizeBaseURL(baseURL),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) CreateProfile(name string, backend *bridge.ProfileBackendCloak) (*ProfileResponse, error) {
	payload := map[string]any{
		"name": name,
	}
	applyBackendPayload(payload, backend)
	var out ProfileResponse
	if err := c.doJSON(http.MethodPost, "/api/profiles", payload, &out, http.StatusCreated); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateProfile(profileID, name string, backend *bridge.ProfileBackendCloak) (*ProfileResponse, error) {
	payload := map[string]any{
		"name": name,
	}
	applyBackendPayload(payload, backend)
	var out ProfileResponse
	if err := c.doJSON(http.MethodPut, "/api/profiles/"+url.PathEscape(strings.TrimSpace(profileID)), payload, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteProfile(profileID string) error {
	return c.doJSON(http.MethodDelete, "/api/profiles/"+url.PathEscape(strings.TrimSpace(profileID)), nil, nil, http.StatusOK)
}

func (c *Client) LaunchProfile(profileID string) (*LaunchResponse, error) {
	var out LaunchResponse
	if err := c.doJSON(http.MethodPost, "/api/profiles/"+url.PathEscape(strings.TrimSpace(profileID))+"/launch", nil, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) StopProfile(profileID string) error {
	return c.doJSON(http.MethodPost, "/api/profiles/"+url.PathEscape(strings.TrimSpace(profileID))+"/stop", nil, nil, http.StatusOK)
}

func (c *Client) BrowserWSEndpoint(profileID string) (string, error) {
	var out VersionResponse
	path := "/api/profiles/" + url.PathEscape(strings.TrimSpace(profileID)) + "/cdp/json/version"
	if err := c.doJSON(http.MethodGet, path, nil, &out, http.StatusOK); err != nil {
		return "", err
	}
	wsURL := strings.TrimSpace(out.WebSocketDebuggerURL)
	if wsURL == "" {
		return "", fmt.Errorf("cloak did not return webSocketDebuggerUrl")
	}
	return wsURL, nil
}

func (c *Client) doJSON(method, path string, body any, out any, expectedStatus int) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode cloak request: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build cloak request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cloak request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != expectedStatus {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("cloak %s %s returned %d: %s", method, path, resp.StatusCode, msg)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode cloak response: %w", err)
	}
	return nil
}

func applyBackendPayload(payload map[string]any, backend *bridge.ProfileBackendCloak) {
	if backend == nil {
		return
	}
	if v := strings.TrimSpace(backend.ProxyURL); v != "" {
		payload["proxy"] = v
	}
	if v := strings.TrimSpace(backend.Timezone); v != "" {
		payload["timezone"] = v
	}
	if v := strings.TrimSpace(backend.Locale); v != "" {
		payload["locale"] = v
	}
	if v := strings.TrimSpace(backend.Platform); v != "" {
		payload["platform"] = v
	}
	if v := strings.TrimSpace(backend.UserAgent); v != "" {
		payload["user_agent"] = v
	}
	if backend.Headless != nil {
		payload["headless"] = *backend.Headless
	}
	if backend.Humanize != nil {
		payload["humanize"] = *backend.Humanize
	}
	if backend.GeoIP != nil {
		payload["geoip"] = *backend.GeoIP
	}
	if v := strings.TrimSpace(backend.Notes); v != "" {
		payload["notes"] = v
	}
}
