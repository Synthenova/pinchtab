package cloudprofiles

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

const finalizeFailureLeaseWindow = 10 * time.Minute

type FinalizeStatus struct {
	State         string    `json:"state,omitempty"`
	Progress      int       `json:"progress,omitempty"`
	BytesDone     int64     `json:"bytesDone,omitempty"`
	BytesTotal    int64     `json:"bytesTotal,omitempty"`
	Error         string    `json:"error,omitempty"`
	RemoteVersion string    `json:"remoteVersion,omitempty"`
	LocalVersion  string    `json:"localVersion,omitempty"`
	StartedAt     time.Time `json:"startedAt,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt,omitempty"`
	CanRetry      bool      `json:"canRetry,omitempty"`
	CanDiscard    bool      `json:"canDiscard,omitempty"`
}

type finalizeJob struct {
	profilePath  string
	profileName  string
	cfg          *bridge.ProfileCloudConfig
	status       FinalizeStatus
	session      *Session
	failureUntil time.Time
}

var (
	finalizeMu   sync.Mutex
	finalizeJobs = map[string]*finalizeJob{}
)

func finalizeKey(profilePath string, cfg *bridge.ProfileCloudConfig) string {
	id := ""
	if cfg != nil {
		id = cfg.ProfileID
	}
	return filepath.Clean(profilePath) + "::" + id
}

func updateFinalizeStatus(job *finalizeJob, fn func(*FinalizeStatus)) {
	finalizeMu.Lock()
	defer finalizeMu.Unlock()
	fn(&job.status)
	job.status.UpdatedAt = time.Now().UTC()
}

func startLeaseRenewLoopUntil(cfg *bridge.ProfileCloudConfig, leaseID string, deadline time.Time) func() {
	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if !deadline.IsZero() && time.Now().UTC().After(deadline) {
					return
				}
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

func uploadVersionWithProgress(ctx context.Context, client *Client, archivePath, versionID string, update func(done, total int64)) (int64, string, error) {
	obj := client.object("versions/" + versionID + ".tar.gz")
	w := obj.NewWriter(ctx)
	w.ContentType = "application/gzip"
	file, err := os.Open(archivePath)
	if err != nil {
		_ = w.Close()
		return 0, "", err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		_ = w.Close()
		return 0, "", err
	}
	total := info.Size()
	buf := make([]byte, 128*1024)
	var done int64
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			if _, err := w.Write(buf[:n]); err != nil {
				_ = w.Close()
				return 0, "", err
			}
			done += int64(n)
			update(done, total)
		}
		if readErr == nil {
			continue
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = w.Close()
			return 0, "", readErr
		}
	}
	if err := w.Close(); err != nil {
		return 0, "", err
	}
	hash, err := fileHash(archivePath)
	if err != nil {
		return 0, "", err
	}
	update(total, total)
	return total, hash, nil
}

func runFinalize(ctx context.Context, profilePath string, session *Session, setStatus func(state string, progress int, done, total int64), setError func(string), setVersions func(localVersion, remoteVersion string)) error {
	if session == nil || session.Config == nil {
		return nil
	}
	client, err := New(ctx, session.Config)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	state := readLocalState(profilePath)
	currentHash, err := hashDirectory(profilePath)
	if err != nil {
		state.LastError = err.Error()
		state.LastFinalizeState = "upload-failed"
		_ = writeLocalState(profilePath, state)
		return err
	}
	state.LastLocalHash = currentHash
	setVersions(state.LastSyncedVersion, session.RemoteLatest.VersionID)

	if currentHash != session.InitialHash || state.LastSyncedVersion == "" {
		setStatus("archiving", 0, 0, 0)
		archivePath, sizeBytes, archiveHash, err := archiveDirectory(profilePath)
		if err != nil {
			state.LastError = err.Error()
			state.LastFinalizeState = "upload-failed"
			_ = writeLocalState(profilePath, state)
			return err
		}
		defer func() { _ = os.Remove(archivePath) }()
		versionID := randomVersionID()
		setStatus("uploading", 0, 0, sizeBytes)
		if _, _, err := uploadVersionWithProgress(ctx, client, archivePath, versionID, func(done, total int64) {
			setStatus("uploading", progressPercent(done, total), done, total)
		}); err != nil {
			state.LastError = err.Error()
			state.LastFinalizeState = "upload-failed"
			_ = writeLocalState(profilePath, state)
			return err
		}
		setStatus("updating-latest", 100, sizeBytes, sizeBytes)
		latest := latestVersion{
			VersionID: versionID,
			Hash:      archiveHash,
			UpdatedAt: time.Now().UTC(),
			SizeBytes: sizeBytes,
		}
		if err := writeJSON(ctx, client.object("latest.json"), latest); err != nil {
			state.LastError = err.Error()
			state.LastFinalizeState = "upload-failed"
			_ = writeLocalState(profilePath, state)
			return err
		}
		state.LastSyncedVersion = latest.VersionID
		state.LastSyncedHash = latest.Hash
		state.LastSyncedAt = latest.UpdatedAt
		state.LastError = ""
		state.LastFinalizeState = "available"
		setVersions(state.LastSyncedVersion, latest.VersionID)
	} else {
		state.LastFinalizeState = "available"
		state.LastError = ""
	}

	if err := writeLocalState(profilePath, state); err != nil {
		return err
	}
	setStatus("releasing-lease", 100, 0, 0)
	if session.stopRenew != nil {
		session.stopRenew()
		session.stopRenew = nil
	}
	if err := client.ReleaseLease(ctx, session.Lease.LeaseID); err != nil {
		return err
	}
	if session.Config.KeepLocalCache != nil && !*session.Config.KeepLocalCache {
		if err := clearPortableState(profilePath); err != nil {
			return err
		}
	}
	setStatus("done", 100, 0, 0)
	return nil
}

func StartFinalize(ctx context.Context, profileName, profilePath string, session *Session) (*FinalizeStatus, error) {
	if session == nil || session.Config == nil {
		return &FinalizeStatus{State: "disabled"}, nil
	}
	cfg := normalizeConfig(session.Config)
	key := finalizeKey(profilePath, cfg)
	finalizeMu.Lock()
	if existing := finalizeJobs[key]; existing != nil {
		status := existing.status
		finalizeMu.Unlock()
		return &status, nil
	}
	job := &finalizeJob{
		profilePath: profilePath,
		profileName: profileName,
		cfg:         cfg,
		session:     session,
		status: FinalizeStatus{
			State:     "queued",
			StartedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}
	finalizeJobs[key] = job
	finalizeMu.Unlock()
	state := readLocalState(profilePath)
	state.LastFinalizeState = "stopped-uploading"
	state.LastError = ""
	_ = writeLocalState(profilePath, state)

	go func() {
		err := runFinalize(context.Background(), profilePath, session,
			func(state string, progress int, done, total int64) {
				updateFinalizeStatus(job, func(status *FinalizeStatus) {
					status.State = state
					status.Progress = progress
					status.BytesDone = done
					status.BytesTotal = total
					status.CanRetry = false
					status.CanDiscard = false
					status.Error = ""
				})
			},
			func(message string) {
				updateFinalizeStatus(job, func(status *FinalizeStatus) {
					status.Error = message
				})
			},
			func(localVersion, remoteVersion string) {
				updateFinalizeStatus(job, func(status *FinalizeStatus) {
					if localVersion != "" {
						status.LocalVersion = localVersion
					}
					if remoteVersion != "" {
						status.RemoteVersion = remoteVersion
					}
				})
			},
		)
		if err != nil {
			updateFinalizeStatus(job, func(status *FinalizeStatus) {
				status.State = "error"
				status.Error = err.Error()
				status.CanRetry = true
				status.CanDiscard = true
			})
			state := readLocalState(profilePath)
			state.LastFinalizeState = "upload-failed"
			state.LastError = err.Error()
			_ = writeLocalState(profilePath, state)

			deadline := time.Now().UTC().Add(finalizeFailureLeaseWindow)
			finalizeMu.Lock()
			job.failureUntil = deadline
			finalizeMu.Unlock()
			if session.stopRenew != nil {
				session.stopRenew()
			}
			session.stopRenew = startLeaseRenewLoopUntil(cfg, session.Lease.LeaseID, deadline)
			return
		}
		finalizeMu.Lock()
		delete(finalizeJobs, key)
		finalizeMu.Unlock()
	}()

	status := job.status
	return &status, nil
}

func GetFinalizeStatus(ctx context.Context, profilePath string, cfg *bridge.ProfileCloudConfig) (*FinalizeStatus, error) {
	cfg = normalizeConfig(cfg)
	if !Enabled(cfg) {
		return &FinalizeStatus{State: "disabled"}, nil
	}
	key := finalizeKey(profilePath, cfg)
	finalizeMu.Lock()
	if existing := finalizeJobs[key]; existing != nil {
		status := existing.status
		finalizeMu.Unlock()
		return &status, nil
	}
	finalizeMu.Unlock()

	state := readLocalState(profilePath)
	status := &FinalizeStatus{
		State:        "idle",
		LocalVersion: state.LastSyncedVersion,
		Error:        state.LastError,
	}
	if state.LastFinalizeState == "upload-failed" {
		status.State = "error"
		status.CanRetry = true
		status.CanDiscard = true
	}
	client, err := New(ctx, cfg)
	if err != nil {
		status.State = "error"
		status.Error = err.Error()
		return status, nil
	}
	defer func() { _ = client.Close() }()
	if latest, ok, err := client.latest(ctx); err == nil && ok {
		status.RemoteVersion = latest.VersionID
	}
	return status, nil
}

func RetryFinalize(ctx context.Context, profileName, profilePath string, cfg *bridge.ProfileCloudConfig) (*FinalizeStatus, error) {
	cfg = normalizeConfig(cfg)
	key := finalizeKey(profilePath, cfg)
	finalizeMu.Lock()
	job := finalizeJobs[key]
	if job != nil {
		if job.status.State != "error" {
			status := job.status
			finalizeMu.Unlock()
			return &status, nil
		}
		delete(finalizeJobs, key)
	}
	finalizeMu.Unlock()

	session := (*Session)(nil)
	if job != nil {
		session = job.session
	}
	if session == nil {
		client, err := New(ctx, cfg)
		if err != nil {
			return nil, err
		}
		defer func() { _ = client.Close() }()
		lease, ok, err := client.currentLease(ctx)
		if err != nil {
			return nil, err
		}
		if !ok || lease.ExpiresAt.Before(time.Now().UTC()) {
			return nil, fmt.Errorf("no active lease available to retry upload for %s", profileName)
		}
		latest, ok, err := client.latest(ctx)
		if err != nil {
			return nil, err
		}
		if !ok {
			latest = latestVersion{}
		}
		session = &Session{
			Config:       cfg,
			Lease:        lease,
			RemoteLatest: latest,
			InitialHash:  "",
		}
	}
	if session.stopRenew != nil {
		session.stopRenew()
	}
	session.stopRenew = startLeaseRenewLoop(cfg, session.Lease.LeaseID)
	state := readLocalState(profilePath)
	state.LastFinalizeState = "stopped-uploading"
	state.LastError = ""
	_ = writeLocalState(profilePath, state)
	return StartFinalize(ctx, profileName, profilePath, session)
}

func DiscardFinalize(ctx context.Context, profilePath string, cfg *bridge.ProfileCloudConfig) error {
	cfg = normalizeConfig(cfg)
	key := finalizeKey(profilePath, cfg)
	finalizeMu.Lock()
	job := finalizeJobs[key]
	if job != nil {
		delete(finalizeJobs, key)
	}
	finalizeMu.Unlock()

	var session *Session
	if job != nil {
		session = job.session
	}
	if session != nil && session.stopRenew != nil {
		session.stopRenew()
		session.stopRenew = nil
	}
	client, err := New(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	if session != nil {
		_ = client.ReleaseLease(ctx, session.Lease.LeaseID)
	} else if lease, ok, err := client.currentLease(ctx); err == nil && ok {
		_ = client.ReleaseLease(ctx, lease.LeaseID)
	}
	if err := clearPortableState(profilePath); err != nil {
		return err
	}
	state := readLocalState(profilePath)
	state.LastLocalHash = ""
	state.LastSyncedHash = ""
	state.LastSyncedVersion = ""
	state.LastSyncedAt = time.Time{}
	state.LastFinalizeState = "available"
	state.LastError = ""
	return writeLocalState(profilePath, state)
}
