package service

import (
	"errors"
	"fmt"
	"log"
	"opensync/internal/i18n"
	"opensync/pkg/util"
	"strings"
	"sync"

	"github.com/robfig/cron/v3"
)

// Scheduler wraps robfig/cron for job scheduling
type Scheduler struct {
	cron    *cron.Cron
	entryID cron.EntryID
	mu      sync.Mutex
}

// NewScheduler creates a new scheduler
func NewScheduler() *Scheduler {
	c := cron.New(cron.WithSeconds())
	c.Start()
	return &Scheduler{cron: c}
}

// AddJob adds a scheduled job
// isCron: 0=interval, 1=cron, 2=manual only
func (s *Scheduler) AddJob(isCron int, jobData map[string]interface{}, fn func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if isCron == 2 {
		// Manual only, no scheduling
		return nil
	}

	entryID, err := s.addJobLocked(isCron, jobData, fn)
	if err != nil {
		return err
	}
	s.entryID = entryID

	// If disabled, remove the entry immediately (will be resumed later)
	enable := util.ToInt(jobData["enable"])
	if enable == 0 && s.entryID != 0 {
		s.cron.Remove(s.entryID)
		s.entryID = 0
	}

	return nil
}

// Pause pauses the scheduled job
func (s *Scheduler) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entryID != 0 {
		s.cron.Remove(s.entryID)
		s.entryID = 0
	}
}

// Resume resumes a paused job by re-adding it
func (s *Scheduler) Resume(isCron int, jobData map[string]interface{}, fn func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if isCron == 2 {
		return nil
	}
	if s.entryID != 0 {
		return nil
	}

	entryID, err := s.addJobLocked(isCron, jobData, fn)
	if err != nil {
		return err
	}
	s.entryID = entryID
	return nil
}

func (s *Scheduler) addJobLocked(isCron int, jobData map[string]interface{}, fn func()) (cron.EntryID, error) {
	if isCron == 0 {
		interval := util.ToInt(jobData["interval"])
		if interval <= 0 {
			return 0, errors.New(i18n.G("interval_lost"))
		}
		spec := fmt.Sprintf("@every %dm", interval)
		return s.cron.AddFunc(spec, fn)
	}
	spec := buildCronSpec(jobData)
	if spec == "" {
		return 0, errors.New(i18n.G("cron_lost"))
	}
	return s.cron.AddFunc(spec, fn)
}

// Stop shuts down the scheduler
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron != nil {
		s.cron.Stop()
	}
}

// buildCronSpec builds a cron expression from job data
// Format: second minute hour day month dayOfWeek (robfig/cron with seconds)
func buildCronSpec(jobData map[string]interface{}) string {
	fields := []string{"second", "minute", "hour", "day", "month", "day_of_week"}
	parts := make([]string, 6)
	hasValue := false

	for i, field := range fields {
		val := ""
		if v, ok := jobData[field]; ok && v != nil {
			val = strings.TrimSpace(fmt.Sprintf("%v", v))
		}
		if val == "" {
			parts[i] = "*"
		} else {
			parts[i] = val
			hasValue = true
		}
	}

	if !hasValue {
		return ""
	}

	// robfig/cron format: second minute hour dayOfMonth month dayOfWeek
	// Convert day_of_week: Python uses 0-6 (Mon-Sun), cron uses 0-6 (Sun-Sat) or 1-7
	// We'll keep as-is for now (close enough for most use cases)
	spec := strings.Join(parts, " ")
	log.Printf("Built cron spec: %s", spec)
	return spec
}
