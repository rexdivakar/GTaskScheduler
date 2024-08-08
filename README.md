# GtaskScheduler

## Cron Job Scheduler with Logging and Database Tracking

This Go program is a cron job scheduler that reads job definitions from a file, executes the jobs at the scheduled times, logs the execution details, and tracks the job status in an SQLite database. Additionally, it provides a web interface to display the job statuses and allows downloading of job logs.

## Features

- Schedules jobs using cron expressions from a specified file.
- Logs job execution details to a log file.
- Tracks job execution status in an SQLite database.
- Provides a web interface to display job statuses.
- Allows downloading of detailed job logs.

## Requirements

- Go (version 1.16 or later)
- SQLite3
- Go packages: `github.com/google/uuid`, `github.com/mattn/go-sqlite3`, `github.com/robfig/cron/v3`, `go get github.com/joho/godotenv`

## Installation

1. Install Go from the [official website](https://golang.org/dl/).
2. Install SQLite3 from [SQLite's download page](https://www.sqlite.org/download.html).
3. Install the required Go packages:

```sh
go get github.com/google/uuid
go get github.com/mattn/go-sqlite3
go get github.com/robfig/cron/v3
go get github.com/joho/godotenv
```

### Main Components

- JobStatus Struct: Holds details of each job execution, including the command, timestamp, status, output, duration, and CPU utilization.

Initialization Functions:

- initLogFile: Initializes the log file.
- initDatabase: Initializes the SQLite database and creates the job_status table if it doesn't exist.

Logging Functions:

- logJobStatus: Writes job status to the log file.
- logJobStatusToDB: Writes job status to the SQLite database.

Job Execution:

- job: Executes a shell command, logs the status, and calculates the execution duration and CPU utilization.

Scheduling:

- scheduleJobsFromFile: Reads the job definitions from the file and schedules them using cron.

Web Handlers:

- distinctCommandsHandler: Displays distinct commands and their statuses on the web interface.
- downloadLogHandler: Allows downloading of logs for specific jobs.
