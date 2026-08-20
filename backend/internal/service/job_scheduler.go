package service

import (
	"errors"
	"fmt"
	"log"
	"opensync/internal/msg"
	"opensync/pkg/util"
	"os"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"github.com/robfig/cron/v3"
)

const defaultSchedulerTimeZone = "Asia/Shanghai"
const schedulerStopTimeout = 5 * time.Second

// Scheduler wraps robfig/cron for job scheduling
type Scheduler struct {
	cron    *cron.Cron
	entryID cron.EntryID
	mu      sync.Mutex
}

// NewScheduler creates a new scheduler
func NewScheduler() *Scheduler {
	loc := schedulerLocation()
	c := cron.New(cron.WithSeconds(), cron.WithLocation(loc))
	c.Start()
	logSchedulerTimezone(loc)
	return &Scheduler{cron: c}
}

var logSchedulerTimezoneOnce sync.Once

// logSchedulerTimezone logs the scheduler timezone once at first scheduler
// creation so deployments can confirm cron semantics are correct (TZ vs
// config timezone); repeated scheduler rebuilds do not spam the log.
func logSchedulerTimezone(loc *time.Location) {
	logSchedulerTimezoneOnce.Do(func() {
		log.Printf("调度时区 / scheduler timezone = %s", loc)
	})
}

func schedulerLocation() *time.Location {
	timeZone := strings.TrimSpace(os.Getenv("TZ"))
	if timeZone == "" {
		timeZone = defaultSchedulerTimeZone
	}

	loc, err := time.LoadLocation(timeZone)
	if err == nil {
		return loc
	}

	if timeZone != defaultSchedulerTimeZone {
		log.Printf("Failed to load timezone %q, falling back to %s: %v", timeZone, defaultSchedulerTimeZone, err)
	}
	loc, err = time.LoadLocation(defaultSchedulerTimeZone)
	if err == nil {
		return loc
	}

	log.Printf("Failed to load timezone %q, falling back to fixed UTC+8: %v", defaultSchedulerTimeZone, err)
	return time.FixedZone(defaultSchedulerTimeZone, 8*60*60)
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
	if s.cron == nil {
		return
	}
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
	if s.cron == nil {
		return errors.New(msg.CannotResumeLostJob)
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
	if s.cron == nil {
		return 0, errors.New(msg.CannotResumeLostJob)
	}
	if isCron == 0 {
		interval := util.ToInt(jobData["interval"])
		if interval <= 0 {
			return 0, errors.New(msg.IntervalLost)
		}
		spec := fmt.Sprintf("@every %dm", interval)
		return s.cron.AddFunc(spec, fn)
	}
	spec, err := buildCronSpec(jobData)
	if err != nil {
		return 0, err
	}
	if spec == "" {
		return 0, errors.New(msg.CronLost)
	}
	entryID, err := s.cron.AddFunc(spec, fn)
	if err != nil {
		return 0, err
	}
	log.Printf("Built cron spec: %q", spec)
	return entryID, nil
}

// Stop shuts down the scheduler
func (s *Scheduler) Stop() {
	s.mu.Lock()
	c := s.cron
	s.cron = nil
	s.entryID = 0
	s.mu.Unlock()

	if c == nil {
		return
	}
	ctx := c.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(schedulerStopTimeout):
		log.Printf("scheduler stop timed out after %s", schedulerStopTimeout)
	}
}

// buildCronSpec builds a cron expression from job data
// Format: second minute hour day month dayOfWeek (robfig/cron with seconds)
func buildCronSpec(jobData map[string]interface{}) (string, error) {
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
			if !isSafeCronField(val) {
				return "", errors.New(msg.CronLost)
			}
			parts[i] = val
			hasValue = true
		}
	}

	if !hasValue {
		return "", nil
	}

	// robfig/cron format: second minute hour dayOfMonth month dayOfWeek.
	// day_of_week is a free-text cron field on the frontend, so users enter
	// cron-standard values directly (0=Sunday..6=Saturday) and no conversion
	// is needed. The values are passed through unchanged.
	return strings.Join(parts, " "), nil
}

func isSafeCronField(value string) bool {
	for _, r := range value {
		if r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '*', '/', ',', '-':
			continue
		default:
			return false
		}
	}
	return true
}
