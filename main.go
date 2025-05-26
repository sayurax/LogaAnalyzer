package main

import (
	"Log_Anallyzer/handlers"
	"fmt"
	"net/http"
	"os"
)

func main() {
	if _, err := os.Stat("uploads"); os.IsNotExist(err) {
		os.Mkdir("uploads", os.ModePerm)
	}

	http.HandleFunc("/upload", handlers.UploadHandler)
	http.HandleFunc("/request-details", handlers.RequestDetailsHandler)
	http.HandleFunc("/queryExecutions", handlers.QueryExecutionsHandler)
	http.HandleFunc("/correlationDetails", handlers.CorrelationDetailsHandler)
	http.HandleFunc("/queryExecutionsForRequestPath", handlers.QueryExecutionsForRequestHandler)
	http.HandleFunc("/queryDetails", handlers.QueryDetailsHandler)

	fmt.Println("Server started on :8080...")
	http.ListenAndServe(":8080", nil)
}
