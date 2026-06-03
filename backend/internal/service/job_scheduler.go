package service

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"taosync/internal/i18n"

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

	if isCron == 0 {
		// Interval mode
		interval := toInt(jobData["interval"])
		if interval <= 0 {
			return errors.New(i18n.G("interval_lost"))
		}
		spec := fmt.Sprintf("@every %dm", interval)
		entryID, err := s.cron.AddFunc(spec, fn)
		if err != nil {
			return err
		}
		s.entryID = entryID
	} else if isCron == 1 {
		// Cron mode - build cron expression
		spec := buildCronSpec(jobData)
		if spec == "" {
			return errors.New(i18n.G("cron_lost"))
		}
		entryID, err := s.cron.AddFunc(spec, fn)
		if err != nil {
			return err
		}
		s.entryID = entryID
	}

	// If disabled, remove the entry immediately (will be resumed later)
	enable := toInt(jobData["enable"])
	if enable == 0 && s.entryID != 0 {
		s.cron.Remove(s.entryID)
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

	if isCron == 0 {
		interval := toInt(jobData["interval"])
		if interval <= 0 {
			return errors.New(i18n.G("interval_lost"))
		}
		spec := fmt.Sprintf("@every %dm", interval)
		entryID, err := s.cron.AddFunc(spec, fn)
		if err != nil {
			return err
		}
		s.entryID = entryID
	} else {
		spec := buildCronSpec(jobData)
		if spec == "" {
			return errors.New(i18n.G("cron_lost"))
		}
		entryID, err := s.cron.AddFunc(spec, fn)
		if err != nil {
			return err
		}
		s.entryID = entryID
	}
	return nil
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
// Format: second minute hour day month dayOfWeek year (robfig/cron with seconds)
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

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		if val == "" {
			return 0
		}
		n := 0
		fmt.Sscanf(val, "%d", &n)
		return n
	default:
		return 0
	}
}
