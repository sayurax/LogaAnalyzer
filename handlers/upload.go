package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)



// FileDetail holds the structured content extracted from an uploaded file
type FileDetail struct {
	Timestamp               string `json:"timestamp"`
	CorrelationId           string `json:"correlationId"`
	ThreadId                string `json:"threadId"`
	TotalDurationForRequest string `json:"totalDurationForRequest"`
	CallType                string `json:"callType"`
	StartTime               string `json:"startTime"`
	MethodName              string `json:"methodName"`
	RequestQuery            string `json:"requestQuery"`
	RequestPath             string `json:"requestPath"`
}

var uploadedFiles []FileDetail                                   // uploadedFiles stores the content of all uploaded files
var requestPathStats = map[string]map[string]*RequestPathStats{} // The first key is tabUUID, and the second is RequestPath
var httpResponses []FileDetail

// TemplateData holds data passed to the HTML template
type TemplateData struct {
	RequestPathStats map[string]*RequestPathStats //A map (from a string key to a *RequestPathStats) containing statistics about request paths.
	QueryMetrics     map[string]*QueryMetrics     //A map (from a string key to a *QueryMetrics) that holds metrics related to request queries
	FileDetails      []FileDetail                 //A slice of FileDetail representing the processed file uploads.
	FileName         string
	FileNames        []string //A slice of strings that could list all file names available or processed.
	HttpResponses    []FileDetail
}

// Holds the following fields to track performance metrics:
type RequestPathStats struct {
	Count       int
	TotalTime   float64
	AverageTime float64
	MaxTime     float64
	MinTime     float64
	Durations   []float64
	Percentile  float64
}

// QueryMetrics holds statistics for request queries
type QueryMetrics struct {
	Count       int
	TotalTime   float64
	AverageTime float64
	MaxTime     float64
	MinTime     float64
	Durations   []float64
	Percentile  float64
}

// Stores statistics for request queries per tab
var queryMetricsMap = map[string]map[string]*QueryMetrics{}

// UploadHandler handles both the GET and POST requests
func UploadHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Received request:", r.Method)

	if r.Method == http.MethodPost {
		log.Println("Processing POST request")

		// Set a limit of 10MB for file uploads
		r.ParseMultipartForm(10 << 20) // 10MB file size limit
		log.Println("Multipart form parsed successfully")

		// Retrieve all the uploaded files
		files := r.MultipartForm.File["uploadedFile"]
		if len(files) == 0 {
			log.Println("No files uploaded")
			http.Error(w, "No files uploaded", http.StatusBadRequest)
			return
		}
		log.Printf("%d file(s) uploaded\n", len(files))

		// Collect file names
		var fileNames []string
		for _, fileHeader := range files {
			fileNames = append(fileNames, fileHeader.Filename)
		}
		log.Printf("File names collected: %v\n", fileNames)

		// Process each file content
		for _, fileHeader := range files {
			log.Printf("Processing file: %s\n", fileHeader.Filename)
			file, err := fileHeader.Open() // Open each file
			if err != nil {
				log.Printf("Error opening file %s: %v\n", fileHeader.Filename, err)
				http.Error(w, "Error opening the file", http.StatusInternalServerError)
				return
			}
			defer file.Close()

			// Process the file content
			if err := processFile(file); err != nil {
				log.Printf("Error processing file %s: %v\n", fileHeader.Filename, err)
				http.Error(w, "Error processing the file", http.StatusInternalServerError)
				return
			}
			log.Printf("File %s processed successfully\n", fileHeader.Filename)
		}

		// Retrieve the tabUUID from the form
		tabUUID := r.FormValue("uniqueID")
		log.Printf("Tab UUID: %s\n", tabUUID)

		// Ensure the tabUUID exists in requestPathStats, if not, initialize it
		if _, exists := requestPathStats[tabUUID]; !exists {
			requestPathStats[tabUUID] = map[string]*RequestPathStats{}
			log.Printf("Initialized requestPathStats for tabUUID: %s\n", tabUUID)
		}

		// Ensure the tabUUID exists in queryMetricsMap, if not, initialize it
		if _, exists := queryMetricsMap[tabUUID]; !exists {
			queryMetricsMap[tabUUID] = map[string]*QueryMetrics{}
			log.Printf("Initialized queryMetricsMap for tabUUID: %s\n", tabUUID)
		}

		// Calculate request path stats for the specific tab
		log.Println("Calculating request path stats...")
		calculateRequestPathStats(tabUUID, false)

		// Extract and calculate request query metrics
		log.Println("Extracting and calculating query metrics...")
		extractRequestQueries(tabUUID)
		calculateQueryMetrics(tabUUID)

		httpResponses := extractHTTPResponses(uploadedFiles)

		// Save processed data as JSON
		log.Println("Saving processed data as JSON...")
		saveFilesAsJSON(uploadedFiles, tabUUID)

		// Serve the HTML template with request path stats, query metrics, and file details
		tmpl := template.Must(template.ParseFiles("template/index.html"))
		err := tmpl.Execute(w, TemplateData{
			RequestPathStats: requestPathStats[tabUUID], // Show request path stats for this tab
			QueryMetrics:     queryMetricsMap[tabUUID],  // Show request query stats for this tab
			FileDetails:      uploadedFiles,
			FileNames:        fileNames,
			HttpResponses:    httpResponses,
		})
		if err != nil {
			log.Printf("Error rendering template: %v\n", err)
			http.Error(w, "Error rendering template", http.StatusInternalServerError)
			return
		}
		log.Println("HTML template rendered successfully")

		// Reset uploadedFiles for the next upload
		uploadedFiles = nil
		log.Println("Reset uploaded files for next upload")

	} else if r.Method == http.MethodGet {
		log.Println("Processing GET request")

		// Retrieve the tabUUID from the URL (e.g., in the query params)
		tabUUID := r.URL.Query().Get("uniqueID")
		log.Printf("Tab UUID from URL: %s\n", tabUUID)

		// Ensure the tabUUID exists in requestPathStats, if not, initialize it
		if _, exists := requestPathStats[tabUUID]; !exists {
			requestPathStats[tabUUID] = map[string]*RequestPathStats{}
			log.Printf("Initialized requestPathStats for tabUUID: %s\n", tabUUID)
		}

		// Ensure the tabUUID exists in queryMetricsMap, if not, initialize it
		if _, exists := queryMetricsMap[tabUUID]; !exists {
			queryMetricsMap[tabUUID] = map[string]*QueryMetrics{}
			log.Printf("Initialized queryMetricsMap for tabUUID: %s\n", tabUUID)
		}

		// Serve the upload page for GET requests, showing only the stats for the current tab
		tmpl := template.Must(template.ParseFiles("template/index.html"))
		err := tmpl.Execute(w, TemplateData{
			RequestPathStats: requestPathStats[tabUUID], // Only show request path stats for this tab
			QueryMetrics:     queryMetricsMap[tabUUID],  // Only show query metrics for this tab
			HttpResponses:    httpResponses,
		})
		if err != nil {
			log.Printf("Error rendering template: %v\n", err)
			http.Error(w, "Error rendering template", http.StatusInternalServerError)
			return
		}
		log.Println("HTML template rendered successfully for GET request")

	} else {
		log.Println("Invalid request method:", r.Method)
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
	}
}

func extractHTTPResponses(uploadedFiles []FileDetail) []FileDetail {
	var httpResponses []FileDetail

	for _, fileDetail := range uploadedFiles {
		if strings.EqualFold(fileDetail.CallType, "HTTP-IN-Response") {
			httpResponses = append(httpResponses, fileDetail)
		}
	}

	return httpResponses
}

// processFile processes the uploaded file and extracts the details
func processFile(file io.Reader) error {
	content, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	extractFileDetails(string(content))
	return nil
}

// extractFileDetails parses the content and extracts structured fields for each line
func extractFileDetails(content string) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		fields := strings.Split(line, "|")
		if len(fields) >= 9 {
			requestPath := strings.TrimSpace(fields[8])

			// Check if RequestPath ends with any excluded extensions
			excludedExtensions := []string{".js", ".js(1)", ".ttf", ".css", ".gif", ".ico", ".png", ".jsf", ".woff", ".woff2", ".jpg", ".map"}
			exclude := false
			for _, ext := range excludedExtensions {
				if strings.HasSuffix(requestPath, ext) {
					exclude = true
					break
				}
			}

			// Only append if the RequestPath does not match excluded extensions
			if !exclude {
				fileDetail := FileDetail{
					Timestamp:               strings.TrimSpace(fields[0]),
					CorrelationId:           strings.TrimSpace(fields[1]),
					ThreadId:                strings.TrimSpace(fields[2]),
					TotalDurationForRequest: strings.TrimSpace(fields[3]),
					CallType:                strings.TrimSpace(fields[4]),
					StartTime:               strings.TrimSpace(fields[5]),
					MethodName:              strings.TrimSpace(fields[6]),
					RequestQuery:            strings.TrimSpace(fields[7]),
					RequestPath:             requestPath,
				}
				uploadedFiles = append(uploadedFiles, fileDetail)
			}
		}
	}
}

func calculateRequestPathStats(tabUUID string, resetStats bool) {
	// Reset stats only if resetStats is true
	if resetStats {
		requestPathStats[tabUUID] = map[string]*RequestPathStats{}
	}

	for _, fileDetail := range uploadedFiles {
		if strings.EqualFold(fileDetail.CallType, "HTTP-IN-Response") {
			duration, err := strconv.ParseFloat(fileDetail.TotalDurationForRequest, 64)
			if err != nil {
				continue // Skip invalid durations
			}

			// Initialize stats for a new request path if it doesn't exist
			if _, exists := requestPathStats[tabUUID][fileDetail.RequestPath]; !exists {
				requestPathStats[tabUUID][fileDetail.RequestPath] = &RequestPathStats{
					MaxTime:   duration, // Set MaxTime as the first value
					MinTime:   duration, // Set MinTime as the first value (even if 0)
					Durations: []float64{},
				}
			}

			stats := requestPathStats[tabUUID][fileDetail.RequestPath]
			stats.Count++
			stats.TotalTime += duration
			stats.Durations = append(stats.Durations, duration)
			stats.AverageTime = stats.TotalTime / float64(stats.Count)

			// Update MaxTime if the current duration is greater
			if duration > stats.MaxTime {
				stats.MaxTime = duration
			}

			// Update MinTime: Allow 0 as a valid minimum value
			if stats.Count > 1 {
				if duration < stats.MinTime {
					stats.MinTime = duration
				}
			} else {
				stats.MinTime = stats.MaxTime // If only 1 occurrence, set MinTime = MaxTime
			}
		}
	}

	for _, stats := range requestPathStats[tabUUID] {
		if len(stats.Durations) > 0 {
			sort.Float64s(stats.Durations)
			index := int(0.95 * float64(len(stats.Durations)-1))
			stats.Percentile = stats.Durations[index]

		}

	}
}

func saveFilesAsJSON(uploadedFiles []FileDetail, tabUUID string) {
	// Use the tabUUID to generate a unique file name
	fileName := fmt.Sprintf("uploads/%s.json", tabUUID)

	// Check if the file exists
	var existingFiles []FileDetail // Replace 'YourFileType' with the actual struct type of uploadedFiles

	if _, err := os.Stat(fileName); err == nil {
		// File exists, read existing content
		existingFile, err := os.Open(fileName)
		if err != nil {
			fmt.Println("Error opening existing JSON file:", err)
			return
		}
		defer existingFile.Close()

		// Decode existing JSON data
		decoder := json.NewDecoder(existingFile)
		err = decoder.Decode(&existingFiles)
		if err != nil {
			fmt.Println("Error decoding existing JSON data:", err)
			return
		}
	} else if !os.IsNotExist(err) {
		fmt.Println("Error checking if file exists:", err)
		return
	}

	// Append new uploadedFiles to existing data
	combinedFiles := append(existingFiles, uploadedFiles...)

	// Create or overwrite the file with updated data
	file, err := os.Create(fileName)
	if err != nil {
		fmt.Println("Error creating JSON file:", err)
		return
	}
	defer file.Close()

	// Write the combined data to the file
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(combinedFiles)
	if err != nil {
		fmt.Println("Error encoding JSON data:", err)
		return
	}

	fmt.Printf("File successfully saved to: %s\n", fileName)
}

func extractRequestQueries(tabUUID string) {
	// Ensure the tabUUID exists in queryMetricsMap, if not, initialize it
	if _, exists := queryMetricsMap[tabUUID]; !exists {
		queryMetricsMap[tabUUID] = map[string]*QueryMetrics{}
	}

	for _, fileDetail := range uploadedFiles {
		// Skip if CallType is "HTTP-IN-Request" or "HTTP-IN-Response"
		if strings.EqualFold(fileDetail.CallType, "HTTP-IN-Request") || strings.EqualFold(fileDetail.CallType, "HTTP-IN-Response") {
			continue
		}

		query := strings.TrimSpace(fileDetail.RequestQuery)
		if query == "" {
			continue // Skip empty queries
		}

		// Initialize metrics for a new request query if it doesn't exist
		if _, exists := queryMetricsMap[tabUUID][query]; !exists {
			queryMetricsMap[tabUUID][query] = &QueryMetrics{
				MaxTime: 0,   // Initialize MaxTime to 0
				MinTime: 1e9, // Start MinTime with a very large number (1 billion)
			}
		}
	}
}

func calculateQueryMetrics(tabUUID string) {
	for _, fileDetail := range uploadedFiles {
		// Skip if CallType is "HTTP-IN-Request" or "HTTP-IN-Response"
		if strings.EqualFold(fileDetail.CallType, "HTTP-IN-Request") || strings.EqualFold(fileDetail.CallType, "HTTP-IN-Response") {
			continue
		}

		query := strings.TrimSpace(fileDetail.RequestQuery)
		if query == "" {
			continue // Skip empty queries
		}

		duration, err := strconv.ParseFloat(fileDetail.TotalDurationForRequest, 64)
		if err != nil {
			continue // Skip invalid durations
		}

		// Ensure the query exists in the map
		if _, exists := queryMetricsMap[tabUUID][query]; !exists {
			if queryMetricsMap[tabUUID] == nil {
				queryMetricsMap[tabUUID] = make(map[string]*QueryMetrics)
			}
			queryMetricsMap[tabUUID][query] = &QueryMetrics{
				MaxTime:   0,
				MinTime:   1e9, // Set a high initial MinTime
				Durations: []float64{},
			}
		}

		metrics := queryMetricsMap[tabUUID][query]
		metrics.Count++
		metrics.TotalTime += duration
		metrics.AverageTime = metrics.TotalTime / float64(metrics.Count)
		metrics.Durations = append(metrics.Durations, duration)

		// Update MaxTime if the current duration is greater
		if duration > metrics.MaxTime {
			metrics.MaxTime = duration
		}

		// Update MinTime
		if duration < metrics.MinTime {
			metrics.MinTime = duration
		}
	}

	//Calculate the 95th percentile for each query
	for _, queryMetrics := range queryMetricsMap[tabUUID] {
		sort.Float64s(queryMetrics.Durations)
		n := len(queryMetrics.Durations)
		if n > 0 {
			index := int(float64(n)*0.95+0.5) - 1
			if index >= n {
				index = n - 1
			}
			queryMetrics.Percentile = queryMetrics.Durations[index]
		}
	}
}
