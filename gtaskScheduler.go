package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/robfig/cron/v3"
)

// Struct to hold job execution status
type JobStatus struct {
	UID               string
	AutoIncrementalID int64
	Command           string
	Timestamp         string
	Status            string
	Output            string
}

// Global log file handle, database handle, and mutex
var (
	logFile *os.File
	db      *sql.DB
	mu      sync.Mutex
)

// Function to initialize the log file
func initLogFile(filePath string) (*os.File, error) {
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("error opening log file: %w", err)
	}
	return file, nil
}

// Function to initialize the SQLite database
func initDatabase(dbPath string) (*sql.DB, error) {
	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("error opening database: %w", err)
	}

	// Create table if not exists
	createTableSQL := `
CREATE TABLE IF NOT EXISTS job_status (
    job_id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT UNIQUE,
    command TEXT,
    timestamp TEXT,
    status TEXT,
    output TEXT
);
	`
	_, err = database.Exec(createTableSQL)
	if err != nil {
		return nil, fmt.Errorf("error creating table: %w", err)
	}
	return database, nil
}

// Function to write job status to the log file and print to terminal
func logJobStatus(jobStatus JobStatus) {
	mu.Lock()
	defer mu.Unlock()

	if logFile == nil {
		return
	}

	logLine := fmt.Sprintf("[%s] Status: %s, Job UID: %s, Command: %s\n", jobStatus.Timestamp, jobStatus.Status, jobStatus.UID, jobStatus.Command)
	if jobStatus.Status == "Failure" {
		logLine += fmt.Sprintf("[%s] Error Occured Status: %s, Job UID: %s\nCommand: %s, Output: %s\n", jobStatus.Timestamp, jobStatus.Status, jobStatus.UID, jobStatus.Command, jobStatus.Output)
	}

	// Print to terminal
	fmt.Print(logLine)

	_, err := logFile.WriteString(logLine)
	if err != nil {
		fmt.Printf("Error writing to log file: %s\n", err)
	}

	// Save the output to an individual log file for each task
	taskLogFilePath := fmt.Sprintf("./logs/%s.log", jobStatus.UID)
	err = ioutil.WriteFile(taskLogFilePath, []byte(logLine), 0644)
	if err != nil {
		fmt.Printf("Error writing task log file: %s\n", err)
	}
}

// Function to log job status into the SQLite database
func logJobStatusToDB(jobStatus JobStatus) {
	mu.Lock()
	defer mu.Unlock()

	if db == nil {
		return
	}

	insertSQL := `INSERT INTO job_status (task_id, command, timestamp, status, output) VALUES (?, ?, ?, ?, ?)`
	result, err := db.Exec(insertSQL, jobStatus.UID, jobStatus.Command, jobStatus.Timestamp, jobStatus.Status, jobStatus.Output)
	if err != nil {
		fmt.Printf("Error inserting into database: %s\n", err)
		return
	}

	// Get the auto-incremental ID
	autoIncrementalID, _ := result.LastInsertId()
	jobStatus.AutoIncrementalID = autoIncrementalID

	// Debug logging for database insertion
	fmt.Printf("Inserted job status into database with Auto Incremental ID: %d\n", jobStatus.AutoIncrementalID)
}

// Function to simulate a job
func job(command string) {
	cmd := exec.Command("bash", "-c", command)
	output, err := cmd.CombinedOutput()

	endTime := time.Now()

	status := "Success"
	if err != nil {
		status = "Failure"
	}

	uid := uuid.New().String()

	jobStatus := JobStatus{
		UID:       uid,
		Command:   command,
		Timestamp: endTime.Format("02-01-2006 15:04:05"), // Custom timestamp format
		Status:    status,
		Output:    string(output),
	}

	logJobStatusToDB(jobStatus)
	logJobStatus(jobStatus)
}

// Function to parse cron job file and schedule jobs
func scheduleJobsFromFile(c *cron.Cron, filePath string) {
	file, err := os.Open(filePath)
	if (err != nil) {
		fmt.Printf("Error opening file: %s\n", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 6 {
			fmt.Printf("Skipping invalid line: %s\n", line)
			continue
		}

		cronExpr := strings.Join(parts[:5], " ")
		command := strings.Join(parts[5:], " ")

		_, err := c.AddFunc(cronExpr, func(cmd string) func() {
			return func() {
				job(cmd)
			}
		}(command))
		var SchedulerLine string
		if err != nil {
			SchedulerLine += fmt.Sprintf("Error scheduling job: %s\n", err)
		} else {
			SchedulerLine += fmt.Sprintf("Scheduled job: %s with cron expression: %s\n", command, cronExpr)
		}
		fmt.Print(SchedulerLine)

		_, err = logFile.WriteString(SchedulerLine)
		if err != nil {
			fmt.Printf("Error writing to log file: %s\n", err)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading file: %s\n", err)
	}
}

// Function to print scheduler start log
func logSchedulerStart() {
	timestamp := time.Now().Format("02-01-2006 15:04:05")
	message := fmt.Sprintf("[%s] Scheduler has started\n", timestamp)
	fmt.Print(message)
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		_, err := logFile.WriteString(message)
		if err != nil {
			fmt.Printf("Error writing to log file: %s\n", err)
		}
	}
}

// Function to get the current date and time
func getCurrentTime() string {
	return time.Now().Format("02-01-2006 15:04:05")
}

// Helper function to check if an option should be selected
func checkSelected(current, option string) string {
	if current == option {
		return "selected"
	}
	return ""
}

// Handler for displaying distinct commands and their last status
func distinctCommandsHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	// Get refresh interval from URL query parameters
	refreshInterval := r.URL.Query().Get("interval")
	if refreshInterval == "" {
		refreshInterval = "5" // default to 5 seconds if no interval specified
	}

	rows, err := db.Query(`
		SELECT command, task_id, MAX(timestamp) AS last_run, 
		       SUM(CASE WHEN status = 'Success' THEN 1 ELSE 0 END) AS success_count,
		       SUM(CASE WHEN status = 'Failure' THEN 1 ELSE 0 END) AS failure_count,
		       output
		FROM job_status
		GROUP BY command
		ORDER BY last_run DESC
	`)
	if err != nil {
		http.Error(w, "Error querying database", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	currentTime := getCurrentTime()

	fmt.Fprintln(w, "<html><body>")
	fmt.Fprintln(w, "<h1>Job Execution Details</h1>")
	fmt.Fprintln(w, "<p>Current Time: ", currentTime, "</p>")
	fmt.Fprintln(w, "<p>Select refresh interval: ")
	fmt.Fprintln(w, "<select id='refreshInterval' onchange='updateRefreshInterval()'>")
	fmt.Fprintln(w, "<option value='5' ", checkSelected(refreshInterval, "5"), ">5s</option>")
	fmt.Fprintln(w, "<option value='10' ", checkSelected(refreshInterval, "10"), ">10s</option>")
	fmt.Fprintln(w, "<option value='30' ", checkSelected(refreshInterval, "30"), ">30s</option>")
	fmt.Fprintln(w, "</select></p>")
	fmt.Fprintln(w, "<table border='1'>")
	fmt.Fprintln(w, "<tr><th>UID</th><th>Command</th><th>Last Run</th><th>Success Count</th><th>Failure Count</th><th>Output</th></tr>")

	for rows.Next() {
		var taskID string
		var command string
		var lastRun string
		var successCount, failureCount int
		var output string

		err := rows.Scan(&command, &taskID, &lastRun, &successCount, &failureCount, &output)
		if err != nil {
			http.Error(w, "Error reading from database", http.StatusInternalServerError)
			return
		}

		if len(output) > 20 {
			// Create a button to download the log file
			fmt.Fprintf(w, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%d</td><td><button onclick=\"downloadLog('%s')\">Download Log</button></td></tr>", taskID, command, lastRun, successCount, failureCount, taskID)
		} else {
			fmt.Fprintf(w, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%d</td><td>%s</td></tr>", taskID, command, lastRun, successCount, failureCount, output)
		}
	}
	fmt.Fprintln(w, "</table>")
	fmt.Fprintln(w, `<script>
	function updateRefreshInterval() {
		var interval = document.getElementById('refreshInterval').value;
		if (interval == 0) {
			interval = 5; // Default to 5 seconds for real-time
		}
		window.location.search = 'interval=' + interval;
	}

	var selectedInterval = document.getElementById('refreshInterval').value;
	setInterval(function() {
		window.location.search = 'interval=' + selectedInterval;
	}, selectedInterval * 1000);

	function downloadLog(taskID) {
		window.location.href = '/download?task_id=' + taskID;
	}
	</script>`)
	fmt.Fprintln(w, "</body></html>")
}

// Handler for downloading logs
func downloadLogHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task_id")

	if taskID == "" {
		http.Error(w, "Task ID not specified", http.StatusBadRequest)
		return
	}

	// Retrieve the log file contents based on taskID
	taskLogFilePath := fmt.Sprintf("./logs/%s.log", taskID)
	logContents, err := ioutil.ReadFile(taskLogFilePath)
	if err != nil {
		http.Error(w, "Error reading log file", http.StatusInternalServerError)
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.log", taskID))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(logContents)
}

// Function to check and create necessary directories
func CheckAndCreateDirs() error {
	directories := []string{"logs", "database"}

	for _, dir := range directories {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			err := os.Mkdir(dir, 0755)
			if err != nil {
				return err
			}
			fmt.Printf("Directory created: %s\n", dir)
		}
	}
	return nil
}

func main() {
	// Check and create necessary directories
	err := CheckAndCreateDirs()
	if err != nil {
		fmt.Printf("Error creating directories: %s\n", err)
		return
	}

	logFile, err = initLogFile("./logs/scheduler.log")
	if err != nil {
		fmt.Printf("Error initializing log file: %s\n", err)
		return
	}
	defer logFile.Close()

	db, err = initDatabase("./database/jobs.db")
	if err != nil {
		fmt.Printf("Error initializing database: %s\n", err)
		return
	}
	defer db.Close()

	c := cron.New()
	scheduleJobsFromFile(c, "cron_jobs.txt")
	c.Start()
	logSchedulerStart()

	http.HandleFunc("/status", distinctCommandsHandler)
	http.HandleFunc("/download", downloadLogHandler)
	err = http.ListenAndServe("localhost:8080", nil)
	if err != nil {
		fmt.Printf("Error starting server: %s\n", err)
		return
	}
}
