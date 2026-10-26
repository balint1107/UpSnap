package pb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/seriousm4x/upsnap/networking"
)

// BootJob persists one physical host's OS-switch progress; times are UTC and deadline bounds readiness checks.
type BootJob struct {
	Target       string    `json:"target"`
	Actor        string    `json:"actor"`
	SourceOS     string    `json:"source_os"`
	SourceBootID string    `json:"source_boot_id"`
	BootOrder    string    `json:"boot_order"`
	LoaderOrder  string    `json:"loader_order"`
	Stage        string    `json:"stage"`
	Message      string    `json:"message"`
	Started      time.Time `json:"started"`
	Deadline     time.Time `json:"deadline"`
}

var bootJobs sync.Map

// bootJobPath returns an owned path and shared mutex for ip under app's private data directory; never executes SSH.
func bootJobPath(app core.App, ip string) (string, *sync.Mutex) {
	sum := sha256.Sum256([]byte(ip))
	key := hex.EncodeToString(sum[:])
	path := filepath.Join(app.DataDir(), "boot-jobs", key+".json")
	lock, _ := bootJobs.LoadOrStore(path, &sync.Mutex{})
	return path, lock.(*sync.Mutex)
}

// saveBootJob atomically persists job at path with owner-only permissions; caller holds the host mutex. Returns I/O failure.
func saveBootJob(path string, job *BootJob) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path+".tmp", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(path+".tmp", path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// loadBootJob reads path into an owned job; nil means absent, malformed state fails closed with an error.
func loadBootJob(path string) (*BootJob, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var job BootJob
	if err = json.Unmarshal(data, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

// bootJobActive reports whether job is a nonterminal transition; nil/ready/failed permit another explicit request.
func bootJobActive(job *BootJob) bool {
	return job != nil && job.Stage != "ready" && job.Stage != "failed"
}

// reconcileBootJob updates job from read-only status and now; returns whether state changed. Never sends a power command.
func reconcileBootJob(job *BootJob, status *networking.BootStatus, now time.Time) bool {
	if !bootJobActive(job) {
		return false
	}
	if status != nil && status.Ready && status.OS == job.Target && status.BootID != "" && status.BootID != job.SourceBootID {
		if job.LoaderOrder == "" || status.LoaderOrder != job.LoaderOrder || status.BootNext != nil {
			job.Stage = "failed"
			job.Message = "Target is ready, but firmware settings need review."
		} else {
			job.Stage = "ready"
			job.Message = "SSH ready in " + job.Target + "; boot selection consumed and OS loader order preserved."
		}
		return true
	}
	if !now.Before(job.Deadline) {
		job.Stage = "failed"
		job.Message = "Target SSH readiness timed out. No restart retry was sent; inspect pending BootNext before another attempt."
		return true
	}
	return false
}

// currentBootJob reconciles persisted host progress with status; returns owned job or storage error. No restart is replayed.
func currentBootJob(app core.App, ip string, status *networking.BootStatus) (*BootJob, error) {
	path, lock := bootJobPath(app, ip)
	lock.Lock()
	defer lock.Unlock()
	job, err := loadBootJob(path)
	if err != nil {
		return nil, err
	}
	if reconcileBootJob(job, status, time.Now().UTC()) {
		err = saveBootJob(path, job)
	}
	return job, err
}

// startBootJob serializes an explicit actor's target switch for ip; persists progress before preparation and queueing.
// ctx bounds SSH. Returns a persisted job or error; a lost acknowledgement never causes an automatic retry.
func startBootJob(ctx context.Context, app core.App, ip, target, actor string) (*BootJob, error) {
	if target != "ubuntu" && target != "windows" {
		return nil, errors.New("unsupported boot target")
	}
	path, lock := bootJobPath(app, ip)
	lock.Lock()
	defer lock.Unlock()
	previous, err := loadBootJob(path)
	if err != nil {
		return nil, err
	}
	if bootJobActive(previous) {
		return nil, errors.New("a boot switch is already in progress")
	}
	status, err := networking.ReadBootStatus(ctx, ip)
	if err != nil {
		return nil, err
	}
	if !status.RestartEnabled || !status.WritesEnabled || status.BootID == "" || status.LoaderOrder == "" || !status.Targets[target].Available {
		return nil, errors.New("restart is locked or target is unavailable")
	}
	if status.BootNext != nil {
		return nil, errors.New("existing BootNext must be reviewed before another switch")
	}
	now := time.Now().UTC()
	job := &BootJob{Target: target, Actor: actor, SourceOS: status.OS, SourceBootID: status.BootID, BootOrder: status.BootOrder, LoaderOrder: status.LoaderOrder, Stage: "preparing", Message: "Preparing one-time boot selection.", Started: now, Deadline: now.Add(6 * time.Minute)}
	if err = saveBootJob(path, job); err != nil {
		return nil, err
	}
	prepared, err := networking.CheckBootTarget(ctx, ip, target, true)
	if err == nil && (prepared.BootID != job.SourceBootID || prepared.BootOrder != job.BootOrder) {
		err = errors.New("source boot identity changed during preparation")
	}
	if err == nil {
		job.Stage = "restarting"
		job.Message = "Graceful restart requested; waiting for target SSH readiness."
		err = saveBootJob(path, job)
		if err == nil {
			_, err = networking.RestartBootTarget(ctx, ip, target, job.SourceBootID, job.BootOrder)
		}
	}
	if err != nil {
		job.Stage = "failed"
		job.Message = err.Error() + " No automatic retry; review BootNext before retrying."
	}
	if saveErr := saveBootJob(path, job); saveErr != nil {
		return nil, saveErr
	}
	return job, err
}
