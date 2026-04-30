package cloudprofiles

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/auth/credentials"
	"cloud.google.com/go/storage"
	"github.com/google/uuid"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	gcsapi "google.golang.org/api/storage/v1"
)

const (
	defaultProvider      = "gcs"
	defaultPrefix        = "pinchtab/profiles"
	defaultLeaseDuration = 10 * time.Minute
)

type remoteMeta struct {
	ProfileID           string    `json:"profileId"`
	Name                string    `json:"name"`
	UpdatedAt           time.Time `json:"updatedAt"`
	ProxyURL            string    `json:"proxyUrl,omitempty"`
	Timezone            string    `json:"timezone,omitempty"`
	Locale              string    `json:"locale,omitempty"`
	Binary              string    `json:"binary,omitempty"`
	BrowserVersion      string    `json:"browserVersion,omitempty"`
	FingerprintSeed     string    `json:"fingerprintSeed,omitempty"`
	FingerprintPlatform string    `json:"fingerprintPlatform,omitempty"`
	FingerprintTimezone string    `json:"fingerprintTimezone,omitempty"`
	StorageQuota        string    `json:"storageQuota,omitempty"`
	LaunchArgs          []string  `json:"launchArgs,omitempty"`
}

type latestVersion struct {
	VersionID string    `json:"versionId"`
	Hash      string    `json:"hash"`
	UpdatedAt time.Time `json:"updatedAt"`
	SizeBytes int64     `json:"sizeBytes"`
}

type leaseRecord struct {
	LeaseID    string    `json:"leaseId"`
	Machine    string    `json:"machine"`
	User       string    `json:"user"`
	Status     string    `json:"status"`
	AcquiredAt time.Time `json:"acquiredAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

type localState struct {
	LastLocalHash     string    `json:"lastLocalHash,omitempty"`
	LastSyncedHash    string    `json:"lastSyncedHash,omitempty"`
	LastSyncedAt      time.Time `json:"lastSyncedAt,omitempty"`
	LastSyncedVersion string    `json:"lastSyncedVersion,omitempty"`
	LastFinalizeState string    `json:"lastFinalizeState,omitempty"`
	LastError         string    `json:"lastError,omitempty"`
}

type Session struct {
	Config       *bridge.ProfileCloudConfig
	Lease        leaseRecord
	RemoteLatest latestVersion
	InitialHash  string
	stopRenew    func()
}

type DiscoveredProfile struct {
	ProfileID           string    `json:"profileId"`
	Name                string    `json:"name"`
	UpdatedAt           time.Time `json:"updatedAt"`
	ProxyURL            string    `json:"proxyUrl,omitempty"`
	Timezone            string    `json:"timezone,omitempty"`
	Locale              string    `json:"locale,omitempty"`
	Binary              string    `json:"binary,omitempty"`
	BrowserVersion      string    `json:"browserVersion,omitempty"`
	FingerprintSeed     string    `json:"fingerprintSeed,omitempty"`
	FingerprintPlatform string    `json:"fingerprintPlatform,omitempty"`
	FingerprintTimezone string    `json:"fingerprintTimezone,omitempty"`
	StorageQuota        string    `json:"storageQuota,omitempty"`
	LaunchArgs          []string  `json:"launchArgs,omitempty"`
}

type Client struct {
	cfg    *bridge.ProfileCloudConfig
	client *storage.Client
	bucket *storage.BucketHandle
}

func Enabled(cfg *bridge.ProfileCloudConfig) bool {
	return cfg != nil && cfg.Enabled != nil && *cfg.Enabled
}

func New(ctx context.Context, cfg *bridge.ProfileCloudConfig) (*Client, error) {
	if !Enabled(cfg) {
		return nil, fmt.Errorf("cloud profile disabled")
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("cloud profile bucket is required")
	}
	prefix := strings.TrimSpace(cfg.Prefix)
	if prefix == "" {
		prefix = defaultPrefix
		cfg.Prefix = prefix
	}
	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" {
		provider = defaultProvider
		cfg.Provider = provider
	}
	if provider != defaultProvider {
		return nil, fmt.Errorf("unsupported cloud provider %q", provider)
	}

	var opts []option.ClientOption
	if path := strings.TrimSpace(cfg.CredentialPath); path != "" {
		creds, err := credentials.DetectDefault(&credentials.DetectOptions{
			Scopes:          []string{gcsapi.DevstorageFullControlScope},
			CredentialsFile: path,
		})
		if err != nil {
			return nil, err
		}
		opts = append(opts, option.WithAuthCredentials(creds))
	}
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{
		cfg:    cfg,
		client: client,
		bucket: client.Bucket(cfg.Bucket),
	}, nil
}

func (c *Client) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

func EnsureProfileID(cfg *bridge.ProfileCloudConfig) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.ProfileID) == "" {
		cfg.ProfileID = "cp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	if strings.TrimSpace(cfg.Provider) == "" {
		cfg.Provider = defaultProvider
	}
	if strings.TrimSpace(cfg.Prefix) == "" {
		cfg.Prefix = defaultPrefix
	}
}

func normalizeConfig(cfg *bridge.ProfileCloudConfig) *bridge.ProfileCloudConfig {
	if cfg == nil {
		return nil
	}
	copy := *cfg
	copy.Provider = strings.TrimSpace(copy.Provider)
	if copy.Provider == "" {
		copy.Provider = defaultProvider
	}
	copy.Bucket = strings.TrimSpace(copy.Bucket)
	copy.Prefix = strings.Trim(strings.TrimSpace(copy.Prefix), "/")
	if copy.Prefix == "" {
		copy.Prefix = defaultPrefix
	}
	copy.ProfileID = strings.TrimSpace(copy.ProfileID)
	copy.CredentialPath = strings.TrimSpace(copy.CredentialPath)
	return &copy
}

func (c *Client) profileRoot() string {
	return strings.Trim(c.cfg.Prefix, "/") + "/" + c.cfg.ProfileID
}

func (c *Client) object(name string) *storage.ObjectHandle {
	return c.bucket.Object(c.profileRoot() + "/" + strings.TrimLeft(name, "/"))
}

func loadJSON[T any](ctx context.Context, obj *storage.ObjectHandle, out *T) (bool, int64, error) {
	r, err := obj.NewReader(ctx)
	if err != nil {
		if isNotFound(err) {
			return false, 0, nil
		}
		return false, 0, err
	}
	defer func() { _ = r.Close() }()
	if err := json.NewDecoder(r).Decode(out); err != nil {
		return false, 0, err
	}
	return true, r.Attrs.Generation, nil
}

func writeJSON(ctx context.Context, obj *storage.ObjectHandle, value any) error {
	w := obj.NewWriter(ctx)
	w.ContentType = "application/json"
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		_ = w.Close()
		return err
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

func isNotFound(err error) bool {
	var gerr *googleapi.Error
	return err != nil && (strings.Contains(err.Error(), "object doesn't exist") || strings.Contains(err.Error(), "storage: object doesn't exist") || (errors.As(err, &gerr) && gerr.Code == 404))
}

func machineLabel() string {
	host, err := os.Hostname()
	if err == nil && strings.TrimSpace(host) != "" {
		return host
	}
	return "unknown-machine"
}

func userLabel() string {
	u, err := user.Current()
	if err == nil && strings.TrimSpace(u.Username) != "" {
		return u.Username
	}
	return "unknown-user"
}

func settingsSnapshot(settings *bridge.ProfileBackendPinchTab) remoteMeta {
	if settings == nil {
		return remoteMeta{}
	}
	fingerprintSeed := launchArgValue(settings.LaunchArgs, "--fingerprint=")
	fingerprintPlatform := launchArgValue(settings.LaunchArgs, "--fingerprint-platform=")
	fingerprintTimezone := launchArgValue(settings.LaunchArgs, "--fingerprint-timezone=")
	storageQuota := launchArgValue(settings.LaunchArgs, "--fingerprint-storage-quota=")
	if fingerprintTimezone == "" {
		fingerprintTimezone = strings.TrimSpace(settings.Timezone)
	}
	return remoteMeta{
		ProxyURL:            strings.TrimSpace(settings.ProxyURL),
		Timezone:            strings.TrimSpace(settings.Timezone),
		Locale:              strings.TrimSpace(settings.Locale),
		Binary:              strings.TrimSpace(settings.Binary),
		BrowserVersion:      strings.TrimSpace(settings.BrowserVersion),
		FingerprintSeed:     fingerprintSeed,
		FingerprintPlatform: fingerprintPlatform,
		FingerprintTimezone: fingerprintTimezone,
		StorageQuota:        storageQuota,
		LaunchArgs:          append([]string(nil), settings.LaunchArgs...),
	}
}

func launchArgValue(args []string, prefix string) string {
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		}
	}
	return ""
}

func normalizeLaunchArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(args))
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func isCloakBackedPinchTab(settings *bridge.ProfileBackendPinchTab) bool {
	if settings == nil {
		return false
	}
	binary := strings.ToLower(strings.TrimSpace(settings.Binary))
	return strings.Contains(binary, ".cloakbrowser") && strings.Contains(binary, "chromium-")
}

func validateFrozenIdentity(settings *bridge.ProfileBackendPinchTab) error {
	if settings == nil || !isCloakBackedPinchTab(settings) {
		return nil
	}
	if strings.TrimSpace(settings.ProxyURL) == "" {
		return fmt.Errorf("cloud-shared cloak profile requires proxyUrl")
	}
	if strings.TrimSpace(settings.Timezone) == "" {
		return fmt.Errorf("cloud-shared cloak profile requires timezone")
	}
	if strings.TrimSpace(settings.Locale) == "" {
		return fmt.Errorf("cloud-shared cloak profile requires locale")
	}
	if strings.TrimSpace(settings.Binary) == "" {
		return fmt.Errorf("cloud-shared cloak profile requires binary")
	}
	if strings.TrimSpace(settings.BrowserVersion) == "" {
		return fmt.Errorf("cloud-shared cloak profile requires browserVersion")
	}
	if launchArgValue(settings.LaunchArgs, "--fingerprint=") == "" {
		return fmt.Errorf("cloud-shared cloak profile requires --fingerprint")
	}
	if launchArgValue(settings.LaunchArgs, "--fingerprint-platform=") == "" {
		return fmt.Errorf("cloud-shared cloak profile requires --fingerprint-platform")
	}
	if launchArgValue(settings.LaunchArgs, "--fingerprint-storage-quota=") == "" {
		return fmt.Errorf("cloud-shared cloak profile requires --fingerprint-storage-quota")
	}
	if launchArgValue(settings.LaunchArgs, "--fingerprint-timezone=") == "" && strings.TrimSpace(settings.Timezone) == "" {
		return fmt.Errorf("cloud-shared cloak profile requires fingerprint timezone")
	}
	return nil
}

func defaultFingerprintPlatform() string {
	if goruntime.GOOS == "darwin" {
		return "macos"
	}
	return "windows"
}

func deterministicFingerprintSeed(source string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		trimmed = "pinchtab"
	}
	sum := sha256.Sum256([]byte(trimmed))
	n := int(sum[0])<<8 | int(sum[1])
	return fmt.Sprintf("%d", 10000+(n%90000))
}

func upsertLaunchArg(args []string, prefix, value string) []string {
	if strings.TrimSpace(value) == "" {
		return normalizeLaunchArgs(args)
	}
	next := make([]string, 0, len(args)+1)
	replaced := false
	for _, arg := range normalizeLaunchArgs(args) {
		if strings.HasPrefix(arg, prefix) {
			if !replaced {
				next = append(next, prefix+value)
				replaced = true
			}
			continue
		}
		next = append(next, arg)
	}
	if !replaced {
		next = append(next, prefix+value)
	}
	return next
}

func applyFrozenIdentityDefaults(cfg *bridge.ProfileCloudConfig, settings *bridge.ProfileBackendPinchTab) *bridge.ProfileBackendPinchTab {
	if settings == nil {
		return nil
	}
	copy := *settings
	copy.LaunchArgs = append([]string(nil), settings.LaunchArgs...)
	if !isCloakBackedPinchTab(&copy) {
		return &copy
	}
	if strings.TrimSpace(copy.BrowserVersion) == "" && strings.TrimSpace(copy.Binary) != "" {
		copy.BrowserVersion = inferBrowserVersionFromBinaryPath(copy.Binary)
	}
	if strings.TrimSpace(copy.Locale) == "" {
		if lang := launchArgValue(copy.LaunchArgs, "--lang="); lang != "" {
			copy.Locale = lang
		} else if locale := launchArgValue(copy.LaunchArgs, "--fingerprint-locale="); locale != "" {
			copy.Locale = locale
		} else {
			copy.Locale = "en-US"
		}
	}
	if strings.TrimSpace(copy.Timezone) == "" {
		copy.Timezone = launchArgValue(copy.LaunchArgs, "--fingerprint-timezone=")
	}
	seed := launchArgValue(copy.LaunchArgs, "--fingerprint=")
	if seed == "" {
		seed = deterministicFingerprintSeed(strings.TrimSpace(cfg.ProfileID) + "|" + strings.TrimSpace(copy.ProxyURL) + "|" + strings.TrimSpace(copy.Binary))
	}
	copy.LaunchArgs = upsertLaunchArg(copy.LaunchArgs, "--fingerprint=", seed)

	platform := launchArgValue(copy.LaunchArgs, "--fingerprint-platform=")
	if platform == "" {
		platform = defaultFingerprintPlatform()
	}
	copy.LaunchArgs = upsertLaunchArg(copy.LaunchArgs, "--fingerprint-platform=", platform)

	quota := launchArgValue(copy.LaunchArgs, "--fingerprint-storage-quota=")
	if quota == "" {
		quota = "10000"
	}
	copy.LaunchArgs = upsertLaunchArg(copy.LaunchArgs, "--fingerprint-storage-quota=", quota)

	if strings.TrimSpace(copy.Timezone) != "" {
		copy.LaunchArgs = upsertLaunchArg(copy.LaunchArgs, "--fingerprint-timezone=", strings.TrimSpace(copy.Timezone))
	}
	if strings.TrimSpace(copy.Locale) != "" {
		copy.LaunchArgs = upsertLaunchArg(copy.LaunchArgs, "--lang=", strings.TrimSpace(copy.Locale))
		copy.LaunchArgs = upsertLaunchArg(copy.LaunchArgs, "--fingerprint-locale=", strings.TrimSpace(copy.Locale))
	}
	return &copy
}

func mergeFrozenMeta(existing *remoteMeta, next remoteMeta) remoteMeta {
	if existing == nil {
		return next
	}
	merged := next
	if strings.TrimSpace(existing.ProxyURL) != "" {
		merged.ProxyURL = strings.TrimSpace(existing.ProxyURL)
	}
	if strings.TrimSpace(existing.Timezone) != "" {
		merged.Timezone = strings.TrimSpace(existing.Timezone)
	}
	if strings.TrimSpace(existing.Locale) != "" {
		merged.Locale = strings.TrimSpace(existing.Locale)
	}
	if strings.TrimSpace(existing.Binary) != "" {
		merged.Binary = strings.TrimSpace(existing.Binary)
	}
	if strings.TrimSpace(existing.BrowserVersion) != "" {
		merged.BrowserVersion = strings.TrimSpace(existing.BrowserVersion)
	}
	if strings.TrimSpace(existing.FingerprintSeed) != "" {
		merged.FingerprintSeed = strings.TrimSpace(existing.FingerprintSeed)
	}
	if strings.TrimSpace(existing.FingerprintPlatform) != "" {
		merged.FingerprintPlatform = strings.TrimSpace(existing.FingerprintPlatform)
	}
	if strings.TrimSpace(existing.FingerprintTimezone) != "" {
		merged.FingerprintTimezone = strings.TrimSpace(existing.FingerprintTimezone)
	}
	if strings.TrimSpace(existing.StorageQuota) != "" {
		merged.StorageQuota = strings.TrimSpace(existing.StorageQuota)
	}
	if len(existing.LaunchArgs) > 0 {
		merged.LaunchArgs = append([]string(nil), existing.LaunchArgs...)
	}
	return merged
}

func metaToSettings(meta remoteMeta, fallback *bridge.ProfileBackendPinchTab) *bridge.ProfileBackendPinchTab {
	var resolved bridge.ProfileBackendPinchTab
	if fallback != nil {
		resolved = *fallback
		resolved.LaunchArgs = append([]string(nil), fallback.LaunchArgs...)
	}
	if strings.TrimSpace(meta.ProxyURL) != "" {
		resolved.ProxyURL = strings.TrimSpace(meta.ProxyURL)
	}
	if strings.TrimSpace(meta.Timezone) != "" {
		resolved.Timezone = strings.TrimSpace(meta.Timezone)
	}
	if strings.TrimSpace(meta.Locale) != "" {
		resolved.Locale = strings.TrimSpace(meta.Locale)
	}
	if strings.TrimSpace(meta.Binary) != "" {
		resolved.Binary = strings.TrimSpace(meta.Binary)
	}
	if strings.TrimSpace(meta.BrowserVersion) != "" {
		resolved.BrowserVersion = strings.TrimSpace(meta.BrowserVersion)
	}
	if len(meta.LaunchArgs) > 0 {
		resolved.LaunchArgs = append([]string(nil), meta.LaunchArgs...)
	}
	return &resolved
}

func ResolveEffectiveSettings(ctx context.Context, cfg *bridge.ProfileCloudConfig, settings *bridge.ProfileBackendPinchTab) (*bridge.ProfileBackendPinchTab, error) {
	cfg = normalizeConfig(cfg)
	resolved := applyFrozenIdentityDefaults(cfg, metaToSettings(remoteMeta{}, settings))
	if !Enabled(cfg) {
		return resolved, validateFrozenIdentity(resolved)
	}
	client, err := New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	var meta remoteMeta
	if ok, _, err := loadJSON(ctx, client.object("meta.json"), &meta); err != nil {
		return nil, err
	} else if ok {
		resolved = applyFrozenIdentityDefaults(cfg, metaToSettings(meta, resolved))
	}
	if resolved.BrowserVersion == "" && strings.TrimSpace(resolved.Binary) != "" {
		resolved.BrowserVersion = inferBrowserVersionFromBinaryPath(resolved.Binary)
	}
	return resolved, validateFrozenIdentity(resolved)
}

var cloakBinaryVersionPattern = regexp.MustCompile(`(?:^|[/\\])chromium-(\d+\.\d+\.\d+\.\d+)(?:\.\d+)?(?:[/\\]|$)`)

func inferBrowserVersionFromBinaryPath(binary string) string {
	matches := cloakBinaryVersionPattern.FindStringSubmatch(strings.TrimSpace(binary))
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

func (c *Client) ensureMeta(ctx context.Context, profileName string, settings *bridge.ProfileBackendPinchTab) error {
	settings = applyFrozenIdentityDefaults(c.cfg, settings)
	snapshot := settingsSnapshot(settings)
	if err := validateFrozenIdentity(settings); err != nil {
		return err
	}
	meta := remoteMeta{
		ProfileID:           c.cfg.ProfileID,
		Name:                profileName,
		UpdatedAt:           time.Now().UTC(),
		ProxyURL:            snapshot.ProxyURL,
		Timezone:            snapshot.Timezone,
		Locale:              snapshot.Locale,
		Binary:              snapshot.Binary,
		BrowserVersion:      snapshot.BrowserVersion,
		FingerprintSeed:     snapshot.FingerprintSeed,
		FingerprintPlatform: snapshot.FingerprintPlatform,
		FingerprintTimezone: snapshot.FingerprintTimezone,
		StorageQuota:        snapshot.StorageQuota,
		LaunchArgs:          snapshot.LaunchArgs,
	}
	var existing remoteMeta
	if ok, _, err := loadJSON(ctx, c.object("meta.json"), &existing); err == nil && ok {
		meta = mergeFrozenMeta(&existing, meta)
	}
	return writeJSON(ctx, c.object("meta.json"), meta)
}

func (c *Client) AcquireLease(ctx context.Context, profileName string, settings *bridge.ProfileBackendPinchTab) (leaseRecord, error) {
	_ = c.ensureMeta(ctx, profileName, settings)
	obj := c.object("lease.json")
	var current leaseRecord
	exists, gen, err := loadJSON(ctx, obj, &current)
	if err != nil {
		return leaseRecord{}, err
	}
	now := time.Now().UTC()
	if exists && current.ExpiresAt.After(now) {
		return leaseRecord{}, fmt.Errorf("profile is in use by %s on %s until %s", current.User, current.Machine, current.ExpiresAt.Format(time.RFC3339))
	}
	next := leaseRecord{
		LeaseID:    uuid.NewString(),
		Machine:    machineLabel(),
		User:       userLabel(),
		Status:     "running-locally",
		AcquiredAt: now,
		UpdatedAt:  now,
		ExpiresAt:  now.Add(defaultLeaseDuration),
	}
	var target *storage.ObjectHandle
	if exists {
		target = obj.If(storage.Conditions{GenerationMatch: gen})
	} else {
		target = obj.If(storage.Conditions{DoesNotExist: true})
	}
	if err := writeJSON(ctx, target, next); err != nil {
		return leaseRecord{}, err
	}
	return next, nil
}

func (c *Client) ReleaseLease(ctx context.Context, leaseID string) error {
	obj := c.object("lease.json")
	var current leaseRecord
	exists, _, err := loadJSON(ctx, obj, &current)
	if err != nil || !exists {
		return err
	}
	if current.LeaseID != leaseID {
		return nil
	}
	return obj.Delete(ctx)
}

func (c *Client) RenewLease(ctx context.Context, leaseID string) error {
	obj := c.object("lease.json")
	var current leaseRecord
	exists, _, err := loadJSON(ctx, obj, &current)
	if err != nil || !exists {
		return err
	}
	if current.LeaseID != leaseID {
		return nil
	}
	now := time.Now().UTC()
	current.UpdatedAt = now
	current.ExpiresAt = now.Add(defaultLeaseDuration)
	return writeJSON(ctx, obj, current)
}

func localStatePath(profilePath string) string {
	return filepath.Join(profilePath, ".pinchtab-cloud-state.json")
}

func readLocalState(profilePath string) localState {
	var st localState
	data, err := os.ReadFile(localStatePath(profilePath))
	if err != nil {
		return st
	}
	_ = json.Unmarshal(data, &st)
	return st
}

func writeLocalState(profilePath string, st localState) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(localStatePath(profilePath), data, 0644)
}

func (c *Client) latest(ctx context.Context) (latestVersion, bool, error) {
	var latest latestVersion
	ok, _, err := loadJSON(ctx, c.object("latest.json"), &latest)
	return latest, ok, err
}

func (c *Client) currentLease(ctx context.Context) (leaseRecord, bool, error) {
	var lease leaseRecord
	ok, _, err := loadJSON(ctx, c.object("lease.json"), &lease)
	return lease, ok, err
}

func excludedPath(rel string) bool {
	rel = filepath.ToSlash(rel)
	if rel == "profile.json" || rel == ".pinchtab-cloud-state.json" {
		return true
	}
	if strings.HasPrefix(rel, ".pinchtab-state/") || rel == ".pinchtab-state" {
		return true
	}
	base := filepath.Base(rel)
	return strings.HasPrefix(base, "Singleton")
}

var portableSyncDirPrefixes = []string{
	"Default/Local Storage",
	"Default/IndexedDB",
	"Default/Session Storage",
	"Default/Extensions",
	"Default/Extension State",
	"Default/Local Extension Settings",
	"Default/Sync Extension Settings",
	"Default/Managed Extension Settings",
}

var portableSyncExactFiles = []string{
	"Default/Bookmarks",
	"Default/Cookies",
	"Default/History",
	"Default/History-journal",
	"Default/Favicons",
	"Default/Favicons-journal",
	"Default/Visited Links",
	"Default/Preferences",
	"Default/Secure Preferences",
	"Default/Login Data",
	"Default/Login Data For Account",
	"Default/Login Data-journal",
	"Default/Login Data For Account-journal",
	"Default/Network/Cookies",
	"Default/Network/Cookies-journal",
}

func includedPortablePath(rel string) bool {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" || excludedPath(rel) {
		return false
	}
	for _, exact := range portableSyncExactFiles {
		if rel == exact {
			return true
		}
	}
	for _, prefix := range portableSyncDirPrefixes {
		if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
			return true
		}
	}
	return false
}

func hashDirectory(root string) (string, error) {
	h := sha256.New()
	var files []string
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if excludedPath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			relSlash := filepath.ToSlash(rel)
			for _, prefix := range portableSyncDirPrefixes {
				if relSlash == prefix || strings.HasPrefix(prefix, relSlash+"/") {
					return nil
				}
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !includedPortablePath(rel) {
			return nil
		}
		files = append(files, rel)
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(files)
	buf := make([]byte, 32*1024)
	for _, rel := range files {
		path := filepath.Join(root, rel)
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		_, _ = h.Write([]byte(filepath.ToSlash(rel)))
		_, _ = h.Write([]byte(info.Mode().String()))
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		for {
			n, err := f.Read(buf)
			if n > 0 {
				_, _ = h.Write(buf[:n])
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				_ = f.Close()
				return "", err
			}
		}
		_ = f.Close()
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func clearPortableState(profilePath string) error {
	candidates := append([]string{}, portableSyncExactFiles...)
	candidates = append(candidates, portableSyncDirPrefixes...)
	for _, rel := range candidates {
		target := filepath.Join(profilePath, filepath.FromSlash(rel))
		if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func archiveDirectory(root string) (string, int64, string, error) {
	tmpFile, err := os.CreateTemp("", "pinchtab-cloud-*.tar.gz")
	if err != nil {
		return "", 0, "", err
	}
	defer func() { _ = tmpFile.Close() }()

	gzw := gzip.NewWriter(tmpFile)
	tw := tar.NewWriter(gzw)
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if excludedPath(rel) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			relSlash := filepath.ToSlash(rel)
			includeDir := false
			for _, prefix := range portableSyncDirPrefixes {
				if relSlash == prefix || strings.HasPrefix(prefix, relSlash+"/") {
					includeDir = true
					break
				}
			}
			if !includeDir {
				return nil
			}
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if !info.IsDir() && !includedPortablePath(rel) {
			return nil
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = io.Copy(tw, f)
		return err
	}); err != nil {
		_ = tw.Close()
		_ = gzw.Close()
		return "", 0, "", err
	}
	if err := tw.Close(); err != nil {
		_ = gzw.Close()
		return "", 0, "", err
	}
	if err := gzw.Close(); err != nil {
		return "", 0, "", err
	}
	info, err := os.Stat(tmpFile.Name())
	if err != nil {
		return "", 0, "", err
	}
	hash, err := fileHash(tmpFile.Name())
	if err != nil {
		return "", 0, "", err
	}
	return tmpFile.Name(), info.Size(), hash, nil
}

func extractArchive(archivePath, dest string) error {
	if err := clearPortableState(dest); err != nil {
		return err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gzr.Close() }()
	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.Clean(hdr.Name))
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) && filepath.Clean(target) != filepath.Clean(dest) {
			return fmt.Errorf("invalid archive path %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			_ = out.Close()
		}
	}
	return nil
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func randomVersionID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err == nil {
		return time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(buf)
	}
	return time.Now().UTC().Format("20060102T150405Z")
}

func startLeaseRenewLoop(cfg *bridge.ProfileCloudConfig, leaseID string) func() {
	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				client, err := New(ctx, cfg)
				if err == nil {
					_ = client.RenewLease(ctx, leaseID)
					_ = client.Close()
				}
				cancel()
			case <-stopCh:
				return
			}
		}
	}()
	return func() {
		select {
		case <-stopCh:
		default:
			close(stopCh)
		}
	}
}

func (c *Client) downloadVersion(ctx context.Context, versionID, destArchive string) error {
	r, err := c.object("versions/" + versionID + ".tar.gz").NewReader(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	out, err := os.Create(destArchive)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, r)
	return err
}

func (c *Client) uploadVersion(ctx context.Context, archivePath, versionID string) (int64, string, error) {
	obj := c.object("versions/" + versionID + ".tar.gz")
	w := obj.NewWriter(ctx)
	w.ContentType = "application/gzip"
	file, err := os.Open(archivePath)
	if err != nil {
		_ = w.Close()
		return 0, "", err
	}
	defer func() { _ = file.Close() }()
	n, err := io.Copy(w, file)
	if err != nil {
		_ = w.Close()
		return 0, "", err
	}
	if err := w.Close(); err != nil {
		return 0, "", err
	}
	hash, err := fileHash(archivePath)
	if err != nil {
		return 0, "", err
	}
	return n, hash, nil
}

func Prepare(ctx context.Context, profileName, profilePath string, cfg *bridge.ProfileCloudConfig, settings *bridge.ProfileBackendPinchTab) (*Session, error) {
	cfg = normalizeConfig(cfg)
	client, err := New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	if err := os.MkdirAll(profilePath, 0755); err != nil {
		return nil, err
	}
	lease, err := client.AcquireLease(ctx, profileName, settings)
	if err != nil {
		return nil, err
	}
	latest, hasLatest, err := client.latest(ctx)
	if err != nil {
		_ = client.ReleaseLease(ctx, lease.LeaseID)
		return nil, err
	}
	state := readLocalState(profilePath)
	if hasLatest && state.LastSyncedVersion != latest.VersionID {
		tmpArchive := filepath.Join(os.TempDir(), "pinchtab-cloud-"+cfg.ProfileID+".tar.gz")
		if err := client.downloadVersion(ctx, latest.VersionID, tmpArchive); err != nil {
			_ = client.ReleaseLease(ctx, lease.LeaseID)
			return nil, err
		}
		if err := extractArchive(tmpArchive, profilePath); err != nil {
			_ = os.Remove(tmpArchive)
			_ = client.ReleaseLease(ctx, lease.LeaseID)
			return nil, err
		}
		_ = os.Remove(tmpArchive)
		state.LastSyncedVersion = latest.VersionID
		state.LastSyncedHash = latest.Hash
		state.LastSyncedAt = latest.UpdatedAt
		state.LastError = ""
	}
	if err := os.MkdirAll(filepath.Join(profilePath, "Default"), 0755); err != nil {
		_ = client.ReleaseLease(ctx, lease.LeaseID)
		return nil, err
	}
	localHash, err := hashDirectory(profilePath)
	if err != nil {
		_ = client.ReleaseLease(ctx, lease.LeaseID)
		return nil, err
	}
	state.LastLocalHash = localHash
	if err := writeLocalState(profilePath, state); err != nil {
		_ = client.ReleaseLease(ctx, lease.LeaseID)
		return nil, err
	}
	stopRenew := startLeaseRenewLoop(cfg, lease.LeaseID)
	return &Session{
		Config:       cfg,
		Lease:        lease,
		RemoteLatest: latest,
		InitialHash:  localHash,
		stopRenew:    stopRenew,
	}, nil
}

func Finalize(ctx context.Context, profilePath string, session *Session) error {
	if session == nil || session.Config == nil {
		return nil
	}
	client, err := New(ctx, session.Config)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	if session.stopRenew != nil {
		session.stopRenew()
		session.stopRenew = nil
	}

	state := readLocalState(profilePath)
	currentHash, err := hashDirectory(profilePath)
	if err != nil {
		_ = client.ReleaseLease(ctx, session.Lease.LeaseID)
		return err
	}
	state.LastLocalHash = currentHash
	if currentHash != session.InitialHash || state.LastSyncedVersion == "" {
		archivePath, sizeBytes, archiveHash, err := archiveDirectory(profilePath)
		if err != nil {
			state.LastError = err.Error()
			_ = writeLocalState(profilePath, state)
			_ = client.ReleaseLease(ctx, session.Lease.LeaseID)
			return err
		}
		defer func() { _ = os.Remove(archivePath) }()
		versionID := randomVersionID()
		if _, _, err := client.uploadVersion(ctx, archivePath, versionID); err != nil {
			state.LastError = err.Error()
			_ = writeLocalState(profilePath, state)
			_ = client.ReleaseLease(ctx, session.Lease.LeaseID)
			return err
		}
		latest := latestVersion{
			VersionID: versionID,
			Hash:      archiveHash,
			UpdatedAt: time.Now().UTC(),
			SizeBytes: sizeBytes,
		}
		if err := writeJSON(ctx, client.object("latest.json"), latest); err != nil {
			state.LastError = err.Error()
			_ = writeLocalState(profilePath, state)
			_ = client.ReleaseLease(ctx, session.Lease.LeaseID)
			return err
		}
		state.LastSyncedVersion = latest.VersionID
		state.LastSyncedHash = latest.Hash
		state.LastSyncedAt = latest.UpdatedAt
		state.LastError = ""
	}
	if err := writeLocalState(profilePath, state); err != nil {
		_ = client.ReleaseLease(ctx, session.Lease.LeaseID)
		return err
	}
	if err := client.ReleaseLease(ctx, session.Lease.LeaseID); err != nil {
		return err
	}
	if session.Config.KeepLocalCache != nil && !*session.Config.KeepLocalCache {
		if err := clearPortableState(profilePath); err != nil {
			return err
		}
	}
	return nil
}

func Release(ctx context.Context, session *Session) error {
	if session == nil || session.Config == nil {
		return nil
	}
	client, err := New(ctx, session.Config)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	if session.stopRenew != nil {
		session.stopRenew()
		session.stopRenew = nil
	}
	return client.ReleaseLease(ctx, session.Lease.LeaseID)
}

func Status(ctx context.Context, profilePath string, cfg *bridge.ProfileCloudConfig) (*bridge.ProfileCloudStatus, error) {
	cfg = normalizeConfig(cfg)
	if !Enabled(cfg) {
		return nil, nil
	}
	if finalizeStatus, err := GetFinalizeStatus(ctx, profilePath, cfg); err == nil && finalizeStatus != nil {
		switch finalizeStatus.State {
		case "queued", "archiving", "uploading", "updating-latest", "releasing-lease":
			status := &bridge.ProfileCloudStatus{
				State:         "stopped-uploading",
				RemoteVersion: finalizeStatus.RemoteVersion,
				LocalVersion:  finalizeStatus.LocalVersion,
				Progress:      finalizeStatus.Progress,
				Message:       finalizeStatus.State,
			}
			return status, nil
		case "error":
			status := &bridge.ProfileCloudStatus{
				State:         "upload-failed",
				RemoteVersion: finalizeStatus.RemoteVersion,
				LocalVersion:  finalizeStatus.LocalVersion,
				Message:       finalizeStatus.Error,
			}
			return status, nil
		}
	}
	if syncStatus, err := GetSyncStatus(ctx, profilePath, cfg); err == nil && syncStatus != nil {
		switch syncStatus.State {
		case "queued", "checking", "downloading", "extracting", "ready", "error":
			status := &bridge.ProfileCloudStatus{
				State:         syncStatus.State,
				RemoteVersion: syncStatus.RemoteVersion,
				LocalVersion:  syncStatus.LocalVersion,
				Message:       syncStatus.Error,
			}
			return status, nil
		}
	}
	state := readLocalState(profilePath)
	status := &bridge.ProfileCloudStatus{
		LocalVersion: state.LastSyncedVersion,
		LastSyncAt:   state.LastSyncedAt,
		Message:      state.LastError,
		State:        "available",
	}
	if state.LastFinalizeState == "upload-failed" {
		status.State = "upload-failed"
	}
	client, err := New(ctx, cfg)
	if err != nil {
		status.State = "error"
		status.Message = err.Error()
		return status, nil
	}
	defer func() { _ = client.Close() }()
	if latest, ok, err := client.latest(ctx); err == nil && ok {
		status.RemoteVersion = latest.VersionID
		if status.LocalVersion != "" && status.LocalVersion != latest.VersionID {
			status.State = "sync-required"
		}
	}
	var lease leaseRecord
	if ok, _, err := loadJSON(ctx, client.object("lease.json"), &lease); err == nil && ok {
		now := time.Now().UTC()
		status.LeaseMachine = lease.Machine
		status.LeaseUser = lease.User
		status.LeaseExpiresAt = lease.ExpiresAt
		if lease.ExpiresAt.After(now) {
			status.State = "in-use"
		} else {
			status.State = "lease-expired"
		}
	}
	if state.LastError != "" {
		status.State = "error"
	}
	return status, nil
}

func LocalStatus(profilePath string, cfg *bridge.ProfileCloudConfig) *bridge.ProfileCloudStatus {
	cfg = normalizeConfig(cfg)
	if !Enabled(cfg) {
		return nil
	}

	key := finalizeKey(profilePath, cfg)
	finalizeMu.Lock()
	if existing := finalizeJobs[key]; existing != nil {
		status := existing.status
		finalizeMu.Unlock()
		switch status.State {
		case "queued", "archiving", "uploading", "updating-latest", "releasing-lease":
			return &bridge.ProfileCloudStatus{
				State:         "stopped-uploading",
				RemoteVersion: status.RemoteVersion,
				LocalVersion:  status.LocalVersion,
				Progress:      status.Progress,
				Message:       status.State,
			}
		case "error":
			return &bridge.ProfileCloudStatus{
				State:         "upload-failed",
				RemoteVersion: status.RemoteVersion,
				LocalVersion:  status.LocalVersion,
				Message:       status.Error,
			}
		}
	} else {
		finalizeMu.Unlock()
	}

	skey := syncKey(profilePath, cfg)
	syncMu.Lock()
	if existing := syncJobs[skey]; existing != nil {
		status := existing.status
		syncMu.Unlock()
		return &bridge.ProfileCloudStatus{
			State:         status.State,
			RemoteVersion: status.RemoteVersion,
			LocalVersion:  status.LocalVersion,
			Message:       status.Error,
		}
	}
	syncMu.Unlock()

	state := readLocalState(profilePath)
	status := &bridge.ProfileCloudStatus{
		LocalVersion: state.LastSyncedVersion,
		LastSyncAt:   state.LastSyncedAt,
		Message:      state.LastError,
		State:        "available",
	}
	if state.LastFinalizeState == "upload-failed" {
		status.State = "upload-failed"
	}
	if state.LastError != "" {
		status.State = "error"
	}
	if state.LastSyncedVersion != "" {
		status.State = "cached"
	}
	return status
}

func Discover(ctx context.Context, cfg *bridge.ProfileCloudConfig) ([]DiscoveredProfile, error) {
	cfg = normalizeConfig(cfg)
	client, err := New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()

	prefix := strings.Trim(cfg.Prefix, "/") + "/"
	it := client.bucket.Objects(ctx, &storage.Query{Prefix: prefix})
	var profiles []DiscoveredProfile
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		if !strings.HasSuffix(attrs.Name, "/meta.json") {
			continue
		}
		obj := client.bucket.Object(attrs.Name)
		var meta remoteMeta
		ok, _, err := loadJSON(ctx, obj, &meta)
		if err != nil || !ok {
			if err != nil {
				return nil, err
			}
			continue
		}
		profiles = append(profiles, DiscoveredProfile{
			ProfileID:           meta.ProfileID,
			Name:                meta.Name,
			UpdatedAt:           meta.UpdatedAt,
			ProxyURL:            meta.ProxyURL,
			Timezone:            meta.Timezone,
			Locale:              meta.Locale,
			Binary:              meta.Binary,
			BrowserVersion:      meta.BrowserVersion,
			FingerprintSeed:     meta.FingerprintSeed,
			FingerprintPlatform: meta.FingerprintPlatform,
			FingerprintTimezone: meta.FingerprintTimezone,
			StorageQuota:        meta.StorageQuota,
			LaunchArgs:          append([]string(nil), meta.LaunchArgs...),
		})
	}
	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].UpdatedAt.Equal(profiles[j].UpdatedAt) {
			return profiles[i].Name < profiles[j].Name
		}
		return profiles[i].UpdatedAt.After(profiles[j].UpdatedAt)
	})
	return profiles, nil
}
