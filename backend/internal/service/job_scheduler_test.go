package service

import (
	"testing"
	"time"
)

func TestBuildCronSpecFromSeparateFields(t *testing.T) {
	spec, err := buildCronSpec(map[string]interface{}{
		"second":      "0",
		"minute":      "*/5",
		"hour":        "*",
		"day":         "*",
		"month":       "*",
		"day_of_week": "*",
	})
	if err != nil {
		t.Fatalf("buildCronSpec() error: %v", err)
	}

	if spec != "0 */5 * * * *" {
		t.Fatalf("buildCronSpec() = %q, want %q", spec, "0 */5 * * * *")
	}
}

func TestBuildCronSpecRejectsEmptySchedule(t *testing.T) {
	spec, err := buildCronSpec(map[string]interface{}{})
	if err != nil {
		t.Fatalf("buildCronSpec(empty) error: %v", err)
	}
	if spec != "" {
		t.Fatalf("buildCronSpec(empty) = %q, want empty string", spec)
	}
}

func TestBuildCronSpecRejectsLogInjectionCharacters(t *testing.T) {
	_, err := buildCronSpec(map[string]interface{}{
		"second": "0\nmalicious",
	})
	if err == nil {
		t.Fatalf("buildCronSpec() error = nil, want invalid cron field error")
	}
}

func TestSchedulerResumeIsIdempotent(t *testing.T) {
	s := NewScheduler()
	defer s.Stop()

	jobData := map[string]interface{}{
		"enable":   1,
		"isCron":   0,
		"interval": 60,
	}
	if err := s.AddJob(0, jobData, func() {}); err != nil {
		t.Fatalf("AddJob() error: %v", err)
	}
	if got := len(s.cron.Entries()); got != 1 {
		t.Fatalf("entries after AddJob = %d, want 1", got)
	}

	if err := s.Resume(0, jobData, func() {}); err != nil {
		t.Fatalf("Resume() error: %v", err)
	}
	if got := len(s.cron.Entries()); got != 1 {
		t.Fatalf("entries after duplicate Resume = %d, want 1", got)
	}
}

func TestSchedulerResumeReAddsInitiallyDisabledJob(t *testing.T) {
	s := NewScheduler()
	defer s.Stop()

	jobData := map[string]interface{}{
		"enable":   0,
		"isCron":   0,
		"interval": 60,
	}
	if err := s.AddJob(0, jobData, func() {}); err != nil {
		t.Fatalf("AddJob() error: %v", err)
	}
	if got := len(s.cron.Entries()); got != 0 {
		t.Fatalf("entries after disabled AddJob = %d, want 0", got)
	}

	jobData["enable"] = 1
	if err := s.Resume(0, jobData, func() {}); err != nil {
		t.Fatalf("Resume() error: %v", err)
	}
	if got := len(s.cron.Entries()); got != 1 {
		t.Fatalf("entries after Resume = %d, want 1", got)
	}
}

func TestSchedulerResumeAfterStopReturnsError(t *testing.T) {
	s := NewScheduler()
	s.Stop()

	if err := s.Resume(0, map[string]interface{}{"interval": 1}, func() {}); err == nil {
		t.Fatalf("Resume() error = nil after Stop, want error")
	}
}

func TestSchedulerCronUsesDefaultAsiaShanghaiWhenLocalIsUTC(t *testing.T) {
	t.Setenv("TZ", "")

	oldLocal := time.Local
	time.Local = time.UTC
	defer func() {
		time.Local = oldLocal
	}()

	s := NewScheduler()
	defer s.Stop()

	jobData := map[string]interface{}{
		"enable":      1,
		"isCron":      1,
		"second":      "0",
		"minute":      "0",
		"hour":        "8",
		"day":         "*",
		"month":       "*",
		"day_of_week": "*",
	}
	if err := s.AddJob(1, jobData, func() {}); err != nil {
		t.Fatalf("AddJob() error: %v", err)
	}

	entry := s.cron.Entry(s.entryID)
	if entry.Next.IsZero() {
		t.Fatal("entry.Next is zero, want scheduled next run time")
	}
	if got := entry.Next.Location().String(); got != "Asia/Shanghai" {
		t.Fatalf("next run location = %q, want Asia/Shanghai", got)
	}
}

func TestSchedulerCronHonorsExplicitTZ(t *testing.T) {
	t.Setenv("TZ", "UTC")

	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("LoadLocation(Asia/Shanghai) error: %v", err)
	}
	oldLocal := time.Local
	time.Local = shanghai
	defer func() {
		time.Local = oldLocal
	}()

	s := NewScheduler()
	defer s.Stop()

	jobData := map[string]interface{}{
		"enable":      1,
		"isCron":      1,
		"second":      "0",
		"minute":      "0",
		"hour":        "8",
		"day":         "*",
		"month":       "*",
		"day_of_week": "*",
	}
	if err := s.AddJob(1, jobData, func() {}); err != nil {
		t.Fatalf("AddJob() error: %v", err)
	}

	entry := s.cron.Entry(s.entryID)
	if entry.Next.IsZero() {
		t.Fatal("entry.Next is zero, want scheduled next run time")
	}
	if got := entry.Next.Location().String(); got != "UTC" {
		t.Fatalf("next run location = %q, want UTC", got)
	}
}
