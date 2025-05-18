# Go Task Manager

Это простое приложение для управления задачами, написанное на Go. Оно позволяет определять задачи в YAML файлах, запускать их по расписанию или в зависимости от результатов выполнения других задач. Поддерживается выполнение команд оболочки и HTTP запросов.

## Возможности

*   Определение задач в YAML файлах.
*   Запуск задач по расписанию (cron-подобный синтаксис).
*   Запуск задач на основе успешного или неуспешного выполнения других задач.
*   Выполнение команд оболочки.
*   Выполнение HTTP запросов.
*   Параллельное выполнение задач.
*   Логирование результатов выполнения задач.
*   Настройка максимального количества повторных попыток и таймаутов для задач.
*   **Динамическая перезагрузка задач:** Автоматическое обнаружение изменений (создание, обновление, удаление) в файлах задач в указанной директории и соответствующая перезагрузка/обновление задач в планировщике без перезапуска приложения.
*   **Валидация конфигурации задач:** Возможность запуска приложения с флагом для проверки синтаксиса и корректности файлов конфигурации задач.

## Структура проекта

```
gotask2/
├── cmd/                    # Основное приложение
│   └── main.go
├── pkg/                    # Основные пакеты
│   ├── config/             # Загрузка конфигурации задач
│   ├── executor/           # Выполнение задач
│   ├── logger/             # Логирование
│   ├── scheduler/          # Планировщик задач
│   └── task/               # Определения структур задач
├── tasks/                  # Директория для YAML файлов с задачами
├── go.mod
├── go.sum
└── README.md
└── app.log                 # Файл логов (создается при первом запуске)
```

## Предварительные требования

*   Go (версия 1.18 или выше)

## Установка и запуск

1.  **Клонируйте репозиторий (если применимо) или убедитесь, что у вас есть все файлы проекта.**

2.  **Перейдите в директорию проекта:**
    ```bash
    cd gotask2
    ```

3.  **Загрузите зависимости (включая `fsnotify` для отслеживания файлов):**
    ```bash
    go mod tidy
    ```

4.  **Соберите проект (опционально, можно запускать через `go run`):**
    ```bash
    go build -o taskmanager cmd/main.go
    ```

5.  **Запустите приложение:**
    *   Через собранный бинарный файл:
        ```bash
        ./taskmanager [флаги]
        ```
    *   Или напрямую:
        ```bash
        go run cmd/main.go [флаги]
        ```

    Приложение поддерживает следующие флаги командной строки:
    *   `-logfile <путь>`: Путь к файлу логов (по умолчанию: `app.log`).
    *   `-tasksdir <путь>`: Директория, содержащая YAML файлы задач (по умолчанию: `./tasks`).
    *   `-validate`: Если указан, приложение проверит конфигурацию задач в `tasksdir` и завершит работу.

    **Примеры запуска:**
    *   Обычный запуск:
        ```bash
        ./taskmanager
        ```
    *   Запуск с указанием директории задач и файла логов:
        ```bash
        ./taskmanager -tasksdir /etc/myapp/tasks -logfile /var/log/myapp.log
        ```
    *   Запуск в режиме валидации задач:
        ```bash
        ./taskmanager -validate
        ```
        Или с указанием конкретной директории для валидации:
        ```bash
        ./taskmanager -validate -tasksdir ./my_specific_tasks
        ```

    По умолчанию приложение будет искать задачи в директории `./tasks/`, логировать в `app.log` и отслеживать изменения в директории задач для автоматической перезагрузки.

## Конфигурация задач

Задачи определяются в YAML файлах (с расширением `.yml` или `.yaml`) в директории `tasks/`. Каждая задача должна иметь уникальный `id`.

### Структура YAML файла задачи:

```yaml
id: "unique-task-id"
description: "Краткое описание задачи (опционально)."
max_retries: 3 # Опционально, количество повторных попыток при неудаче (по умолчанию 0)
timeout: "30s" # Опционально, таймаут на выполнение задачи, например "10s", "1m", "500ms" (по умолчанию нет таймаута)

triggers:
  - type: "schedule" # Тип триггера: "schedule" или "dependency"
    schedule: "*/5 * * * * *" # Cron выражение (секунды минуты часы дни_месяца месяцы дни_недели)
  # Можно указать несколько триггеров

depends_on: # Опционально, список зависимостей от других задач
  - task_id: "another-task-id"
    status: "success" # Требуемый статус: "success" или "failure"
  # Можно указать несколько зависимостей

action:
  type: "command" # Тип действия: "command" или "http"
  command: # Используется, если type = "command"
    path: "echo"
    args: ["Привет из задачи!"]
    # dir: "/tmp" # Опционально, рабочая директория для команды
  http_request: # Используется, если type = "http"
    url: "https://api.example.com/data"
    method: "GET" # "GET", "POST", "PUT", "DELETE", etc.
    headers: # Опционально
      Content-Type: "application/json"
      Authorization: "Bearer your_token"
    body: '''{"key": "value"}''' # Опционально, тело запроса (для POST, PUT)
```

### Поля задачи:

*   `id` (string, обязательное): Уникальный идентификатор задачи.
*   `description` (string, опциональное): Описание задачи.
*   `max_retries` (int, опциональное): Максимальное количество повторных попыток выполнения задачи в случае неудачи. По умолчанию `0`.
*   `timeout` (string, опциональное): Таймаут на выполнение задачи. Формат строки как в `time.ParseDuration` (например, "30s", "5m", "1h30m"). Если не указан, таймаута нет.
*   `triggers` (list, обязательное): Список триггеров для запуска задачи.
    *   `type` (string, обязательное):
        *   `schedule`: Запускает задачу по расписанию.
        *   `dependency`: Запускает задачу при выполнении условия зависимости (этот тип используется неявно через `depends_on`, явно указывать его в `triggers` не нужно, если задача запускается *только* по зависимости). Если задача должна запускаться и по расписанию, и иметь возможность быть запущенной по зависимости, то указывается `schedule`.
    *   `schedule` (string, опциональное, если `type: "schedule"`): Cron-выражение для расписания. Формат: `секунды минуты часы дни_месяца месяцы дни_недели`. Пример: `0 * * * * *` (каждую минуту), `0 0 1 * * *` (каждый день в 01:00:00).
*   `depends_on` (list, опциональное): Список задач, от которых зависит текущая задача.
    *   `task_id` (string, обязательное): `id` задачи, от которой есть зависимость.
    *   `status` (string, обязательное): Требуемый статус зависимой задачи для запуска текущей.
        *   `success`: Зависимая задача должна завершиться успешно.
        *   `failure`: Зависимая задача должна завершиться с ошибкой.
*   `action` (object, обязательное): Действие, которое выполняет задача.
    *   `type` (string, обязательное):
        *   `command`: Выполнить команду оболочки.
        *   `http`: Выполнить HTTP запрос.
    *   `command` (object, если `action.type: "command"`):
        *   `path` (string, обязательное): Путь к исполняемому файлу или команда.
        *   `args` (list of strings, опциональное): Аргументы команды.
        *   `dir` (string, опциональное): Рабочая директория для выполнения команды.
    *   `http_request` (object, если `action.type: "http"`):
        *   `url` (string, обязательное): URL для HTTP запроса.
        *   `method` (string, обязательное): HTTP метод (GET, POST, PUT, DELETE и т.д.).
        *   `headers` (map, опциональное): Заголовки HTTP запроса.
        *   `body` (string, опциональное): Тело HTTP запроса (например, для POST или PUT).

## Логирование

Все действия, запуск задач, результаты выполнения и ошибки логируются в файл `app.log` в корневой директории проекта.

## Примеры задач

Создайте следующие файлы в директории `tasks/`:

### `tasks/task1-echo-schedule.yml`

Эта задача будет выполняться каждые 10 секунд и выводить сообщение.

```yaml
id: "task1-echo-schedule"
description: "Простая задача echo, запускаемая по расписанию."
triggers:
  - type: "schedule"
    schedule: "*/10 * * * * *" # Каждые 10 секунд
action:
  type: "command"
  command:
    path: "echo"
    args: ["Привет от task1! Время:", "$(date)"]
```

### `tasks/task2-http-get.yml`

Эта задача выполнит GET-запрос к `httpbin.org` каждую минуту.

```yaml
id: "task2-http-get"
description: "Задача для выполнения HTTP GET запроса."
triggers:
  - type: "schedule"
    schedule: "0 */1 * * * *" # Каждую минуту
action:
  type: "http"
  http_request:
    url: "https://httpbin.org/get?param=value"
    method: "GET"
    headers:
      X-Custom-Header: "GoTaskManager"
```

### `tasks/task3-dependent-success.yml`

Эта задача запустится только после успешного выполнения `task1-echo-schedule`.

```yaml
id: "task3-dependent-success"
description: "Задача, зависящая от успешного выполнения task1."
depends_on:
  - task_id: "task1-echo-schedule"
    status: "success"
action:
  type: "command"
  command:
    path: "echo"
    args: ["task1 успешно выполнена, запускаю task3!"]
```

### `tasks/task4-dependent-failure-with-retry.yml`

Эта задача попытается выполниться, если `non-existent-task` (которой нет) завершится неудачей (что всегда будет так, так как она не будет найдена или не сможет запуститься).
Она также настроена на повторные попытки.

```yaml
id: "task4-command-fail"
description: "Команда, которая заведомо завершится ошибкой."
max_retries: 2
timeout: "5s"
triggers:
  - type: "schedule"
    schedule: "*/30 * * * * *" # Каждые 30 секунд, для демонстрации
action:
  type: "command"
  command:
    path: "ls"
    args: ["/non/existent/path"] # Эта команда вызовет ошибку
```

### `tasks/task5-dependent-on-task4-failure.yml`

Эта задача запустится после того, как `task4-command-fail` завершится неудачей.

```yaml
id: "task5-dependent-on-task4-failure"
description: "Задача, зависящая от НЕУСПЕШНОГО выполнения task4."
depends_on:
  - task_id: "task4-command-fail"
    status: "failure"
action:
  type: "command"
  command:
    path: "echo"
    args: ["task4 завершилась с ошибкой, как и ожидалось! Запускаю task5."]
```

### `tasks/task6-long-running-with-timeout.yml`

Эта задача имитирует долгую операцию, которая будет прервана по таймауту.

```yaml
id: "task6-long-running-with-timeout"
description: "Задача, которая будет прервана по таймауту."
timeout: "3s" # Таймаут 3 секунды
triggers:
  - type: "schedule"
    schedule: "*/20 * * * * *" # Каждые 20 секунд
action:
  type: "command"
  command:
    path: "sleep"
    args: ["10"] # Команда будет спать 10 секунд
```

После создания этих файлов и запуска `taskmanager`, вы увидите логи их выполнения в `app.log`.

## Сборка и запуск основного приложения (`cmd/main.go`)

Файл `cmd/main.go` должен инициализировать логгер, загрузчик конфигурации, исполнитель и планировщик, а затем запустить планировщик.

Примерное содержимое `cmd/main.go`:

```go
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/piligrim/gotask2/pkg/config"
	"github.com/piligrim/gotask2/pkg/executor"
	"github.com/piligrim/gotask2/pkg/logger"
	"github.com/piligrim/gotask2/pkg/scheduler"
)

const (
	logFilePath    = "app.log"
	tasksDirPath   = "./tasks"
)

func main() {
	appLogger, err := logger.New(logFilePath)
	if (err != nil) {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	appLogger.Info("Main", "Application starting...")

	taskExecutor := executor.New(appLogger)
	taskScheduler := scheduler.New(taskExecutor, appLogger)

	loadedTasks, err := config.LoadTasksFromDir(tasksDirPath)
	if (err != nil) {
		appLogger.Error("Main", "Failed to load tasks", err)
		os.Exit(1)
	}

	if len(loadedTasks) == 0 {
		appLogger.Info("Main", "No tasks found in %s. The application will run but will not schedule any tasks.", tasksDirPath)
	} else {
		for _, t := range loadedTasks {
			if err := taskScheduler.AddTask(t); err != nil {
				appLogger.Error("Main", fmt.Sprintf("Failed to add task %s", t.ID), err)
			}
		}
	}

	taskScheduler.Start()
	appLogger.Info("Main", "Task scheduler started. Press Ctrl+C to exit.")

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	appLogger.Info("Main", "Shutdown signal received. Stopping scheduler...")
	taskScheduler.Stop()
	appLogger.Info("Main", "Application stopped.")
}

```

Не забудьте создать файл `cmd/main.go` с этим содержимым, если его еще нет.
Также убедитесь, что ваш `go.mod` файл содержит необходимые зависимости, например:
```
go get github.com/robfig/cron/v3
go get gopkg.in/yaml.v3
go get github.com/fsnotify/fsnotify
```
Выполните `go mod tidy` после добавления зависимостей.
