package profiles

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/cloudprofiles"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

func encodeProfileStringList(items []string) string {
	return strings.Join(normalizeStringList(items), ",")
}

func decodeProfileRequest(r *http.Request, v any) (map[string]json.RawMessage, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func jsonObjectField(raw map[string]json.RawMessage, key string) map[string]json.RawMessage {
	if raw == nil {
		return nil
	}
	value, ok := raw[key]
	if !ok {
		return nil
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(value, &out); err != nil {
		return nil
	}
	return out
}

func hasJSONField(raw map[string]json.RawMessage, key string) bool {
	if raw == nil {
		return false
	}
	_, ok := raw[key]
	return ok
}

func appendBackendUpdates(updates map[string]string, backend *bridge.ProfileBackend, raw map[string]json.RawMessage) {
	if backend == nil {
		return
	}
	backendRaw := jsonObjectField(raw, "backend")
	if hasJSONField(backendRaw, "kind") {
		updates["backend.kind"] = backend.Kind
	}
	if backend.Cloak != nil {
		cloakRaw := jsonObjectField(backendRaw, "cloak")
		if hasJSONField(cloakRaw, "baseUrl") {
			updates["backend.cloak.baseUrl"] = backend.Cloak.BaseURL
		}
		if hasJSONField(cloakRaw, "profileId") && strings.TrimSpace(backend.Cloak.ProfileID) != "" {
			updates["backend.cloak.profileId"] = backend.Cloak.ProfileID
		}
		if hasJSONField(cloakRaw, "proxyUrl") {
			updates["backend.cloak.proxyUrl"] = backend.Cloak.ProxyURL
		}
		if hasJSONField(cloakRaw, "timezone") {
			updates["backend.cloak.timezone"] = backend.Cloak.Timezone
		}
		if hasJSONField(cloakRaw, "locale") {
			updates["backend.cloak.locale"] = backend.Cloak.Locale
		}
		if hasJSONField(cloakRaw, "platform") {
			updates["backend.cloak.platform"] = backend.Cloak.Platform
		}
		if hasJSONField(cloakRaw, "userAgent") {
			updates["backend.cloak.userAgent"] = backend.Cloak.UserAgent
		}
		if hasJSONField(cloakRaw, "launchArgs") {
			updates["backend.cloak.launchArgs"] = encodeProfileStringList(backend.Cloak.LaunchArgs)
		}
		if hasJSONField(cloakRaw, "notes") {
			updates["backend.cloak.notes"] = backend.Cloak.Notes
		}
		if hasJSONField(cloakRaw, "headless") {
			updates["backend.cloak.headless"] = formatOptionalBool(backend.Cloak.Headless)
		}
		if hasJSONField(cloakRaw, "humanize") {
			updates["backend.cloak.humanize"] = formatOptionalBool(backend.Cloak.Humanize)
		}
		if hasJSONField(cloakRaw, "geoip") {
			updates["backend.cloak.geoip"] = formatOptionalBool(backend.Cloak.GeoIP)
		}
	}
	if backend.Steel != nil {
		steelRaw := jsonObjectField(backendRaw, "steel")
		if hasJSONField(steelRaw, "proxyUrl") {
			updates["backend.steel.proxyUrl"] = backend.Steel.ProxyURL
		}
		if hasJSONField(steelRaw, "extensionPaths") {
			updates["backend.steel.extensionPaths"] = encodeProfileStringList(backend.Steel.ExtensionPaths)
		}
	}
	if backend.PinchTab != nil {
		pinchTabRaw := jsonObjectField(backendRaw, "pinchtab")
		if hasJSONField(pinchTabRaw, "proxyUrl") {
			updates["backend.pinchtab.proxyUrl"] = backend.PinchTab.ProxyURL
		}
		if hasJSONField(pinchTabRaw, "timezone") {
			updates["backend.pinchtab.timezone"] = backend.PinchTab.Timezone
		}
		if hasJSONField(pinchTabRaw, "locale") {
			updates["backend.pinchtab.locale"] = backend.PinchTab.Locale
		}
		if hasJSONField(pinchTabRaw, "binary") {
			updates["backend.pinchtab.binary"] = backend.PinchTab.Binary
		}
		if hasJSONField(pinchTabRaw, "browserVersion") {
			updates["backend.pinchtab.browserVersion"] = backend.PinchTab.BrowserVersion
		}
		if hasJSONField(pinchTabRaw, "launchArgs") {
			updates["backend.pinchtab.launchArgs"] = encodeProfileStringList(backend.PinchTab.LaunchArgs)
		}
		if backend.PinchTab.Cloud != nil {
			cloudRaw := jsonObjectField(pinchTabRaw, "cloud")
			if hasJSONField(cloudRaw, "enabled") {
				updates["backend.pinchtab.cloud.enabled"] = formatOptionalBool(backend.PinchTab.Cloud.Enabled)
			}
			if hasJSONField(cloudRaw, "provider") {
				updates["backend.pinchtab.cloud.provider"] = backend.PinchTab.Cloud.Provider
			}
			if hasJSONField(cloudRaw, "bucket") {
				updates["backend.pinchtab.cloud.bucket"] = backend.PinchTab.Cloud.Bucket
			}
			if hasJSONField(cloudRaw, "prefix") {
				updates["backend.pinchtab.cloud.prefix"] = backend.PinchTab.Cloud.Prefix
			}
			if hasJSONField(cloudRaw, "profileId") {
				updates["backend.pinchtab.cloud.profileId"] = backend.PinchTab.Cloud.ProfileID
			}
			if hasJSONField(cloudRaw, "credentialPath") {
				updates["backend.pinchtab.cloud.credentialPath"] = backend.PinchTab.Cloud.CredentialPath
			}
			if hasJSONField(cloudRaw, "keepLocalCache") {
				updates["backend.pinchtab.cloud.keepLocalCache"] = formatOptionalBool(backend.PinchTab.Cloud.KeepLocalCache)
			}
		}
	}
}

func profileMutationStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case isProfileNameValidationError(err):
		return http.StatusBadRequest
	case strings.Contains(err.Error(), "already exists"):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func (pm *ProfileManager) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /profiles", pm.handleList)
	mux.HandleFunc("POST /profiles", pm.handleCreate)
	mux.HandleFunc("POST /profiles/create", pm.handleCreate)
	mux.HandleFunc("GET /profiles/{id}", pm.handleGetByID)
	mux.HandleFunc("POST /profiles/cloud/discover", pm.handleCloudDiscover)
	mux.HandleFunc("POST /profiles/cloud/import", pm.handleCloudImport)

	mux.HandleFunc("POST /profiles/export", pm.handleExport)
	mux.HandleFunc("POST /profiles/import", pm.handleImport)
	mux.HandleFunc("POST /profiles/import-config", pm.handleImportConfig)
	mux.HandleFunc("PATCH /profiles/meta", pm.handleUpdateMeta)
	mux.HandleFunc("POST /profiles/{id}/reset", pm.handleResetByIDOrName)
	mux.HandleFunc("GET /profiles/{id}/logs", pm.handleLogsByIDOrName)
	mux.HandleFunc("GET /profiles/{id}/analytics", pm.handleAnalyticsByIDOrName)
	mux.HandleFunc("DELETE /profiles/{id}", pm.handleDeleteByID)
	mux.HandleFunc("PATCH /profiles/{id}", pm.handleUpdateByID)
}

func (pm *ProfileManager) handleCloudDiscover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Bucket         string `json:"bucket"`
		Prefix         string `json:"prefix"`
		CredentialPath string `json:"credentialPath"`
	}
	if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), err)
		return
	}
	enabled := true
	results, err := cloudprofiles.Discover(r.Context(), &bridge.ProfileCloudConfig{
		Enabled:        &enabled,
		Provider:       "gcs",
		Bucket:         req.Bucket,
		Prefix:         req.Prefix,
		CredentialPath: req.CredentialPath,
	})
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"profiles": results})
}

func (pm *ProfileManager) handleCloudImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string `json:"name,omitempty"`
		Bucket         string `json:"bucket"`
		Prefix         string `json:"prefix"`
		CredentialPath string `json:"credentialPath"`
		ProfileID      string `json:"profileId"`
		KeepLocalCache *bool  `json:"keepLocalCache,omitempty"`
	}
	if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), err)
		return
	}
	enabled := true
	results, err := cloudprofiles.Discover(r.Context(), &bridge.ProfileCloudConfig{
		Enabled:        &enabled,
		Provider:       "gcs",
		Bucket:         req.Bucket,
		Prefix:         req.Prefix,
		CredentialPath: req.CredentialPath,
	})
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}
	var selected *cloudprofiles.DiscoveredProfile
	for i := range results {
		if strings.TrimSpace(results[i].ProfileID) == strings.TrimSpace(req.ProfileID) {
			selected = &results[i]
			break
		}
	}
	if selected == nil {
		httpx.Error(w, 404, fmt.Errorf("cloud profile %q not found", req.ProfileID))
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimSpace(selected.Name)
	}
	if name == "" {
		name = selected.ProfileID
	}
	keepLocal := true
	if req.KeepLocalCache != nil {
		keepLocal = *req.KeepLocalCache
	}
	meta := ProfileMeta{
		Name: name,
		Backend: &bridge.ProfileBackend{
			Kind: "pinchtab",
			PinchTab: &bridge.ProfileBackendPinchTab{
				ProxyURL:       selected.ProxyURL,
				Timezone:       selected.Timezone,
				Locale:         selected.Locale,
				Binary:         selected.Binary,
				BrowserVersion: selected.BrowserVersion,
				LaunchArgs:     append([]string(nil), selected.LaunchArgs...),
				Cloud: &bridge.ProfileCloudConfig{
					Enabled:        &enabled,
					Provider:       "gcs",
					Bucket:         req.Bucket,
					Prefix:         req.Prefix,
					ProfileID:      selected.ProfileID,
					CredentialPath: req.CredentialPath,
					KeepLocalCache: &keepLocal,
				},
			},
		},
	}
	if err := pm.CreateWithMeta(name, meta); err != nil {
		httpx.Error(w, profileMutationStatus(err), err)
		return
	}
	authn.AuditLog(r, "profile.cloud_imported", "profileName", name, "cloudProfileId", selected.ProfileID)
	httpx.JSON(w, 201, map[string]any{"status": "imported", "name": name})
}

func (pm *ProfileManager) handleList(w http.ResponseWriter, r *http.Request) {
	profiles, err := pm.List()
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}

	showAll := r.URL.Query().Get("all") == "true"
	if !showAll {
		filtered := []map[string]any{}
		for _, p := range profiles {
			if !p.Temporary {
				sizeMB := float64(p.DiskUsage) / (1024 * 1024)
				filtered = append(filtered, map[string]any{
					"id":                p.ID,
					"name":              p.Name,
					"path":              p.Path,
					"pathExists":        p.PathExists,
					"created":           p.Created,
					"lastUsed":          p.LastUsed,
					"diskUsage":         p.DiskUsage,
					"sizeMB":            sizeMB,
					"running":           p.Running,
					"source":            p.Source,
					"chromeProfileName": p.ChromeProfileName,
					"accountEmail":      p.AccountEmail,
					"accountName":       p.AccountName,
					"hasAccount":        p.HasAccount,
					"useWhen":           p.UseWhen,
					"description":       p.Description,
					"backend":           p.Backend,
					"cloudStatus":       p.CloudStatus,
				})
			}
		}
		httpx.JSON(w, 200, filtered)
		return
	}

	httpx.JSON(w, 200, profiles)
}

func (pm *ProfileManager) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		UseWhen     string                 `json:"useWhen"`
		Backend     *bridge.ProfileBackend `json:"backend"`
	}
	if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), err)
		return
	}
	if req.Name == "" {
		httpx.Error(w, 400, fmt.Errorf("name required"))
		return
	}

	meta := ProfileMeta{
		Description: req.Description,
		UseWhen:     req.UseWhen,
		Backend:     req.Backend,
	}

	if err := pm.CreateWithMeta(req.Name, meta); err != nil {
		httpx.Error(w, profileMutationStatus(err), err)
		return
	}

	generatedID := profileID(req.Name)
	authn.AuditLog(r, "profile.created", "profileId", generatedID, "profileName", req.Name)
	httpx.JSON(w, 200, map[string]any{
		"status": "created",
		"id":     generatedID,
		"name":   req.Name,
	})
}

func (pm *ProfileManager) handleImport(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.Error(w, 400, err)
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), err)
		return
	}
	if hasJSONField(raw, "profiles") || hasJSONField(raw, "bundle") {
		pm.handleImportConfigPayload(w, r, body)
		return
	}

	var req struct {
		Name        string                 `json:"name"`
		SourcePath  string                 `json:"sourcePath"`
		Description string                 `json:"description"`
		UseWhen     string                 `json:"useWhen"`
		Backend     *bridge.ProfileBackend `json:"backend"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), err)
		return
	}
	if req.Name == "" || req.SourcePath == "" {
		httpx.Error(w, 400, fmt.Errorf("name and sourcePath required"))
		return
	}

	meta := ProfileMeta{
		Description: req.Description,
		UseWhen:     req.UseWhen,
		Backend:     req.Backend,
	}

	if err := pm.ImportWithMeta(req.Name, req.SourcePath, meta); err != nil {
		httpx.Error(w, profileMutationStatus(err), err)
		return
	}
	authn.AuditLog(r, "profile.imported", "profileName", req.Name)
	httpx.JSON(w, 200, map[string]string{"status": "imported", "name": req.Name})
}

func (pm *ProfileManager) handleExport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs   []string `json:"ids"`
		Names []string `json:"names"`
	}
	if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), err)
		return
	}
	bundle, err := pm.ExportConfigBundle(req.IDs, req.Names)
	if err != nil {
		httpx.Error(w, profileMutationStatus(err), err)
		return
	}
	authn.AuditLog(r, "profile.exported", "count", len(bundle.Profiles))
	httpx.JSON(w, 200, bundle)
}

func (pm *ProfileManager) handleImportConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.Error(w, 400, err)
		return
	}
	pm.handleImportConfigPayload(w, r, body)
}

func (pm *ProfileManager) handleImportConfigPayload(w http.ResponseWriter, r *http.Request, body []byte) {
	var req struct {
		Bundle                  *ProfileConfigBundle  `json:"bundle"`
		Profiles                []ProfileConfigRecord `json:"profiles"`
		Overwrite               bool                  `json:"overwrite"`
		PreserveCloakProfileIDs bool                  `json:"preserveCloakProfileIds"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), err)
		return
	}

	bundle := ProfileConfigBundle{
		Version:    profileConfigBundleVersion,
		ExportedAt: time.Time{},
		Profiles:   req.Profiles,
	}
	if req.Bundle != nil {
		bundle = *req.Bundle
		if len(req.Profiles) > 0 && len(bundle.Profiles) == 0 {
			bundle.Profiles = req.Profiles
		}
	}
	if len(bundle.Profiles) == 0 {
		httpx.Error(w, 400, fmt.Errorf("profiles required"))
		return
	}

	imported, err := pm.ImportConfigBundle(bundle, ProfileConfigImportOptions{
		Overwrite:               req.Overwrite,
		PreserveCloakProfileIDs: req.PreserveCloakProfileIDs,
	})
	if err != nil {
		httpx.Error(w, profileMutationStatus(err), err)
		return
	}

	authn.AuditLog(r, "profile.config_imported", "count", len(imported))
	httpx.JSON(w, 200, map[string]any{
		"status":   "imported",
		"profiles": imported,
		"count":    len(imported),
	})
}

func (pm *ProfileManager) handleUpdateMeta(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string                 `json:"name"`
		Description *string                `json:"description"`
		UseWhen     *string                `json:"useWhen"`
		Backend     *bridge.ProfileBackend `json:"backend"`
	}
	raw, err := decodeProfileRequest(r, &req)
	if err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), err)
		return
	}
	if req.Name == "" {
		httpx.Error(w, 400, fmt.Errorf("name required"))
		return
	}

	updates := make(map[string]string)
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.UseWhen != nil {
		updates["useWhen"] = *req.UseWhen
	}
	appendBackendUpdates(updates, req.Backend, raw)

	if err := pm.UpdateMeta(req.Name, updates); err != nil {
		httpx.Error(w, profileMutationStatus(err), err)
		return
	}
	authn.AuditLog(r, "profile.meta_updated", "profileName", req.Name)
	httpx.JSON(w, 200, map[string]string{"status": "updated", "name": req.Name})
}

func (pm *ProfileManager) handleGetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	profiles, err := pm.List()
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}

	var foundProfile map[string]any

	for _, p := range profiles {
		if p.ID != id && p.Name != id {
			continue
		}
		foundProfile = map[string]any{
			"id":                p.ID,
			"name":              p.Name,
			"path":              p.Path,
			"pathExists":        p.PathExists,
			"created":           p.Created,
			"diskUsage":         p.DiskUsage,
			"sizeMB":            float64(p.DiskUsage) / (1024 * 1024),
			"source":            p.Source,
			"chromeProfileName": p.ChromeProfileName,
			"accountEmail":      p.AccountEmail,
			"accountName":       p.AccountName,
			"hasAccount":        p.HasAccount,
			"useWhen":           p.UseWhen,
			"description":       p.Description,
			"backend":           p.Backend,
		}
		break
	}

	if foundProfile == nil {
		httpx.Error(w, 404, fmt.Errorf("profile %q not found", id))
		return
	}

	httpx.JSON(w, 200, foundProfile)
}

func (pm *ProfileManager) handleDeleteByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	name, err := pm.resolveIDOnly(id)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	if err := pm.Delete(name); err != nil {
		httpx.Error(w, 500, err)
		return
	}

	authn.AuditLog(r, "profile.deleted", "profileId", id, "profileName", name)
	httpx.JSON(w, 200, map[string]any{"status": "deleted", "id": id, "name": name})
}

func (pm *ProfileManager) resolveIDOrName(idOrName string) (string, error) {
	name, err := pm.FindByID(idOrName)
	if err == nil {
		return name, nil
	}
	if pm.Exists(idOrName) {
		return idOrName, nil
	}
	return "", fmt.Errorf("profile %q not found (not a valid ID or name)", idOrName)
}

func (pm *ProfileManager) resolveIDOnly(id string) (string, error) {
	name, err := pm.FindByID(id)
	if err != nil {
		return "", fmt.Errorf("profile %q not found (must use profile ID, not name)", id)
	}
	return name, nil
}

func (pm *ProfileManager) handleUpdateByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name, err := pm.resolveIDOnly(id)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	var req struct {
		Name        *string                `json:"name"`
		UseWhen     *string                `json:"useWhen"`
		Description *string                `json:"description"`
		Backend     *bridge.ProfileBackend `json:"backend"`
	}
	raw, err := decodeProfileRequest(r, &req)
	if err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), fmt.Errorf("invalid JSON"))
		return
	}

	updates := make(map[string]string)
	finalName := name
	if req.Name != nil && *req.Name != name {
		if err := pm.Rename(name, *req.Name); err != nil {
			httpx.Error(w, profileMutationStatus(err), err)
			return
		}
		finalName = *req.Name
		updates["name"] = finalName
	}

	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.UseWhen != nil {
		updates["useWhen"] = *req.UseWhen
	}
	appendBackendUpdates(updates, req.Backend, raw)
	if len(updates) > 0 {
		if err := pm.UpdateMeta(finalName, updates); err != nil {
			httpx.Error(w, profileMutationStatus(err), err)
			return
		}
	}

	authn.AuditLog(r, "profile.updated", "profileId", profileID(finalName), "profileName", finalName)
	httpx.JSON(w, 200, map[string]any{"status": "updated", "id": profileID(finalName), "name": finalName})
}

func formatOptionalBool(v *bool) string {
	if v == nil {
		return ""
	}
	if *v {
		return "true"
	}
	return "false"
}

func (pm *ProfileManager) handleResetByIDOrName(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name, err := pm.resolveIDOnly(id)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	if err := pm.Reset(name); err != nil {
		httpx.Error(w, 500, err)
		return
	}
	authn.AuditLog(r, "profile.reset", "profileId", id, "profileName", name)
	httpx.JSON(w, 200, map[string]any{"status": "reset", "id": id, "name": name})
}

func (pm *ProfileManager) handleLogsByIDOrName(w http.ResponseWriter, r *http.Request) {
	idOrName := r.PathValue("id")
	name, err := pm.resolveIDOrName(idOrName)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	logs := pm.Logs(name, limit)
	httpx.JSON(w, 200, logs)
}

func (pm *ProfileManager) handleAnalyticsByIDOrName(w http.ResponseWriter, r *http.Request) {
	idOrName := r.PathValue("id")
	name, err := pm.resolveIDOrName(idOrName)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	report := pm.Analytics(name)
	httpx.JSON(w, 200, report)
}
