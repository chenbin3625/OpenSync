package service

import (
	"testing"
	"time"
)

func TestTaskRetentionCutoffDisabledWhenTaskSaveIsZero(t *testing.T) {
	_, ok := taskRetentionCutoff(time.Unix(1_700_000_000, 0), 0)
	if ok {
		t.Fatalf("taskRetentionCutoff(..., 0) ok = true, want false")
	}
}

func TestTaskRetentionCutoffUsesDays(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cutoff, ok := taskRetentionCutoff(now, 7)
	if !ok {
		t.Fatalf("taskRetentionCutoff(..., 7) ok = false, want true")
	}

	want := now.Add(-7 * 24 * time.Hour).Unix()
	if cutoff != want {
		t.Fatalf("cutoff = %d, want %d", cutoff, want)
	}
}
