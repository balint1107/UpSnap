package pb

import (
	"github.com/seriousm4x/upsnap/networking"
	"path/filepath"
	"testing"
	"time"
)

// TestBootJobRequiresAuthenticatedChangedBoot verifies OS, boot identity, consumed selection and loader order before ready.
func TestBootJobRequiresAuthenticatedChangedBoot(t *testing.T) {
	now := time.Now().UTC()
	job := &BootJob{Target: "windows", SourceBootID: "source", Stage: "restarting", LoaderOrder: "0000,0003", Deadline: now.Add(time.Minute)}
	status := &networking.BootStatus{OS: "ubuntu", Ready: true, BootID: "source", LoaderOrder: "0000,0003"}
	if reconcileBootJob(job, status, now) {
		t.Fatal("source OS counted as target")
	}
	status.OS = "windows"
	if reconcileBootJob(job, status, now) {
		t.Fatal("unchanged boot identity accepted")
	}
	status.BootID = "new"
	next := "0003"
	status.BootNext = &next
	if !reconcileBootJob(job, status, now) || job.Stage != "failed" {
		t.Fatal("unconsumed BootNext accepted")
	}
	job.Stage = "restarting"
	status.BootNext = nil
	status.LoaderOrder = "0003,0000"
	if !reconcileBootJob(job, status, now) || job.Stage != "failed" {
		t.Fatal("changed loader order accepted")
	}
	job.Stage = "restarting"
	status.LoaderOrder = "0000,0003"
	status.BootOrder = "firmware omits legacy BBS entries"
	if !reconcileBootJob(job, status, now) || job.Stage != "ready" {
		t.Fatal("verified target not marked ready")
	}
}

// TestBootJobPersistsAndTimesOut proves progress survives reload and timeout never triggers another command.
func TestBootJobPersistsAndTimesOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs", "host.json")
	job := &BootJob{Target: "ubuntu", Stage: "restarting", Deadline: time.Now().Add(-time.Second)}
	if err := saveBootJob(path, job); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadBootJob(path)
	if err != nil || loaded.Target != "ubuntu" {
		t.Fatalf("reload failed: %v", err)
	}
	if !reconcileBootJob(loaded, nil, time.Now()) || loaded.Stage != "failed" {
		t.Fatal("timeout not recorded")
	}
	if bootJobActive(loaded) {
		t.Fatal("failed job remains active")
	}
}
