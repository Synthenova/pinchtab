package cloudprofiles

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

type SyncStatus struct {
	State         string    `json:"state,omitempty"`
	Progress      int       `json:"progress,omitempty"`
	BytesDone     int64     `json:"bytesDone,omitempty"`
	BytesTotal    int64     `json:"bytesTotal,omitempty"`
	Error         string    `json:"error,omitempty"`
	RemoteVersion string    `json:"remoteVersion,omitempty"`
	LocalVersion  string    `json:"localVersion,omitempty"`
	StartedAt     time.Time `json:"startedAt,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt,omitempty"`
}

type syncJob struct {
	profileName string
	profilePath string
	cfg         *bridge.ProfileCloudConfig
	status      SyncStatus
	session     *Session
}

var (
	syncMu   sync.Mutex
	syncJobs = map[string]*syncJob{}
)

func syncKey(profilePath string, cfg *bridge.ProfileCloudConfig) string {
	id := ""
	if cfg != nil {
		id = cfg.ProfileID
	}
	return filepath.Clean(profilePath) + "::" + id
}

func updateJobStatus(job *syncJob, fn func(*SyncStatus)) {
	syncMu.Lock()
	defer syncMu.Unlock()
	fn(&job.status)
	job.status.UpdatedAt = time.Now().UTC()
}

func ConsumePreparedSession(profilePath string, cfg *bridge.ProfileCloudConfig) (*Session, *SyncStatus) {
	syncMu.Lock()
	defer syncMu.Unlock()
	job := syncJobs[syncKey(profilePath, normalizeConfig(cfg))]
	if job == nil || job.status.State != "ready" || job.session == nil {
		if job == nil {
			return nil, nil
		}
		copy := job.status
		return nil, &copy
	}
	session := job.session
	copy := job.status
	delete(syncJobs, syncKey(profilePath, normalizeConfig(cfg)))
	return session, &copy
}

func progressPercent(done, total int64) int {
	if total <= 0 || done <= 0 {
		return 0
	}
	if done >= total {
		return 100
	}
	return int((done * 100) / total)
}

func downloadVersionWithProgress(ctx context.Context, client *Client, versionID, destArchive string, update func(done, total int64)) error {
	r, err := client.object("versions/" + versionID + ".tar.gz").NewReader(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	out, err := os.Create(destArchive)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	total := r.Attrs.Size
	var done int64
	buf := make([]byte, 128*1024)
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				return err
			}
			done += int64(n)
			update(done, total)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	update(total, total)
	return nil
}

func extractArchiveWithProgress(archivePath, dest string, update func(done, total int64)) error {
	if err := clearProfileData(dest); err != nil {
		return err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	total := info.Size()
	cr := &countingReader{r: f}
	gzr, err := gzip.NewReader(cr)
	if err != nil {
		return err
	}
	defer func() { _ = gzr.Close() }()
	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		update(cr.n, total)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.Clean(hdr.Name))
		if !hasPathPrefix(target, dest) {
			return fmt.Errorf("invalid archive path %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tarTypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tarTypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
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
	update(total, total)
	return nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

const (
	tarTypeDir = byte('5')
	tarTypeReg = byte('0')
)

func hasPathPrefix(target, dest string) bool {
	cleanTarget := filepath.Clean(target)
	cleanDest := filepath.Clean(dest)
	if cleanTarget == cleanDest {
		return true
	}
	return strings.HasPrefix(cleanTarget, cleanDest+string(os.PathSeparator))
}

func prepareWithProgress(ctx context.Context, profileName, profilePath string, cfg *bridge.ProfileCloudConfig, settings *bridge.ProfileBackendPinchTab, setStatus func(state string, done, total int64), setError func(string), setVersions func(localVersion, remoteVersion string)) (*Session, error) {
	cfg = normalizeConfig(cfg)
	client, err := New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	if err := os.MkdirAll(profilePath, 0o755); err != nil {
		return nil, err
	}
	setStatus("checking", 0, 0)
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
	if hasLatest {
		setVersions(state.LastSyncedVersion, latest.VersionID)
	}
	if hasLatest && state.LastSyncedVersion != latest.VersionID {
		tmpArchive := filepath.Join(os.TempDir(), "pinchtab-cloud-"+cfg.ProfileID+".tar.gz")
		setStatus("downloading", 0, 0)
		if err := downloadVersionWithProgress(ctx, client, latest.VersionID, tmpArchive, func(done, total int64) {
			setStatus("downloading", done, total)
		}); err != nil {
			_ = client.ReleaseLease(ctx, lease.LeaseID)
			setError(err.Error())
			return nil, err
		}
		setStatus("extracting", 0, 0)
		if err := extractArchiveWithProgress(tmpArchive, profilePath, func(done, total int64) {
			setStatus("extracting", done, total)
		}); err != nil {
			_ = os.Remove(tmpArchive)
			_ = client.ReleaseLease(ctx, lease.LeaseID)
			setError(err.Error())
			return nil, err
		}
		_ = os.Remove(tmpArchive)
		state.LastSyncedVersion = latest.VersionID
		state.LastSyncedHash = latest.Hash
		state.LastSyncedAt = latest.UpdatedAt
		state.LastError = ""
	}
	if err := os.MkdirAll(filepath.Join(profilePath, "Default"), 0o755); err != nil {
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
	setVersions(state.LastSyncedVersion, latest.VersionID)
	setStatus("ready", 0, 0)
	return &Session{
		Config:       cfg,
		Lease:        lease,
		RemoteLatest: latest,
		InitialHash:  localHash,
		stopRenew:    stopRenew,
	}, nil
}

func StartSync(ctx context.Context, profileName, profilePath string, cfg *bridge.ProfileCloudConfig, settings *bridge.ProfileBackendPinchTab) (*SyncStatus, error) {
	cfg = normalizeConfig(cfg)
	if !Enabled(cfg) {
		return &SyncStatus{State: "disabled"}, nil
	}
	key := syncKey(profilePath, cfg)
	syncMu.Lock()
	if existing := syncJobs[key]; existing != nil {
		if existing.status.State == "error" {
			delete(syncJobs, key)
		} else {
			status := existing.status
			syncMu.Unlock()
			return &status, nil
		}
	}
	job := &syncJob{
		profileName: profileName,
		profilePath: profilePath,
		cfg:         cfg,
		status: SyncStatus{
			State:     "queued",
			StartedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}
	syncJobs[key] = job
	syncMu.Unlock()

	go func() {
		session, err := prepareWithProgress(context.Background(), profileName, profilePath, cfg, settings,
			func(state string, done, total int64) {
				updateJobStatus(job, func(status *SyncStatus) {
					status.State = state
					status.BytesDone = done
					status.BytesTotal = total
					status.Progress = progressPercent(done, total)
					status.Error = ""
				})
			},
			func(message string) {
				updateJobStatus(job, func(status *SyncStatus) {
					status.Error = message
				})
			},
			func(localVersion, remoteVersion string) {
				updateJobStatus(job, func(status *SyncStatus) {
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
			updateJobStatus(job, func(status *SyncStatus) {
				status.State = "error"
				status.Error = err.Error()
			})
			return
		}
		syncMu.Lock()
		defer syncMu.Unlock()
		if current := syncJobs[key]; current == job {
			current.session = session
		}
	}()

	status := job.status
	return &status, nil
}

func GetSyncStatus(ctx context.Context, profilePath string, cfg *bridge.ProfileCloudConfig) (*SyncStatus, error) {
	cfg = normalizeConfig(cfg)
	if !Enabled(cfg) {
		return &SyncStatus{State: "disabled"}, nil
	}
	key := syncKey(profilePath, cfg)
	syncMu.Lock()
	if existing := syncJobs[key]; existing != nil {
		status := existing.status
		syncMu.Unlock()
		return &status, nil
	}
	syncMu.Unlock()

	state := readLocalState(profilePath)
	status := &SyncStatus{
		State:        "idle",
		LocalVersion: state.LastSyncedVersion,
		Error:        state.LastError,
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
		if status.LocalVersion != "" && status.LocalVersion == latest.VersionID {
			status.State = "cached"
		}
	}
	return status, nil
}
