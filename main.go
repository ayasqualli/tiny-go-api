package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

func sendJSON(w http.ResponseWriter, data map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		sendJSON(w, map[string]string{
			"message": "Tiny Go backend is running",
		})
	})

	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		sendJSON(w, map[string]string{
			"message": "Hello from Go net/http!",
		})
	})

	log.Println("Server running at http://localhost:" + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}