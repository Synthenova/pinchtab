// Package types contains shared API types for the dashboard.
// These types are exported to TypeScript via tygo.
package types

import "time"

// ProfileBackendSteel mirrors the Steel-specific backend settings for a profile.
type ProfileBackendSteel struct {
	ProxyURL       string   `json:"proxyUrl,omitempty"`
	ExtensionPaths []string `json:"extensionPaths,omitempty"`
}

// ProfileBackendCloak mirrors the Cloak-specific backend settings for a profile.
type ProfileBackendCloak struct {
	BaseURL    string   `json:"baseUrl,omitempty"`
	ProfileID  string   `json:"profileId,omitempty"`
	ProxyURL   string   `json:"proxyUrl,omitempty"`
	Timezone   string   `json:"timezone,omitempty"`
	Locale     string   `json:"locale,omitempty"`
	Platform   string   `json:"platform,omitempty"`
	UserAgent  string   `json:"userAgent,omitempty"`
	LaunchArgs []string `json:"launchArgs,omitempty"`
	Headless   *bool    `json:"headless,omitempty"`
	Humanize   *bool    `json:"humanize,omitempty"`
	GeoIP      *bool    `json:"geoip,omitempty"`
	Notes      string   `json:"notes,omitempty"`
}

// ProfileBackendPinchTab mirrors the PinchTab-specific backend settings for a profile.
type ProfileBackendPinchTab struct {
	ProxyURL       string              `json:"proxyUrl,omitempty"`
	Timezone       string              `json:"timezone,omitempty"`
	Locale         string              `json:"locale,omitempty"`
	Binary         string              `json:"binary,omitempty"`
	BrowserVersion string              `json:"browserVersion,omitempty"`
	LaunchArgs     []string            `json:"launchArgs,omitempty"`
	Cloud          *ProfileCloudConfig `json:"cloud,omitempty"`
}

// ProfileCloudConfig mirrors cloud profile settings for PinchTab-backed profiles.
type ProfileCloudConfig struct {
	Enabled        *bool  `json:"enabled,omitempty"`
	Provider       string `json:"provider,omitempty"`
	Bucket         string `json:"bucket,omitempty"`
	Prefix         string `json:"prefix,omitempty"`
	ProfileID      string `json:"profileId,omitempty"`
	CredentialPath string `json:"credentialPath,omitempty"`
	KeepLocalCache *bool  `json:"keepLocalCache,omitempty"`
}

// ProfileCloudStatus surfaces current cloud sync / lease state in the dashboard.
type ProfileCloudStatus struct {
	State          string    `json:"state,omitempty"`
	LeaseMachine   string    `json:"leaseMachine,omitempty"`
	LeaseUser      string    `json:"leaseUser,omitempty"`
	LeaseExpiresAt time.Time `json:"leaseExpiresAt,omitempty"`
	RemoteVersion  string    `json:"remoteVersion,omitempty"`
	LocalVersion   string    `json:"localVersion,omitempty"`
	LastSyncAt     time.Time `json:"lastSyncAt,omitempty"`
	Progress       int       `json:"progress,omitempty"`
	Message        string    `json:"message,omitempty"`
}

// ProfileBackend describes which browser runtime backs a profile.
type ProfileBackend struct {
	Kind     string                  `json:"kind,omitempty"`
	Cloak    *ProfileBackendCloak    `json:"cloak,omitempty"`
	Steel    *ProfileBackendSteel    `json:"steel,omitempty"`
	PinchTab *ProfileBackendPinchTab `json:"pinchtab,omitempty"`
}

// Profile represents a browser profile stored on disk.
// Matches internal/bridge/api.go ProfileInfo
type Profile struct {
	ID                string              `json:"id,omitempty"`
	Name              string              `json:"name"`
	Path              string              `json:"path,omitempty"`
	PathExists        bool                `json:"pathExists,omitempty"`
	Created           time.Time           `json:"created"`
	LastUsed          time.Time           `json:"lastUsed"`
	DiskUsage         int64               `json:"diskUsage"`
	SizeMB            float64             `json:"sizeMB,omitempty"`
	Running           bool                `json:"running"`
	Temporary         bool                `json:"temporary,omitempty"`
	Source            string              `json:"source,omitempty"`
	ChromeProfileName string              `json:"chromeProfileName,omitempty"`
	AccountEmail      string              `json:"accountEmail,omitempty"`
	AccountName       string              `json:"accountName,omitempty"`
	HasAccount        bool                `json:"hasAccount,omitempty"`
	UseWhen           string              `json:"useWhen,omitempty"`
	Description       string              `json:"description,omitempty"`
	Backend           *ProfileBackend     `json:"backend,omitempty"`
	CloudStatus       *ProfileCloudStatus `json:"cloudStatus,omitempty"`
}

// Instance represents a running browser instance.
// Matches internal/bridge/api.go Instance
type Instance struct {
	ID          string    `json:"id"`
	ProfileID   string    `json:"profileId"`
	ProfileName string    `json:"profileName"`
	Port        string    `json:"port"` // Note: string not int
	URL         string    `json:"url,omitempty"`
	Headless    bool      `json:"headless"`
	Status      string    `json:"status"` // starting/running/stopping/stopped/error
	StartTime   time.Time `json:"startTime"`
	Error       string    `json:"error,omitempty"`
	Attached    bool      `json:"attached"` // True if attached rather than locally launched
	AttachType  string    `json:"attachType,omitempty"`
	CdpURL      string    `json:"cdpUrl,omitempty"` // CDP WebSocket URL (for CDP-attached instances)
}

// Agent represents a connected AI agent.
type Agent struct {
	ID           string    `json:"id"`
	Name         string    `json:"name,omitempty"`
	ConnectedAt  time.Time `json:"connectedAt"`
	LastActivity time.Time `json:"lastActivity,omitempty"`
	RequestCount int       `json:"requestCount"`
}

// AgentDetail returns agent-centric dashboard data for a single agent.
type AgentDetail struct {
	Agent  Agent           `json:"agent"`
	Events []ActivityEvent `json:"events"`
}

// ActivityEvent represents an action in the activity feed.
type ActivityEvent struct {
	ID        string                 `json:"id"`
	AgentID   string                 `json:"agentId"`
	Channel   string                 `json:"channel"` // "tool_call" or "progress"
	Type      string                 `json:"type"`    // navigate/snapshot/action/screenshot/other
	Method    string                 `json:"method"`
	Path      string                 `json:"path"`
	Message   string                 `json:"message,omitempty"`  // human-readable (progress channel)
	Progress  *int                   `json:"progress,omitempty"` // 0-100 numeric progress
	Total     *int                   `json:"total,omitempty"`    // total steps
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// ActivityLogEvent represents a queryable backend activity record.
type ActivityLogEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	Source      string    `json:"source"`
	RequestID   string    `json:"requestId,omitempty"`
	SessionID   string    `json:"sessionId,omitempty"`
	AgentID     string    `json:"agentId,omitempty"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	Status      int       `json:"status"`
	DurationMs  int64     `json:"durationMs"`
	RemoteAddr  string    `json:"remoteAddr,omitempty"`
	InstanceID  string    `json:"instanceId,omitempty"`
	ProfileID   string    `json:"profileId,omitempty"`
	ProfileName string    `json:"profileName,omitempty"`
	TabID       string    `json:"tabId,omitempty"`
	URL         string    `json:"url,omitempty"`
	Action      string    `json:"action,omitempty"`
	Engine      string    `json:"engine,omitempty"`
	Ref         string    `json:"ref,omitempty"`
}

// ActivityLogResponse is returned by the /api/activity endpoint.
type ActivityLogResponse struct {
	Events []ActivityLogEvent `json:"events"`
	Count  int                `json:"count"`
}

// ScreencastSettings configures live tab previews.
type ScreencastSettings struct {
	FPS      int `json:"fps"`
	Quality  int `json:"quality"`
	MaxWidth int `json:"maxWidth"`
}

// BrowserSettings configures browser behavior.
type BrowserSettings struct {
	BlockImages  bool `json:"blockImages"`
	BlockMedia   bool `json:"blockMedia"`
	NoAnimations bool `json:"noAnimations"`
}

// Settings contains all dashboard settings.
type Settings struct {
	Screencast ScreencastSettings `json:"screencast"`
	Stealth    string             `json:"stealth"` // light/medium/full
	Browser    BrowserSettings    `json:"browser"`
	Monitoring MonitoringSettings `json:"monitoring"`
	Agents     AgentSettings      `json:"agents"`
}

// AgentSettings controls agent reasoning output visibility.
type AgentSettings struct {
	ReasoningMode string `json:"reasoningMode"` // "tool_calls" (default), "progress", "both"
}

// MonitoringSettings controls dashboard monitoring features.
type MonitoringSettings struct {
	MemoryMetrics bool `json:"memoryMetrics"` // Enable per-tab memory aggregation (can be heavy)
	PollInterval  int  `json:"pollInterval"`  // Poll interval in seconds (default 30)
}

// ServerInfo contains health/status information.
type ServerInfo struct {
	Version   string `json:"version"`
	Uptime    int64  `json:"uptime"`
	Profiles  int    `json:"profiles"`
	Instances int    `json:"instances"`
	Agents    int    `json:"agents"`
}

// CreateProfileRequest is the request body for creating a profile.
type CreateProfileRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	UseWhen     string          `json:"useWhen,omitempty"`
	Backend     *ProfileBackend `json:"backend,omitempty"`
}

// CreateProfileResponse is returned after creating a profile.
type CreateProfileResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
	Name   string `json:"name"`
}

// InstanceTab represents a browser tab in an instance.
type InstanceTab struct {
	ID         string `json:"id"`
	InstanceID string `json:"instanceId"`
	URL        string `json:"url"`
	Title      string `json:"title"`
}

// InstanceMetrics represents memory metrics for an instance.
type InstanceMetrics struct {
	InstanceID    string  `json:"instanceId"`
	ProfileName   string  `json:"profileName"`
	JSHeapUsedMB  float64 `json:"jsHeapUsedMB"`
	JSHeapTotalMB float64 `json:"jsHeapTotalMB"`
	Documents     int64   `json:"documents"`
	Frames        int64   `json:"frames"`
	Nodes         int64   `json:"nodes"`
	Listeners     int64   `json:"listeners"`
}

// LaunchInstanceRequest is the request body for launching an instance.
type LaunchInstanceRequest struct {
	ProfileID      string   `json:"profileId,omitempty"`      // profile ID (prof_XXXXXXXX)
	Name           string   `json:"name,omitempty"`           // profile name
	Mode           string   `json:"mode,omitempty"`           // "headed" or empty for headless
	Port           string   `json:"port,omitempty"`           // port number as string
	ExtensionPaths []string `json:"extensionPaths,omitempty"` // Chrome extension paths to load
}
