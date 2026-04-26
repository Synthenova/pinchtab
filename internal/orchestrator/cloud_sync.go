package orchestrator

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/cloudprofiles"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type cloudSyncRequiredError struct {
	ProfileName string
	Status      *cloudprofiles.SyncStatus
}

func (e *cloudSyncRequiredError) Error() string {
	if e == nil || e.Status == nil {
		return "profile sync in progress"
	}
	if e.Status.State == "error" && strings.TrimSpace(e.Status.Error) != "" {
		return e.Status.Error
	}
	return fmt.Sprintf("profile %q sync %s", e.ProfileName, e.Status.State)
}

func (o *Orchestrator) profilePathForName(name string) string {
	profilePath := filepath.Join(o.baseDir, name)
	if o.profiles != nil {
		if resolvedPath, err := o.profiles.ProfilePath(name); err == nil {
			profilePath = resolvedPath
		}
	}
	return profilePath
}

func (o *Orchestrator) cloudConfigForProfile(name string) (*bridge.ProfileCloudConfig, string, bool) {
	backend := o.profileBackend(name)
	if backend == nil || backend.Kind != "pinchtab" || backend.PinchTab == nil || !cloudprofiles.Enabled(backend.PinchTab.Cloud) {
		return nil, "", false
	}
	return backend.PinchTab.Cloud, o.profilePathForName(name), true
}

func (o *Orchestrator) finalizeStatusForProfile(name string) (*cloudprofiles.FinalizeStatus, error) {
	cfg, profilePath, ok := o.cloudConfigForProfile(name)
	if !ok {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return cloudprofiles.GetFinalizeStatus(ctx, profilePath, cfg)
}

func (o *Orchestrator) handleGetProfileSync(w http.ResponseWriter, r *http.Request) {
	name, err := o.resolveProfileName(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	cfg, profilePath, ok := o.cloudConfigForProfile(name)
	if !ok {
		httpx.ErrorCode(w, http.StatusBadRequest, "cloud_sync_disabled", "cloud sync is not enabled for this profile", false, nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	status, err := cloudprofiles.GetSyncStatus(ctx, profilePath, cfg)
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}
	httpx.JSON(w, 200, status)
}

func (o *Orchestrator) handleStartProfileSync(w http.ResponseWriter, r *http.Request) {
	name, err := o.resolveProfileName(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	cfg, profilePath, ok := o.cloudConfigForProfile(name)
	if !ok {
		httpx.ErrorCode(w, http.StatusBadRequest, "cloud_sync_disabled", "cloud sync is not enabled for this profile", false, nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	backend := o.profileBackend(name)
	var settings *bridge.ProfileBackendPinchTab
	if backend != nil {
		settings = backend.PinchTab
	}
	status, err := cloudprofiles.StartSync(ctx, name, profilePath, cfg, settings)
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}
	httpx.JSON(w, 202, status)
}

func (o *Orchestrator) handleGetProfileFinalize(w http.ResponseWriter, r *http.Request) {
	name, err := o.resolveProfileName(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	cfg, profilePath, ok := o.cloudConfigForProfile(name)
	if !ok {
		httpx.ErrorCode(w, http.StatusBadRequest, "cloud_sync_disabled", "cloud sync is not enabled for this profile", false, nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	status, err := cloudprofiles.GetFinalizeStatus(ctx, profilePath, cfg)
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}
	httpx.JSON(w, 200, status)
}

func (o *Orchestrator) handleRetryProfileFinalize(w http.ResponseWriter, r *http.Request) {
	name, err := o.resolveProfileName(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	backend := o.profileBackend(name)
	if backend == nil || backend.PinchTab == nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "cloud_sync_disabled", "cloud sync is not enabled for this profile", false, nil)
		return
	}
	cfg, profilePath, ok := o.cloudConfigForProfile(name)
	if !ok {
		httpx.ErrorCode(w, http.StatusBadRequest, "cloud_sync_disabled", "cloud sync is not enabled for this profile", false, nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	status, err := cloudprofiles.RetryFinalize(ctx, name, profilePath, cfg)
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}
	httpx.JSON(w, 202, status)
}

func (o *Orchestrator) handleDiscardProfileFinalize(w http.ResponseWriter, r *http.Request) {
	name, err := o.resolveProfileName(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	cfg, profilePath, ok := o.cloudConfigForProfile(name)
	if !ok {
		httpx.ErrorCode(w, http.StatusBadRequest, "cloud_sync_disabled", "cloud sync is not enabled for this profile", false, nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := cloudprofiles.DiscardFinalize(ctx, profilePath, cfg); err != nil {
		httpx.Error(w, 500, err)
		return
	}
	httpx.JSON(w, 200, map[string]string{"status": "discarded", "name": name})
}

func writeLaunchError(w http.ResponseWriter, err error) bool {
	if syncErr, ok := err.(*cloudSyncRequiredError); ok {
		details := map[string]any{}
		if syncErr.Status != nil {
			details["sync"] = syncErr.Status
		}
		httpx.ErrorCode(w, http.StatusConflict, "profile_sync_in_progress", "profile sync is in progress; poll /profiles/{id}/sync and retry launch when ready", true, details)
		return true
	}
	return false
}
