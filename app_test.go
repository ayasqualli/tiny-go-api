package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func testHandler() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewApp(NewTaskStore(seedTasks), logger).Routes()
}

func performRequest(t *testing.T, handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func decodeResponse[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.NewDecoder(recorder.Body).Decode(&value); err != nil {
		t.Fatalf("decode response: %v; body=%q", err, recorder.Body.String())
	}
	return value
}

func TestRootAndHealth(t *testing.T) {
	handler := testHandler()

	root := performRequest(t, handler, http.MethodGet, "/", "")
	if root.Code != http.StatusOK {
		t.Fatalf("GET /: got %d, want 200", root.Code)
	}
	metadata := decodeResponse[map[string]any](t, root)
	if metadata["name"] != apiName {
		t.Fatalf("GET / name: got %v, want %q", metadata["name"], apiName)
	}

	health := performRequest(t, handler, http.MethodGet, "/health", "")
	if health.Code != http.StatusOK || !strings.Contains(health.Body.String(), `"status":"ok"`) {
		t.Fatalf("GET /health: code=%d body=%q", health.Code, health.Body.String())
	}
}

func TestReadEndpoints(t *testing.T) {
	handler := testHandler()

	list := performRequest(t, handler, http.MethodGet, "/tasks", "")
	if list.Code != http.StatusOK {
		t.Fatalf("GET /tasks: got %d, want 200", list.Code)
	}
	tasks := decodeResponse[[]Task](t, list)
	if len(tasks) != 3 {
		t.Fatalf("GET /tasks: got %d tasks, want 3", len(tasks))
	}
	if list.Header().Get("X-Total-Count") != "3" {
		t.Fatalf("X-Total-Count: got %q, want 3", list.Header().Get("X-Total-Count"))
	}

	one := performRequest(t, handler, http.MethodGet, "/tasks/1", "")
	if one.Code != http.StatusOK {
		t.Fatalf("GET /tasks/1: got %d, want 200", one.Code)
	}
	if task := decodeResponse[Task](t, one); task.ID != 1 {
		t.Fatalf("GET /tasks/1 returned id %d", task.ID)
	}

	missing := performRequest(t, handler, http.MethodGet, "/tasks/99", "")
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), "Task 99 not found") {
		t.Fatalf("GET /tasks/99: code=%d body=%q", missing.Code, missing.Body.String())
	}
}

func TestCreateValidationAndLocation(t *testing.T) {
	handler := testHandler()

	created := performRequest(t, handler, http.MethodPost, "/tasks", `{"title":"  Buy milk  "}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("POST /tasks: got %d body=%q", created.Code, created.Body.String())
	}
	task := decodeResponse[Task](t, created)
	if task.ID != 4 || task.Title != "Buy milk" || task.Done {
		t.Fatalf("created task: %+v", task)
	}
	if got := created.Header().Get("Location"); got != "/tasks/4" {
		t.Fatalf("Location: got %q, want /tasks/4", got)
	}

	for name, body := range map[string]string{
		"missing title": `{}`,
		"empty title":   `{"title":"  "}`,
		"unknown field": `{"title":"ok","priority":1}`,
		"invalid json":  `{"title":`,
	} {
		t.Run(name, func(t *testing.T) {
			response := performRequest(t, handler, http.MethodPost, "/tasks", body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("got %d body=%q, want 400", response.Code, response.Body.String())
			}
		})
	}
}

func TestFullCRUDCycle(t *testing.T) {
	handler := testHandler()

	created := performRequest(t, handler, http.MethodPost, "/tasks", `{"title":"Write tests"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: code=%d body=%q", created.Code, created.Body.String())
	}
	task := decodeResponse[Task](t, created)

	updated := performRequest(t, handler, http.MethodPut, "/tasks/"+strconv.Itoa(task.ID), `{"title":"Write integration tests","done":true}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("update: code=%d body=%q", updated.Code, updated.Body.String())
	}
	updatedTask := decodeResponse[Task](t, updated)
	if updatedTask.Title != "Write integration tests" || !updatedTask.Done {
		t.Fatalf("updated task: %+v", updatedTask)
	}

	deleted := performRequest(t, handler, http.MethodDelete, "/tasks/"+strconv.Itoa(task.ID), "")
	if deleted.Code != http.StatusNoContent || deleted.Body.Len() != 0 {
		t.Fatalf("delete: code=%d body=%q", deleted.Code, deleted.Body.String())
	}

	missing := performRequest(t, handler, http.MethodGet, "/tasks/"+strconv.Itoa(task.ID), "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("get after delete: got %d, want 404", missing.Code)
	}
}

func TestUpdateValidationAndMissingTask(t *testing.T) {
	handler := testHandler()

	empty := performRequest(t, handler, http.MethodPut, "/tasks/1", `{}`)
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("empty update: got %d, want 400", empty.Code)
	}

	blankTitle := performRequest(t, handler, http.MethodPut, "/tasks/1", `{"title":" "}`)
	if blankTitle.Code != http.StatusBadRequest {
		t.Fatalf("blank title: got %d, want 400", blankTitle.Code)
	}

	missing := performRequest(t, handler, http.MethodPut, "/tasks/99", `{"done":true}`)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing update: got %d, want 404", missing.Code)
	}
}

func TestFilteringPaginationStatsAndReset(t *testing.T) {
	handler := testHandler()

	filtered := performRequest(t, handler, http.MethodGet, "/tasks?done=false&search=api&limit=1&offset=0", "")
	if filtered.Code != http.StatusOK {
		t.Fatalf("filtered list: code=%d body=%q", filtered.Code, filtered.Body.String())
	}
	tasks := decodeResponse[[]Task](t, filtered)
	if len(tasks) != 1 || !strings.Contains(strings.ToLower(tasks[0].Title), "api") {
		t.Fatalf("filtered tasks: %+v", tasks)
	}

	badQuery := performRequest(t, handler, http.MethodGet, "/tasks?done=maybe", "")
	if badQuery.Code != http.StatusBadRequest {
		t.Fatalf("bad query: got %d, want 400", badQuery.Code)
	}

	stats := performRequest(t, handler, http.MethodGet, "/stats", "")
	if stats.Code != http.StatusOK {
		t.Fatalf("stats: got %d", stats.Code)
	}
	counts := decodeResponse[TaskStats](t, stats)
	if counts != (TaskStats{Total: 3, Done: 1, Open: 2}) {
		t.Fatalf("stats: %+v", counts)
	}

	_ = performRequest(t, handler, http.MethodDelete, "/tasks/1", "")
	reset := performRequest(t, handler, http.MethodPost, "/reset", "")
	if reset.Code != http.StatusOK {
		t.Fatalf("reset: got %d", reset.Code)
	}
	resetTasks := decodeResponse[[]Task](t, reset)
	if len(resetTasks) != 3 || resetTasks[0].ID != 1 {
		t.Fatalf("reset tasks: %+v", resetTasks)
	}
}

func TestDocsAndOpenAPI(t *testing.T) {
	handler := testHandler()

	docs := performRequest(t, handler, http.MethodGet, "/docs", "")
	if docs.Code != http.StatusOK || !strings.Contains(docs.Body.String(), "SwaggerUIBundle") {
		t.Fatalf("docs: code=%d body=%q", docs.Code, docs.Body.String())
	}

	spec := performRequest(t, handler, http.MethodGet, "/openapi.json", "")
	if spec.Code != http.StatusOK {
		t.Fatalf("openapi: got %d", spec.Code)
	}
	var document map[string]any
	if err := json.NewDecoder(spec.Body).Decode(&document); err != nil {
		t.Fatalf("invalid OpenAPI JSON: %v", err)
	}
	if document["openapi"] != "3.0.3" {
		t.Fatalf("OpenAPI version: got %v", document["openapi"])
	}
}
