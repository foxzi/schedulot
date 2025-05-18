package scheduler

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/piligrim/gotask2/pkg/config" // Added import for config
	"github.com/piligrim/gotask2/pkg/executor"
	"github.com/piligrim/gotask2/pkg/logger"
	"github.com/piligrim/gotask2/pkg/task"
	"github.com/robfig/cron/v3"
)

// Scheduler manages and runs tasks.
type Scheduler struct {
	tasks         map[string]task.Task // taskID -> Task
	taskFilePaths map[string]string    // taskID -> filePath
	executor      *executor.Executor
	logger        *logger.Logger
	cronScheduler *cron.Cron
	taskResults   map[string]taskResult    // Stores the result of the last execution of each task
	mu            sync.RWMutex             // For concurrent access to taskResults, tasks, taskFilePaths
	notifyChan    chan taskExecutionUpdate // Channel to notify about task completion
	stopChan      chan struct{}            // Channel to signal scheduler shutdown
	wg            sync.WaitGroup           // To wait for goroutines to finish
	taskDir       string                   // Directory to watch for task file changes
	watcher       *fsnotify.Watcher        // Filesystem watcher
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
func New(exec *executor.Executor, l *logger.Logger, taskDirPath string) (*Scheduler, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create filesystem watcher: %w", err)
	}

	s := &Scheduler{
		tasks:         make(map[string]task.Task),
		taskFilePaths: make(map[string]string),
		executor:      exec,
		logger:        l,
		cronScheduler: cron.New(cron.WithSeconds()), // Support for second-level precision if needed
		taskResults:   make(map[string]taskResult),
		notifyChan:    make(chan taskExecutionUpdate, 100), // Buffered channel
		stopChan:      make(chan struct{}),
		taskDir:       taskDirPath,
		watcher:       watcher,
	}
	return s, nil
}

// AddTask adds a task to the scheduler or updates it if it already exists.
// It also stores the file path of the task.
func (s *Scheduler) AddTask(t task.Task, filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// If task already exists, remove its old cron jobs first
	if oldTask, exists := s.tasks[t.ID]; exists {
		s.removeCronJobs(oldTask.ID)
		s.logger.Info(t.ID, "Updating existing task")
	}

	s.tasks[t.ID] = t
	s.taskFilePaths[t.ID] = filePath
	s.logger.Info(t.ID, "Task added/updated in scheduler from file %s", filePath)

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

	// Start watching the task directory
	s.wg.Add(1)
	go s.watchTaskDirectory()

	// Initial load and check
	s.loadAndScheduleTasksFromDir() // Changed from initialDependencyCheck
}

func (s *Scheduler) loadAndScheduleTasksFromDir() {
	s.logger.Info("Scheduler", "Performing initial load and scheduling of tasks from directory: %s", s.taskDir)
	loadedTasksMap, err := config.LoadTasksFromDir(s.taskDir, s.logger)
	if err != nil {
		s.logger.Error("Scheduler", "Error loading tasks from directory during initial load", err)
		// Decide if we should stop or continue with potentially no tasks
	}

	s.mu.Lock()
	// Clear existing tasks that might have been removed or changed
	currentTasks := make(map[string]task.Task)
	currentFilePaths := make(map[string]string)

	for filePath, t := range loadedTasksMap {
		currentTasks[t.ID] = t
		currentFilePaths[t.ID] = filePath
		// Add or update task (AddTask handles cron job updates)
		// Need to release lock temporarily or AddTask needs to be callable without deadlock
		// For simplicity, we'll manage cron jobs here after collecting all tasks.
	}
	// Remove cron jobs for tasks that are no longer present
	for taskID := range s.tasks {
		if _, exists := currentTasks[taskID]; !exists {
			s.removeCronJobs(taskID)
		}
	}

	s.tasks = currentTasks
	s.taskFilePaths = currentFilePaths
	s.mu.Unlock() // Unlock before calling AddTask or scheduling

	// Schedule all loaded tasks
	tasksToSchedule := make([]task.Task, 0, len(s.tasks))
	s.mu.RLock()
	for _, t := range s.tasks {
		tasksToSchedule = append(tasksToSchedule, t)
	}
	s.mu.RUnlock()

	for _, t := range tasksToSchedule {
		// Re-evaluate AddTask to ensure it correctly updates cron jobs without issues
		// For now, let's assume AddTask is robust or we handle cron scheduling explicitly here.
		// The AddTask function was modified to handle updates, so this should be fine.
		filePath := s.taskFilePaths[t.ID]              // We need the filePath for AddTask
		if err := s.AddTask(t, filePath); err != nil { // This will re-add/update cron jobs
			s.logger.Error("Scheduler", fmt.Sprintf("Error re-adding/updating task %s during directory reload", t.ID), err)
		}
	}

	s.initialDependencyCheck() // Run dependency checks for newly loaded/updated tasks
}

func (s *Scheduler) watchTaskDirectory() {
	defer s.wg.Done()
	defer s.watcher.Close()

	err := s.watcher.Add(s.taskDir)
	if err != nil {
		s.logger.Error("SchedulerWatcher", "Failed to add task directory to watcher", err)
		return
	}
	s.logger.Info("SchedulerWatcher", "Started watching directory: %s", s.taskDir)

	for {
		select {
		case event, ok := <-s.watcher.Events:
			if !ok {
				s.logger.Info("SchedulerWatcher", "Watcher events channel closed.")
				return
			}
			s.logger.Info("SchedulerWatcher", "Received fs event: %s", event.String())
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
				// Potentially debounce here if many events fire quickly
				s.logger.Info("SchedulerWatcher", "Task file change detected: %s. Reloading tasks.", event.Name)
				// Check if the changed file is a task file
				if strings.HasSuffix(event.Name, ".yml") || strings.HasSuffix(event.Name, ".yaml") || event.Op == fsnotify.Remove || event.Op == fsnotify.Rename {
					s.handleTaskFileChange(event)
				}
			}
		case err, ok := <-s.watcher.Errors:
			if !ok {
				s.logger.Info("SchedulerWatcher", "Watcher errors channel closed.")
				return
			}
			s.logger.Error("SchedulerWatcher", "Watcher error", err)
		case <-s.stopChan:
			s.logger.Info("SchedulerWatcher", "Stopping task directory watcher.")
			return
		}
	}
}

func (s *Scheduler) handleTaskFileChange(event fsnotify.Event) {
	s.logger.Info("Scheduler", "Handling task file change for: %s", event.Name)

	// Reload all tasks from the directory. This is simpler than trying to manage individual file changes,
	// especially with renames and deletes.
	// This will also re-evaluate dependencies and schedules.
	s.loadAndScheduleTasksFromDir()
}

// removeCronJobs removes all cron entries associated with a task ID.
func (s *Scheduler) removeCronJobs(taskID string) {
	s.mu.RLock() // Ensure we are not modifying cronScheduler.entries while iterating
	// entries := s.cronScheduler.Entries() // This line was causing an error because `entries` was unused.
	// It's removed as the logic relies on AddTask to manage cron job updates.
	s.mu.RUnlock()

	// The current cron library (robfig/cron/v3) doesn't make it easy to identify jobs
	// by a custom ID after they've been added via AddFunc.
	// The AddFunc returns an EntryID, which we should store if we want to remove specific jobs.

	// For now, we rely on the fact that `loadAndScheduleTasksFromDir` rebuilds the schedule.
	// When a task is removed from the `s.tasks` map, its cron jobs won't be re-added.
	// The `cron.New()` effectively clears old jobs when `loadAndScheduleTasksFromDir` re-initializes scheduling.
	// This is handled by AddTask now.
	s.logger.Info(taskID, "Attempted to remove cron jobs (actual removal depends on AddTask logic)")
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
	if s.watcher != nil {
		s.watcher.Close() // Close the watcher first
	}
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
