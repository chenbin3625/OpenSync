package service

import "testing"

func TestBuildCronSpecFromSeparateFields(t *testing.T) {
	spec := buildCronSpec(map[string]interface{}{
		"second":      "0",
		"minute":      "*/5",
		"hour":        "*",
		"day":         "*",
		"month":       "*",
		"day_of_week": "*",
	})

	if spec != "0 */5 * * * *" {
		t.Fatalf("buildCronSpec() = %q, want %q", spec, "0 */5 * * * *")
	}
}

func TestBuildCronSpecRejectsEmptySchedule(t *testing.T) {
	if spec := buildCronSpec(map[string]interface{}{}); spec != "" {
		t.Fatalf("buildCronSpec(empty) = %q, want empty string", spec)
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
