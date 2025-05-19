package scheduler

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/piligrim/gotask2/pkg/config"
	"github.com/piligrim/gotask2/pkg/executor"
	"github.com/piligrim/gotask2/pkg/logger"
	"github.com/piligrim/gotask2/pkg/notification"
	"github.com/piligrim/gotask2/pkg/task"
	"github.com/robfig/cron/v3"

	fileNotifierPlugin "github.com/piligrim/gotask2/pkg/plugins/notification/file" // Renamed alias for clarity
	// Import other notifier plugins as they are implemented
	// "github.com/piligrim/gotask2/pkg/plugins/notification/email"
	// "github.com/piligrim/gotask2/pkg/plugins/notification/slack"
	// "github.com/piligrim/gotask2/pkg/plugins/notification/telegram"
)

// Scheduler manages and runs tasks.
type Scheduler struct {
	tasks            map[string]task.Task // taskID -> Task
	taskFilePaths    map[string]string    // taskID -> filePath
	executor         *executor.Executor
	logger           *logger.Logger
	cronScheduler    *cron.Cron
	taskResults      map[string]taskResult     // Stores the result of the last execution of each task
	mu               sync.RWMutex              // For concurrent access to taskResults, tasks, taskFilePaths, taskCronEntryIDs
	notifyChan       chan TaskExecutionUpdate  // Channel to notify about task completion
	stopChan         chan struct{}             // Channel to signal scheduler shutdown
	wg               sync.WaitGroup            // To wait for goroutines to finish
	taskDir          string                    // Directory to watch for task file changes
	watcher          *fsnotify.Watcher         // Filesystem watcher
	taskCronEntryIDs map[string][]cron.EntryID // taskID -> list of EntryIDs for its schedule triggers
}

type taskResult struct {
	success bool
	output  string
}

// TaskExecutionUpdate holds information about a completed task execution.
// Fields are exported (capitalized) to be accessible by other packages if needed,
// though primarily used internally by the scheduler now.
type TaskExecutionUpdate struct {
	TaskID  string
	Success bool
	Output  string
}

// New creates a new Scheduler.
func New(exec *executor.Executor, l *logger.Logger, taskDirPath string) (*Scheduler, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create filesystem watcher: %w", err)
	}

	s := &Scheduler{
		tasks:            make(map[string]task.Task),
		taskFilePaths:    make(map[string]string),
		executor:         exec,
		logger:           l,
		cronScheduler:    cron.New(cron.WithSeconds()), // Support for second-level precision if needed
		taskResults:      make(map[string]taskResult),
		notifyChan:       make(chan TaskExecutionUpdate, 100), // Buffered channel, uses exported struct
		stopChan:         make(chan struct{}),
		taskDir:          taskDirPath,
		watcher:          watcher,
		taskCronEntryIDs: make(map[string][]cron.EntryID), // Initialize the new map
	}
	return s, nil
}

// AddTask adds a task to the scheduler or updates it if it already exists.
// It also stores the file path of the task and manages cron entries.
func (s *Scheduler) AddTask(t task.Task, filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing cron jobs for this specific task ID before adding new ones
	if entryIDs, ok := s.taskCronEntryIDs[t.ID]; ok {
		for _, entryID := range entryIDs {
			s.cronScheduler.Remove(entryID)
		}
		s.logger.Info(t.ID, "Removed %d old cron entries for task", len(entryIDs))
	}
	// Ensure the entry for this task ID is clean before adding new IDs
	s.taskCronEntryIDs[t.ID] = []cron.EntryID{}

	if _, exists := s.tasks[t.ID]; exists {
		s.logger.Info(t.ID, "Updating existing task definition in scheduler map")
	}

	s.tasks[t.ID] = t
	s.taskFilePaths[t.ID] = filePath
	s.logger.Info(t.ID, "Task added/updated in scheduler's internal map from file %s", filePath)

	var currentTaskEntryIDs []cron.EntryID
	for _, trigger := range t.Triggers {
		if trigger.Type == task.TriggerTypeSchedule && trigger.Schedule != "" {
			entryID, err := s.cronScheduler.AddFunc(trigger.Schedule, func() {
				// It's crucial that the taskID used here is from the loop variable 't'
				// to ensure the correct taskID is captured in the closure.
				taskIDForClosure := t.ID
				s.logger.Info(taskIDForClosure, "Cron triggered for task")
				s.tryRunTask(taskIDForClosure, nil) // Triggered by schedule, no prior dependency
			})
			if err != nil {
				s.logger.Error(t.ID, "Failed to schedule task with cron", err)
				// Consider how to handle partial failures. For now, return error.
				return fmt.Errorf("failed to schedule task %s with cron: %s: %w", t.ID, trigger.Schedule, err)
			}
			currentTaskEntryIDs = append(currentTaskEntryIDs, entryID)
			s.logger.Info(t.ID, "Task scheduled with cron: %s, EntryID: %d", trigger.Schedule, entryID)
		}
	}
	s.taskCronEntryIDs[t.ID] = currentTaskEntryIDs // Store the new entry IDs for this task
	return nil
}

// tryRunTask attempts to run a task if all its dependencies are met.
// dependentTask is the task that just finished and might trigger this one.
func (s *Scheduler) tryRunTask(taskID string, dependentTask *TaskExecutionUpdate) {
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
func (s *Scheduler) checkDependencies(t task.Task, completedDependency *TaskExecutionUpdate) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(t.DependsOn) == 0 {
		return true // No dependencies
	}

	for _, dep := range t.DependsOn {
		result, ok := s.taskResults[dep.TaskID]
		// If the completedDependency is the one we are checking, use its fresh result
		if completedDependency != nil && dep.TaskID == completedDependency.TaskID {
			ok = true
			result = taskResult{success: completedDependency.Success, output: completedDependency.Output}
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
		case s.notifyChan <- TaskExecutionUpdate{TaskID: t.ID, Success: success, Output: output}: // Use exported struct
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
		// For now, we continue, potentially with an empty task set.
	}

	s.mu.Lock() // Lock for modifying shared scheduler state (tasks, taskFilePaths, taskCronEntryIDs)

	newlyLoadedTaskIDs := make(map[string]bool)
	for _, t := range loadedTasksMap {
		newlyLoadedTaskIDs[t.ID] = true
	}

	// Identify and remove tasks (and their cron jobs) that no longer exist in the loaded files
	for taskIDFromOldSet := range s.tasks {
		if _, stillExists := newlyLoadedTaskIDs[taskIDFromOldSet]; !stillExists {
			s.logger.Info("Scheduler", "Task %s is no longer present in config files. Removing it and its cron jobs.", taskIDFromOldSet)
			s.removeCronJobs(taskIDFromOldSet) // This will use stored EntryIDs and clean up taskCronEntryIDs map
			delete(s.tasks, taskIDFromOldSet)
			delete(s.taskFilePaths, taskIDFromOldSet)
			// Also remove from taskResults if desired, though not strictly necessary for scheduling
			// delete(s.taskResults, taskIDFromOldSet)
		}
	}

	// Update s.tasks and s.taskFilePaths with the newly loaded tasks
	// Clear and repopulate to ensure consistency
	s.tasks = make(map[string]task.Task)
	s.taskFilePaths = make(map[string]string)
	for filePath, t := range loadedTasksMap {
		s.tasks[t.ID] = t
		s.taskFilePaths[t.ID] = filePath
	}

	s.mu.Unlock() // Unlock before calling AddTask in a loop, as AddTask acquires its own lock.

	// Add or Update tasks in the scheduler, which will handle their cron job (re)scheduling.
	// Iterate over the tasks that were actually loaded from disk.
	for _, t := range loadedTasksMap { // Iterate using loadedTasksMap to ensure we use the latest definitions
		filePath := s.taskFilePaths[t.ID]              // Get the filepath from the updated map (or could get from loadedTasksMap's key)
		if err := s.AddTask(t, filePath); err != nil { // AddTask now correctly handles its own cron entries
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
// This function expects s.mu to be held by the caller if necessary (e.g. when modifying s.taskCronEntryIDs).
func (s *Scheduler) removeCronJobs(taskID string) {
	// s.mu.Lock() // Caller should handle locking if concurrent access to taskCronEntryIDs is possible without it
	// defer s.mu.Unlock()

	if entryIDs, ok := s.taskCronEntryIDs[taskID]; ok {
		s.logger.Info(taskID, "Removing %d cron entries for task", len(entryIDs))
		for _, entryID := range entryIDs {
			s.cronScheduler.Remove(entryID)
		}
		delete(s.taskCronEntryIDs, taskID) // Clean up the map entry for this task
		s.logger.Info(taskID, "Successfully removed cron entries and map key for task", taskID)
	} else {
		s.logger.Info(taskID, "No cron entries found in map to remove for task")
	}
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
	s.logger.Info("Scheduler", "Listening for task completions to trigger dependent tasks and notifications.")

	for {
		select {
		case update := <-s.notifyChan: // Receives TaskExecutionUpdate
			s.logger.Info(update.TaskID, "Scheduler: Task completed. Success: %v, Output: %s", update.Success, update.Output)

			// Process notifications for the completed task
			if update.Success {
				completedTask, exists := s.getTask(update.TaskID) // getTask is an internal helper
				if !exists {
					s.logger.Error(update.TaskID, "Scheduler: Task definition not found for notification processing", nil)
					continue
				}

				if completedTask.Notify != nil && len(completedTask.Notify) > 0 {
					s.logger.Info(completedTask.ID, "Scheduler: Processing %d notification(s) for task", len(completedTask.Notify))

					// Prepare data for notification.Notifier interface (map[string]interface{})
					// The ProcessTemplate function in notification package uses notification.TemplateData struct.
					// Notifier plugins will call ProcessTemplate with the correct struct.
					notificationDataMap := map[string]interface{}{
						"TaskID": completedTask.ID,
						"Date":   time.Now().Format(time.RFC3339),
						"Data":   update.Output,
					}

					for _, notifyCfg := range completedTask.Notify {
						s.logger.Info(completedTask.ID, "Scheduler: Attempting notification type: %s", notifyCfg.Type)
						var notifierPlugin notification.Notifier
						var err error

						switch notifyCfg.Type {
						case "file":
							if notifyCfg.FileNotification.FilePath != "" {
								// Use the imported alias for the file notifier plugin
								notifierPlugin = fileNotifierPlugin.NewFileNotifier(notifyCfg.FileNotification)
							} else {
								s.logger.Error(completedTask.ID, "Scheduler: File notification configured but FilePath is empty", nil)
								continue
							}
						case "email":
							s.logger.Info(completedTask.ID, "Scheduler: Email notification type encountered (implementation pending). Recipient(s): %v", notifyCfg.EmailNotification.To)
							// notifierPlugin = emailNotifier.NewEmailNotifier(notifyCfg.EmailNotification, "smtp.example.com", "587", "user", "pass")
							continue // Skip until implemented
						case "slack":
							s.logger.Info(completedTask.ID, "Scheduler: Slack notification type encountered (implementation pending). Webhook: %s", notifyCfg.SlackNotification.WebhookURL)
							// notifierPlugin = slackNotifier.NewSlackNotifier(notifyCfg.SlackNotification)
							continue // Skip until implemented
						case "telegram":
							s.logger.Info(completedTask.ID, "Scheduler: Telegram notification type encountered (implementation pending). ChatID: %s", notifyCfg.TelegramNotification.ChatID)
							// notifierPlugin = telegramNotifier.NewTelegramNotifier(notifyCfg.TelegramNotification)
							continue // Skip until implemented
						default:
							s.logger.Error(completedTask.ID, fmt.Sprintf("Scheduler: Unsupported notification type: %s", notifyCfg.Type), nil)
							continue
						}

						if notifierPlugin != nil {
							// Pass the map[string]interface{} to the Notify method
							err = notifierPlugin.Notify(notificationDataMap)
							if err != nil {
								s.logger.Error(completedTask.ID, fmt.Sprintf("Scheduler: Failed to send %s notification", notifyCfg.Type), err)
							} else {
								s.logger.Info(completedTask.ID, fmt.Sprintf("Scheduler: Successfully sent %s notification", notifyCfg.Type))
							}
						}
					}
				} else {
					s.logger.Info(completedTask.ID, "Scheduler: No notifications configured for this task.")
				}
			}

			// Trigger dependent tasks using the received update (which has exported fields)
			s.triggerDependentTasks(update.TaskID, update.Success, update.Output)

		case <-s.stopChan:
			s.logger.Info("Scheduler", "Stopping task completion listener.")
			return
		}
	}
}

// getTask retrieves a task definition by its ID. It's an internal helper.
func (s *Scheduler) getTask(taskID string) (task.Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, exists := s.tasks[taskID]
	return t, exists
}

func (s *Scheduler) triggerDependentTasks(completedTaskID string, success bool, output string) {
	s.mu.RLock()
	tasksToConsider := make([]task.Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasksToConsider = append(tasksToConsider, t)
	}
	s.mu.RUnlock()

	for _, t := range tasksToConsider {
		for _, dep := range t.DependsOn {
			if dep.TaskID == completedTaskID {
				// Pass the actual execution update for the dependency check
				s.tryRunTask(t.ID, &TaskExecutionUpdate{TaskID: completedTaskID, Success: success, Output: output})
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
