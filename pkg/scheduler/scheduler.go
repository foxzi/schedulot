package scheduler

import (
	"fmt"
	"sync"
	"time"

	"github.com/piligrim/gotask2/pkg/executor"
	"github.com/piligrim/gotask2/pkg/logger"
	"github.com/piligrim/gotask2/pkg/task"
	"github.com/robfig/cron/v3"
)

// Scheduler manages and runs tasks.
type Scheduler struct {
	tasks         map[string]task.Task
	executor      *executor.Executor
	logger        *logger.Logger
	cronScheduler *cron.Cron
	taskResults   map[string]taskResult    // Stores the result of the last execution of each task
	mu            sync.RWMutex             // For concurrent access to taskResults
	notifyChan    chan taskExecutionUpdate // Channel to notify about task completion
	stopChan      chan struct{}            // Channel to signal scheduler shutdown
	wg            sync.WaitGroup           // To wait for goroutines to finish
}

type taskResult struct {
	success bool
	output  string
}

type taskExecutionUpdate struct {
	taskID  string
	success bool
	output  string
}

// New creates a new Scheduler.
func New(exec *executor.Executor, l *logger.Logger) *Scheduler {
	return &Scheduler{
		tasks:         make(map[string]task.Task),
		executor:      exec,
		logger:        l,
		cronScheduler: cron.New(cron.WithSeconds()), // Support for second-level precision if needed
		taskResults:   make(map[string]taskResult),
		notifyChan:    make(chan taskExecutionUpdate, 100), // Buffered channel
		stopChan:      make(chan struct{}),
	}
}

// AddTask adds a task to the scheduler.
func (s *Scheduler) AddTask(t task.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tasks[t.ID]; exists {
		return fmt.Errorf("task with ID '%s' already exists", t.ID)
	}
	s.tasks[t.ID] = t
	s.logger.Info(t.ID, "Task added to scheduler")

	for _, trigger := range t.Triggers {
		if trigger.Type == task.TriggerTypeSchedule && trigger.Schedule != "" {
			_, err := s.cronScheduler.AddFunc(trigger.Schedule, func() {
				s.logger.Info(t.ID, "Cron triggered for task")
				s.tryRunTask(t.ID, nil) // Triggered by schedule, no prior dependency
			})
			if err != nil {
				s.logger.Error(t.ID, "Failed to schedule task", err)
				return fmt.Errorf("failed to schedule task %s: %w", t.ID, err)
			}
			s.logger.Info(t.ID, "Task scheduled with cron: %s", trigger.Schedule)
		}
	}
	return nil
}

// tryRunTask attempts to run a task if all its dependencies are met.
// dependentTask is the task that just finished and might trigger this one.
func (s *Scheduler) tryRunTask(taskID string, dependentTask *taskExecutionUpdate) {
	t, exists := s.getTask(taskID)
	if !exists {
		s.logger.Error(taskID, "Attempted to run non-existent task", nil)
		return
	}

	if s.checkDependencies(t, dependentTask) {
		s.runTask(t)
	}
}

// checkDependencies checks if all dependencies for a task are met.
func (s *Scheduler) checkDependencies(t task.Task, completedDependency *taskExecutionUpdate) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(t.DependsOn) == 0 {
		return true // No dependencies
	}

	for _, dep := range t.DependsOn {
		result, ok := s.taskResults[dep.TaskID]
		// If the completedDependency is the one we are checking, use its fresh result
		if completedDependency != nil && dep.TaskID == completedDependency.taskID {
			ok = true
			result = taskResult{success: completedDependency.success, output: completedDependency.output}
		}

		if !ok {
			// s.logger.Info(t.ID, "Dependency %s for task %s not yet met (no result)", dep.TaskID, t.ID)
			return false // Dependency hasn't run yet or its result is not available
		}
		if dep.Status == task.DependencyStatusSuccess && !result.success {
			// s.logger.Info(t.ID, "Dependency %s for task %s not met (required success, got failure)", dep.TaskID, t.ID)
			return false // Required success, but was failure
		}
		if dep.Status == task.DependencyStatusFailure && result.success {
			// s.logger.Info(t.ID, "Dependency %s for task %s not met (required failure, got success)", dep.TaskID, t.ID)
			return false // Required failure, but was success
		}
	}
	s.logger.Info(t.ID, "All dependencies met")
	return true
}

func (s *Scheduler) runTask(t task.Task) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		var output string
		var err error
		var success bool

		for i := 0; i <= t.MaxRetries; i++ {
			if i > 0 {
				s.logger.Info(t.ID, "Retrying task (%d/%d)...", i, t.MaxRetries)
				time.Sleep(time.Second * time.Duration(i*2)) // Simple backoff
			}
			output, err = s.executor.ExecuteTask(t)
			if err == nil {
				success = true
				break
			}
			s.logger.Error(t.ID, fmt.Sprintf("Attempt %d/%d failed", i+1, t.MaxRetries+1), err)
		}

		s.mu.Lock()
		s.taskResults[t.ID] = taskResult{success: success, output: output}
		s.mu.Unlock()

		// Notify that this task has completed
		select {
		case s.notifyChan <- taskExecutionUpdate{taskID: t.ID, success: success, output: output}:
		case <-s.stopChan:
			s.logger.Info(t.ID, "Scheduler stopping, not sending notification")
			return
		}
	}()
}

// Start begins the scheduler's operation.
func (s *Scheduler) Start() {
	s.logger.Info("Scheduler", "Starting scheduler...")
	s.cronScheduler.Start()
	s.logger.Info("Scheduler", "Cron scheduler started.")

	s.wg.Add(1)
	go s.listenForTaskCompletions()

	// Initial check for tasks that might run immediately (e.g. no schedule, dependencies already met)
	s.initialDependencyCheck()
}

func (s *Scheduler) initialDependencyCheck() {
	s.mu.RLock()
	tasksToConsider := make([]task.Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasksToConsider = append(tasksToConsider, t)
	}
	s.mu.RUnlock()

	for _, t := range tasksToConsider {
		// Check if the task has any schedule triggers. If so, cron will handle it.
		hasScheduleTrigger := false
		for _, trigger := range t.Triggers {
			if trigger.Type == task.TriggerTypeSchedule {
				hasScheduleTrigger = true
				break
			}
		}
		if hasScheduleTrigger {
			continue
		}

		// If no schedule trigger, check if it can run based on dependencies.
		s.logger.Info(t.ID, "Performing initial dependency check")
		s.tryRunTask(t.ID, nil)
	}
}

func (s *Scheduler) listenForTaskCompletions() {
	defer s.wg.Done()
	s.logger.Info("SchedulerListener", "Starting to listen for task completions.")
	for {
		select {
		case update := <-s.notifyChan:
			s.logger.Info(update.taskID, "Received completion notification. Success: %v", update.success)
			s.triggerDependents(update)
		case <-s.stopChan:
			s.logger.Info("SchedulerListener", "Stopping listener for task completions.")
			return
		}
	}
}

// triggerDependents finds and tries to run tasks that depend on the completed task.
func (s *Scheduler) triggerDependents(completedTaskUpdate taskExecutionUpdate) {
	s.mu.RLock()
	tasksToConsider := make([]task.Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasksToConsider = append(tasksToConsider, t)
	}
	s.mu.RUnlock()

	for _, t := range tasksToConsider {
		for _, dep := range t.DependsOn {
			if dep.TaskID == completedTaskUpdate.taskID {
				s.logger.Info(t.ID, "Dependency %s completed, checking if task can run", completedTaskUpdate.taskID)
				s.tryRunTask(t.ID, &completedTaskUpdate)
				break // Found the dependency, no need to check other dependencies of this task for this trigger
			}
		}
	}
}

// Stop gracefully shuts down the scheduler.
func (s *Scheduler) Stop() {
	s.logger.Info("Scheduler", "Stopping scheduler...")
	close(s.stopChan)             // Signal all goroutines to stop
	ctx := s.cronScheduler.Stop() // Stop cron jobs
	select {
	case <-ctx.Done():
		s.logger.Info("Scheduler", "Cron scheduler stopped.")
	case <-time.After(5 * time.Second): // Timeout for cron stop
		s.logger.Error("Scheduler", "Cron scheduler stop timed out", nil)
	}
	s.wg.Wait() // Wait for all processing goroutines to finish
	s.logger.Info("Scheduler", "Scheduler stopped gracefully.")
}

func (s *Scheduler) getTask(taskID string) (task.Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, exists := s.tasks[taskID]
	return t, exists
}
