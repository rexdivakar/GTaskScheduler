package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
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
CREATE TABLE IF NOT EXISTS tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_uid TEXT UNIQUE,
    command TEXT,
    timestamp TEXT,
    status TEXT,
    output TEXT
);

CREATE TABLE IF NOT EXISTS jobs (
    job_id INTEGER PRIMARY KEY AUTOINCREMENT,
	job_name TEXT UNIQUE,
	job_schedule TEXT,
	job_command TEXT,
    job_description TEXT,
    added_at TEXT,
	job_status Boolean DEFAULT 1,
	updated_at TEXT
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
}

// Function to log job status into the SQLite database
func logJobStatusToDB(jobStatus JobStatus) {
	mu.Lock()
	defer mu.Unlock()

	if db == nil {
		return
	}

	insertTasksTable := `INSERT INTO tasks (task_uid, command, timestamp, status, output) VALUES (?, ?, ?, ?, ?)`
	result, err := db.Exec(insertTasksTable, jobStatus.UID, jobStatus.Command, jobStatus.Timestamp, jobStatus.Status, jobStatus.Output)
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

// // Function to parse cron job file and schedule jobs
// func scheduleJobsFromFile(c *cron.Cron, filePath string) {
// 	file, err := os.Open(filePath)
// 	if err != nil {
// 		fmt.Printf("Error opening file: %s\n", err)
// 		return
// 	}
// 	defer file.Close()

// 	scanner := bufio.NewScanner(file)
// 	for scanner.Scan() {
// 		line := scanner.Text()
// 		parts := strings.Fields(line)
// 		if len(parts) < 6 {
// 			fmt.Printf("Skipping invalid line: %s\n", line)
// 			continue
// 		}

// 		cronExpr := strings.Join(parts[:5], " ")
// 		command := strings.Join(parts[5:], " ")

// 		_, err := c.AddFunc(cronExpr, func(cmd string) func() {
// 			return func() {
// 				job(cmd)
// 			}
// 		}(command))
// 		var SchedulerLine string
// 		if err != nil {
// 			SchedulerLine += fmt.Sprintf("Error scheduling job: %s\n", err)
// 		} else {
// 			SchedulerLine += fmt.Sprintf("Scheduled job: %s with cron expression: %s\n", command, cronExpr)
// 		}
// 		fmt.Print(SchedulerLine)

// 		_, err = logFile.WriteString(SchedulerLine)
// 		if err != nil {
// 			fmt.Printf("Error writing to log file: %s\n", err)
// 		}
// 	}

// 	if err := scanner.Err(); err != nil {
// 		fmt.Printf("Error reading file: %s\n", err)
// 	}
// }

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
        SELECT command, task_uid, MAX(timestamp) AS last_run, 
               SUM(CASE WHEN status = 'Success' THEN 1 ELSE 0 END) AS success_count,
               SUM(CASE WHEN status = 'Failure' THEN 1 ELSE 0 END) AS failure_count,
               output
        FROM tasks
        GROUP BY command
        ORDER BY last_run DESC
    `)
	if err != nil {
		http.Error(w, "Error querying database", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	currentTime := getCurrentTime()

	var jobs []struct {
		TaskUID      string
		Command      string
		LastRun      string
		SuccessCount int
		FailureCount int
		Output       string
		HasOutput    bool
	}

	for rows.Next() {
		var taskID, command, lastRun, output string
		var successCount, failureCount int

		err := rows.Scan(&command, &taskID, &lastRun, &successCount, &failureCount, &output)
		if err != nil {
			http.Error(w, "Error reading from database", http.StatusInternalServerError)
			return
		}

		jobs = append(jobs, struct {
			TaskUID      string
			Command      string
			LastRun      string
			SuccessCount int
			FailureCount int
			Output       string
			HasOutput    bool
		}{
			TaskUID:      taskID,
			Command:      command,
			LastRun:      lastRun,
			SuccessCount: successCount,
			FailureCount: failureCount,
			Output:       output,
			HasOutput:    len(output) > 2,
		})
	}

	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	data := struct {
		CurrentTime     string
		RefreshInterval string
		Jobs            []struct {
			TaskUID      string
			Command      string
			LastRun      string
			SuccessCount int
			FailureCount int
			Output       string
			HasOutput    bool
		}
	}{
		CurrentTime:     currentTime,
		RefreshInterval: refreshInterval,
		Jobs:            jobs,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
	}
}

// Handler for downloading log file
func downloadLogHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task_uid")

	if taskID == "" {
		http.Error(w, "Task ID not specified", http.StatusBadRequest)
		return
	}

	// Retrieve job details from the database based on taskID
	query := `SELECT task_uid, command, timestamp, status, output FROM tasks WHERE task_uid = ?`
	row := db.QueryRow(query, taskID)

	var command, timestamp, status, output string
	err := row.Scan(&taskID, &command, &timestamp, &status, &output)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "No log entries found for the specified task ID", http.StatusNotFound)
		} else {
			http.Error(w, "Error querying database", http.StatusInternalServerError)
		}
		return
	}

	// Format the log content
	logContent := fmt.Sprintf("Task ID: %s\nCommand: %s\nTimestamp: %s\nStatus: %s\n\nOutput:\n%s\n",
		taskID, command, timestamp, status, output)

	// Set headers for file download
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.log", taskID))
	w.Header().Set("Content-Type", "application/octet-stream")
	_, err = w.Write([]byte(logContent))
	if err != nil {
		http.Error(w, "Error writing response", http.StatusInternalServerError)
		return
	}
}

// Handler for displaying the form to add new jobs
func addJobHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, `
	<!DOCTYPE html>
	<html lang="en">
	<head>
	    <meta charset="UTF-8">
	    <meta name="viewport" content="width=device-width, initial-scale=1.0">
	    <title>Add New Job</title>
	    <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/css/bootstrap.min.css" rel="stylesheet">
	</head>
	<body>
	    <div class="container mt-5">
	        <h1>Add New Cron Job</h1>
	        <form action="/submit-job" method="post">
	            <div class="mb-3">
	                <label for="jobName" class="form-label">Job Name</label>
	                <input type="text" class="form-control" id="jobName" name="job_name" required>
	            </div>
	            <div class="mb-3">
	                <label for="jobDescription" class="form-label">Job Description</label>
	                <input type="text" class="form-control" id="jobDescription" name="job_description" required>
	            </div>
	            <div class="mb-3">
	                <label for="cronExpr" class="form-label">Cron Expression</label>
	                <input type="text" class="form-control" id="cronExpr" name="cron_expr" required>
	            </div>
	            <div class="mb-3">
	                <label for="command" class="form-label">Command</label>
	                <input type="text" class="form-control" id="command" name="command" required>
	            </div>
	            <button type="submit" class="btn btn-primary">Add Job</button>
	        </form>
	    </div>
	</body>
	</html>
	`)
}

// Handler for processing the form submission
func submitJobHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	cronExpr := r.FormValue("cron_expr")
	command := r.FormValue("command")
	jobName := r.FormValue("job_name")
	jobDescription := r.FormValue("job_description")

	if cronExpr == "" || command == "" || jobName == "" || jobDescription == "" {
		renderFormWithError(w, cronExpr, command, jobName, jobDescription, "Missing cron expression, command, job name, or job description")
		return
	}

	// Check if the job name is already taken
	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM jobs WHERE job_name = ?)", jobName).Scan(&exists)
	if err != nil {
		renderFormWithError(w, cronExpr, command, jobName, jobDescription, "Error checking job name in database")
		return
	}

	if exists {
		renderFormWithError(w, cronExpr, command, jobName, jobDescription, "Job name already exists. Please enter a different unique job name.")
		return
	}

	// Insert the new job into the jobs table
	timestamp := time.Now().Format("02-01-2006 15:04:05")
	_, err = db.Exec(`INSERT INTO jobs (job_name, job_schedule, job_command, job_description, added_at) VALUES (?, ?, ?, ?, ?)`,
		jobName, cronExpr, command, jobDescription, timestamp)
	if err != nil {
		renderFormWithError(w, cronExpr, command, jobName, jobDescription, "Error inserting job into database")
		return
	}

	// Schedule the new job immediately
	c := cron.New()
	_, err = c.AddFunc(cronExpr, func(cmd string) func() {
		return func() {
			job(cmd)
		}
	}(command))
	if err != nil {
		fmt.Printf("Error scheduling new job: %s\n", err)
	} else {
		c.Start()
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Function to schedule jobs from the jobs table
func scheduleJobsFromTable(c *cron.Cron) {
	rows, err := db.Query(`SELECT job_schedule, job_command FROM jobs WHERE job_status = 1`)
	if err != nil {
		fmt.Printf("Error querying jobs from database: %s\n", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var cronExpr, command string
		err := rows.Scan(&cronExpr, &command)
		if err != nil {
			fmt.Printf("Error scanning job row: %s\n", err)
			continue
		}

		// Schedule the job only if job_status is 1 (active)
		_, err = c.AddFunc(cronExpr, func(cmd string) func() {
			return func() {
				job(cmd)
			}
		}(command))
		if err != nil {
			fmt.Printf("Error scheduling job: %s\n", err)
		} else {
			fmt.Printf("Scheduled job: %s with cron expression: %s\n", command, cronExpr)
		}
	}
}


// Handler to display the delete job page with populated dropdown
func deleteJobPageHandler(w http.ResponseWriter, r *http.Request) {
	// Fetch jobs from the database
	jobs, err := getJobs()
	if err != nil {
		http.Error(w, "Error retrieving jobs from the database", http.StatusInternalServerError)
		return
	}

	// Parse and execute the template with job data
	tmpl := template.Must(template.ParseFiles("templates/delete_job.html"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, struct{ Jobs []Job }{Jobs: jobs}); err != nil {
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
	}
}

type Job struct {
	JobID   string
	JobName string
}

func getJobs() ([]Job, error) {
	rows, err := db.Query("SELECT job_id, job_name FROM jobs where job_status = 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var job Job
		if err := rows.Scan(&job.JobID, &job.JobName); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return jobs, nil
}

// Handler for deleting a job
func deleteJobHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Parse form values
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	// Get job ID from form data
	jobID := r.FormValue("job_id")
	if jobID == "" {
		http.Error(w, "Job ID is required", http.StatusBadRequest)
		return
	}

	// Update job status to 0 (disabled) in the database
	_, err = db.Exec("UPDATE jobs SET job_status = 0 WHERE job_id = ?", jobID)
	if err != nil {
		http.Error(w, "Failed to delete job", http.StatusInternalServerError)
		return
	}

	// Redirect to a page that lists jobs or another appropriate page
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Helper function to render the form with an error message
func renderFormWithError(w http.ResponseWriter, cronExpr, command, jobName, jobDescription, errorMessage string) {
	// Render the form template with the error message and pre-filled form values
	tmpl := template.Must(template.ParseFiles("templates/form.html"))
	data := struct {
		CronExpr       string
		Command        string
		JobName        string
		JobDescription string
		ErrorMessage   string
	}{
		CronExpr:       cronExpr,
		Command:        command,
		JobName:        jobName,
		JobDescription: jobDescription,
		ErrorMessage:   errorMessage,
	}

	w.WriteHeader(http.StatusBadRequest) // Set the status to 400 Bad Request
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Error rendering form template", http.StatusInternalServerError)
	}
}

func main() {

	// Load environment variables from .env file
	loadErr := godotenv.Load()
	if loadErr != nil {
		fmt.Printf("Error loading .env file: %s\n", loadErr)
		return
	}

	// Get the log directory path from environment variables
	logDir := os.Getenv("LOG_DIR")
	if logDir == "" {
		fmt.Println("LOG_DIR environment variable is not set")
		return
	}

	dbDir := os.Getenv("DB_DIR")
	if dbDir == "" {
		fmt.Println("DB_DIR environment variable is not set")
		return
	}

	endPoint := os.Getenv("ENDPOINT")
	if endPoint == "" {
		fmt.Println("ENDPOINT environment variable is not set")
		return
	}

	// Initialize folders
	directories := []string{dbDir, logDir}
	for _, dir := range directories {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("Error creating %s directory: %s\n", dir, err)
			return
		}
	}

	var err error
	logFilePath := fmt.Sprintf("%s/scheduler.log", logDir)
	logFile, err = initLogFile(logFilePath)
	if err != nil {
		fmt.Printf("Error initializing log file: %s\n", err)
		return
	}
	defer logFile.Close()

	db, err = initDatabase(fmt.Sprintf("%s/jobs.db", dbDir))
	if err != nil {
		fmt.Printf("Error initializing database: %s\n", err)
		return
	}
	defer db.Close()

	c := cron.New()
	scheduleJobsFromTable(c)
	c.Start()
	logSchedulerStart()

	// Register routes
	http.HandleFunc("/", distinctCommandsHandler)
	http.HandleFunc("/download", downloadLogHandler)
	http.HandleFunc("/add-job", addJobHandler)
	http.HandleFunc("/submit-job", submitJobHandler)
	http.HandleFunc("/delete-job", deleteJobHandler)
	http.HandleFunc("/delete-job-page", deleteJobPageHandler)
	err = http.ListenAndServe(endPoint, nil)
	if err != nil {
		fmt.Printf("Error starting server: %s\n", err)
		return
	}
}
