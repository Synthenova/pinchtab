package cloudprofiles

import (
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

func TestPreparedSessionAutoReleaseRemovesReadyJob(t *testing.T) {
	oldDelay := preparedSessionReleaseDelay
	oldAction := preparedSessionReleaseAction
	t.Cleanup(func() {
		preparedSessionReleaseDelay = oldDelay
		preparedSessionReleaseAction = oldAction
		syncMu.Lock()
		syncJobs = map[string]*syncJob{}
		syncMu.Unlock()
	})

	preparedSessionReleaseDelay = 10 * time.Millisecond
	released := make(chan *Session, 1)
	preparedSessionReleaseAction = func(session *Session) error {
		released <- session
		return nil
	}

	cfg := &bridge.ProfileCloudConfig{ProfileID: "cp_test"}
	session := &Session{Config: cfg}
	job := &syncJob{
		profilePath: "/tmp/profile",
		cfg:         cfg,
		status: SyncStatus{
			State: "ready",
		},
		session: session,
	}
	key := syncKey(job.profilePath, cfg)

	syncMu.Lock()
	syncJobs[key] = job
	syncMu.Unlock()

	startPreparedSessionReleaseTimer(key, job, session)

	select {
	case got := <-released:
		if got != session {
			t.Fatalf("expected released session pointer %p, got %p", session, got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for prepared session release")
	}

	syncMu.Lock()
	defer syncMu.Unlock()
	if _, ok := syncJobs[key]; ok {
		t.Fatal("expected prepared sync job to be removed after auto-release")
	}
}

func TestPreparedSessionAutoReleaseSkipsConsumedSession(t *testing.T) {
	oldDelay := preparedSessionReleaseDelay
	oldAction := preparedSessionReleaseAction
	t.Cleanup(func() {
		preparedSessionReleaseDelay = oldDelay
		preparedSessionReleaseAction = oldAction
		syncMu.Lock()
		syncJobs = map[string]*syncJob{}
		syncMu.Unlock()
	})

	preparedSessionReleaseDelay = 30 * time.Millisecond
	released := make(chan struct{}, 1)
	preparedSessionReleaseAction = func(session *Session) error {
		released <- struct{}{}
		return nil
	}

	cfg := &bridge.ProfileCloudConfig{ProfileID: "cp_test_consume"}
	session := &Session{Config: cfg}
	job := &syncJob{
		profilePath: "/tmp/profile-consume",
		cfg:         cfg,
		status: SyncStatus{
			State: "ready",
		},
		session: session,
	}
	key := syncKey(job.profilePath, cfg)

	syncMu.Lock()
	syncJobs[key] = job
	syncMu.Unlock()

	startPreparedSessionReleaseTimer(key, job, session)
	got, status := ConsumePreparedSession(job.profilePath, cfg)
	if got != session {
		t.Fatalf("expected consumed session %p, got %p", session, got)
	}
	if status == nil || status.State != "ready" {
		t.Fatalf("expected ready status on consume, got %#v", status)
	}

	select {
	case <-released:
		t.Fatal("prepared session should not auto-release after consume")
	case <-time.After(100 * time.Millisecond):
	}
}
