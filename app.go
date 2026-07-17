package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	apiName        = "Task API"
	apiVersion     = "1.0.0"
	maxRequestBody = 1 << 20
	maxTitleLength = 200
	maxPageSize    = 100
)

//go:embed api/openapi.json
var openAPISpec []byte

//go:embed web/docs.html
var docsHTML []byte

type App struct {
	store  *TaskStore
	logger *slog.Logger
}

func NewApp(store *TaskStore, logger *slog.Logger) *App {
	if store == nil {
		store = NewTaskStore(seedTasks)
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &App{store: store, logger: logger}
}

////////////////////////////////////////////////////////////////////////////////////////
// Routes Handlers

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleRoot)
	mux.HandleFunc("/health", a.handleHealth)
	mux.HandleFunc("/tasks", a.handleTasks)
	mux.HandleFunc("/tasks/", a.handleTask)
	mux.HandleFunc("/stats", a.handleStats)
	mux.HandleFunc("/reset", a.handleReset)
	mux.HandleFunc("/openapi.json", a.handleOpenAPI)
	mux.HandleFunc("/docs", a.handleDocs)

	return a.recoverPanics(a.logRequests(a.securityHeaders(mux)))
}

func (a *App) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.handleListTasks(w, r)
	case http.MethodPost:
		a.handleCreateTask(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (a *App) handleTask(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.handleGetTask(w, r)
	case http.MethodPut:
		a.handleUpdateTask(w, r)
	case http.MethodDelete:
		a.handleDeleteTask(w, r)
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (a *App) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    apiName,
		"version": apiVersion,
		"endpoints": []string{
			"/health", "/tasks", "/tasks/{id}", "/stats", "/reset", "/openapi.json", "/docs",
		},
	})
}

func (a *App) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleListTasks(w http.ResponseWriter, r *http.Request) {
	opts, err := parseListOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	tasks, total := a.store.List(opts)
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	w.Header().Set("X-Limit", strconv.Itoa(opts.Limit))
	w.Header().Set("X-Offset", strconv.Itoa(opts.Offset))
	writeJSON(w, http.StatusOK, tasks)
}

func (a *App) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id, err := parseTaskID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	task, ok := a.store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("Task %d not found", id))
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (a *App) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var input CreateTaskInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	title, err := validateTitle(input.Title)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	task := a.store.Create(title)
	w.Header().Set("Location", fmt.Sprintf("/tasks/%d", task.ID))
	writeJSON(w, http.StatusCreated, task)
}

func (a *App) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id, err := parseTaskID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var input UpdateTaskInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if input.Title == nil && input.Done == nil {
		writeError(w, http.StatusBadRequest, "Request body must include title and/or done")
		return
	}

	if input.Title != nil {
		title, err := validateTitle(*input.Title)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		input.Title = &title
	}

	task, ok := a.store.Update(id, input)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("Task %d not found", id))
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (a *App) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := parseTaskID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !a.store.Delete(id) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("Task %d not found", id))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.store.Stats())
}

func (a *App) handleReset(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.store.Reset(seedTasks))
}

func (a *App) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(openAPISpec)
}

func (a *App) handleDocs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(docsHTML)
}

/////////////////////////////////////////////////////////////////////////////
// Helper Functions

func parseID(raw string) (int, error) {
	id, err := strconv.Atoi(raw)
	if err != nil || id < 1 {
		return 0, errors.New("Task ID must be a positive integer")
	}
	return id, nil
}

func parseTaskID(r *http.Request) (int, error) {
	const prefix = "/tasks/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return 0, errors.New("Task ID must be a positive integer")
	}

	rawID := strings.TrimPrefix(r.URL.Path, prefix)
	if rawID == "" || strings.Contains(rawID, "/") {
		return 0, errors.New("Task ID must be a positive integer")
	}

	return parseID(rawID)
}

func parseListOptions(r *http.Request) (ListOptions, error) {
	query := r.URL.Query()
	opts := ListOptions{Search: query.Get("search"), Limit: 50}

	if rawDone := query.Get("done"); rawDone != "" {
		done, err := strconv.ParseBool(rawDone)
		if err != nil {
			return ListOptions{}, errors.New("done must be true or false")
		}
		opts.Done = &done
	}

	if rawLimit := query.Get("limit"); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit < 1 || limit > maxPageSize {
			return ListOptions{}, fmt.Errorf("limit must be between 1 and %d", maxPageSize)
		}
		opts.Limit = limit
	}

	if rawOffset := query.Get("offset"); rawOffset != "" {
		offset, err := strconv.Atoi(rawOffset)
		if err != nil || offset < 0 {
			return ListOptions{}, errors.New("offset must be zero or greater")
		}
		opts.Offset = offset
	}

	return opts, nil
}

func validateTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	if title == "" {
		return "", errors.New("title is required and cannot be empty")
	}
	if len([]rune(title)) > maxTitleLength {
		return "", fmt.Errorf("title cannot exceed %d characters", maxTitleLength)
	}
	return title, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	contentType := r.Header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil || mediaType != "application/json" {
			return errors.New("Content-Type must be application/json")
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		var maxBytesError *http.MaxBytesError
		switch {
		case errors.Is(err, io.EOF):
			return errors.New("Request body must not be empty")
		case errors.As(err, &maxBytesError):
			return fmt.Errorf("Request body must not exceed %d bytes", maxRequestBody)
		default:
			return fmt.Errorf("Invalid JSON body: %v", err)
		}
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("Request body must contain a single JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(data)
	r.bytes += n
	return n, err
}

func (a *App) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		a.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"bytes", recorder.bytes,
			"duration", time.Since(started),
		)
	})
}

func (a *App) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				a.logger.Error("panic recovered", "error", value, "method", r.Method, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *App) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
