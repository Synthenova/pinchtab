package cloudprofiles

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

func boolPtr(v bool) *bool {
	return &v
}

func TestStatusShowsBackgroundFinalizeProgress(t *testing.T) {
	t.Cleanup(func() {
		finalizeMu.Lock()
		finalizeJobs = map[string]*finalizeJob{}
		finalizeMu.Unlock()
	})

	profilePath := t.TempDir()
	cfg := &bridge.ProfileCloudConfig{
		Enabled:        boolPtr(true),
		Provider:       "gcs",
		Bucket:         "bucket",
		Prefix:         "pinchtab/profiles",
		ProfileID:      "cp_test_finalize_progress",
		CredentialPath: filepath.Join(profilePath, "missing.json"),
	}
	key := finalizeKey(profilePath, cfg)

	finalizeMu.Lock()
	finalizeJobs[key] = &finalizeJob{
		profilePath: profilePath,
		cfg:         cfg,
		status: FinalizeStatus{
			State:         "uploading",
			Progress:      61,
			LocalVersion:  "local-v1",
			RemoteVersion: "remote-v1",
			StartedAt:     time.Now().UTC(),
			UpdatedAt:     time.Now().UTC(),
		},
	}
	finalizeMu.Unlock()

	status, err := Status(context.Background(), profilePath, cfg)
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if status == nil {
		t.Fatal("Status returned nil")
	}
	if status.State != "stopped-uploading" {
		t.Fatalf("expected stopped-uploading, got %q", status.State)
	}
	if status.Progress != 61 {
		t.Fatalf("expected progress 61, got %d", status.Progress)
	}
	if status.Message != "uploading" {
		t.Fatalf("expected uploading message, got %q", status.Message)
	}
	if status.LocalVersion != "local-v1" {
		t.Fatalf("expected local version local-v1, got %q", status.LocalVersion)
	}
	if status.RemoteVersion != "remote-v1" {
		t.Fatalf("expected remote version remote-v1, got %q", status.RemoteVersion)
	}
}

func TestGetFinalizeStatusShowsRetryableFailureFromLocalState(t *testing.T) {
	t.Cleanup(func() {
		finalizeMu.Lock()
		finalizeJobs = map[string]*finalizeJob{}
		finalizeMu.Unlock()
	})

	profilePath := t.TempDir()
	if err := writeLocalState(profilePath, localState{
		LastSyncedVersion: "remote-v2",
		LastFinalizeState: "upload-failed",
		LastError:         "upload failed earlier",
	}); err != nil {
		t.Fatalf("writeLocalState failed: %v", err)
	}

	cfg := &bridge.ProfileCloudConfig{
		Enabled:        boolPtr(true),
		Provider:       "gcs",
		Bucket:         "bucket",
		Prefix:         "pinchtab/profiles",
		ProfileID:      "cp_test_finalize_failure",
		CredentialPath: filepath.Join(profilePath, "missing.json"),
	}

	status, err := GetFinalizeStatus(context.Background(), profilePath, cfg)
	if err != nil {
		t.Fatalf("GetFinalizeStatus returned error: %v", err)
	}
	if status == nil {
		t.Fatal("GetFinalizeStatus returned nil")
	}
	if status.State != "error" {
		t.Fatalf("expected error state, got %q", status.State)
	}
	if !status.CanRetry {
		t.Fatal("expected CanRetry to be true")
	}
	if !status.CanDiscard {
		t.Fatal("expected CanDiscard to be true")
	}
	if status.LocalVersion != "remote-v2" {
		t.Fatalf("expected local version remote-v2, got %q", status.LocalVersion)
	}
	if status.Error == "" {
		t.Fatal("expected error message to be populated")
	}
}
