package scheduler

import (
	"fmt"
	"reminder/internal/pkg/logger"
	"sync"

	"github.com/robfig/cron/v3"
)

// Scheduler manages cron jobs.
type Scheduler struct {
	cron *cron.Cron
	log  logger.Logger
	mu   sync.Mutex // To protect access to job management
}

var (
	schedulerInstance *Scheduler
	once              sync.Once
)

// NewScheduler creates a new singleton instance of the cron scheduler.
func NewScheduler(log logger.Logger) *Scheduler {
	once.Do(func() {
		c := cron.New(cron.WithSeconds()) // Use seconds precision
		c.Start()
		log.Info("Cron scheduler started.")
		schedulerInstance = &Scheduler{
			cron: c,
			log:  log,
		}
	})
	return schedulerInstance
}

// AddJob adds a new job to the scheduler.
// spec follows the cron format (e.g., "0 30 * * * *").
// cmd is the function to execute.
// Returns the EntryID of the added job and an error if any.
func (s *Scheduler) AddJob(spec string, cmd func()) (cron.EntryID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, err := s.cron.AddFunc(spec, cmd)
	if err != nil {
		s.log.Error("🔴 ERROR: Failed to add cron job", err)
		return 0, fmt.Errorf("failed to add cron job: %w", err)
	}
	s.log.Info(fmt.Sprintf("Added cron job with ID %d, spec: %s", id, spec))
	return id, nil
}

// RemoveJob removes a job from the scheduler by its EntryID.
func (s *Scheduler) RemoveJob(id cron.EntryID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cron.Remove(id)
	s.log.Info(fmt.Sprintf("Removed cron job with ID %d", id))
}

// Stop stops the cron scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cron != nil {
		ctx := s.cron.Stop()
		<-ctx.Done() // Wait for running jobs to complete
		s.log.Info("Cron scheduler stopped.")
	}
}

// GetEntries returns the list of scheduled entries. Useful for debugging.
func (s *Scheduler) GetEntries() []cron.Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cron.Entries()
}
