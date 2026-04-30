package orchestrator

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cloudprofiles"
)

func TestWriteLaunchErrorFinalizeInProgress(t *testing.T) {
	w := httptest.NewRecorder()
	err := &cloudFinalizeRequiredError{
		ProfileName: "Facemax Headed",
		Status: &cloudprofiles.FinalizeStatus{
			State: "uploading",
		},
	}

	if !writeLaunchError(w, err) {
		t.Fatal("expected writeLaunchError to handle finalize-in-progress error")
	}
	if w.Code != 409 {
		t.Fatalf("expected 409, got %d", w.Code)
	}

	var body map[string]any
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &body); decodeErr != nil {
		t.Fatalf("failed to decode body: %v", decodeErr)
	}
	if body["code"] != "profile_finalize_in_progress" {
		t.Fatalf("expected code profile_finalize_in_progress, got %#v", body["code"])
	}
}

func TestWriteLaunchErrorFinalizeFailed(t *testing.T) {
	w := httptest.NewRecorder()
	err := &cloudFinalizeRequiredError{
		ProfileName: "Facemax Headed",
		Status: &cloudprofiles.FinalizeStatus{
			State:      "error",
			Error:      "upload failed",
			CanRetry:   true,
			CanDiscard: true,
		},
	}

	if !writeLaunchError(w, err) {
		t.Fatal("expected writeLaunchError to handle finalize-failed error")
	}
	if w.Code != 409 {
		t.Fatalf("expected 409, got %d", w.Code)
	}

	var body map[string]any
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &body); decodeErr != nil {
		t.Fatalf("failed to decode body: %v", decodeErr)
	}
	if body["code"] != "profile_finalize_failed" {
		t.Fatalf("expected code profile_finalize_failed, got %#v", body["code"])
	}
}
