package main

import (
	"sort"
	"strings"
	"sync"
)

var seedTasks = []Task{
	{ID: 1, Title: "Learn HTTP basics", Done: true},
	{ID: 2, Title: "Build a CRUD API", Done: false},
	{ID: 3, Title: "Test the API in Swagger UI", Done: false},
}

type ListOptions struct {
	Done   *bool
	Search string
	Limit  int
	Offset int
}

// TaskStore is a concurrency-safe in-memory task repository
// Its data intentionally disappears when the process starts

type TaskStore struct {
	mu     sync.RWMutex
	tasks  []Task
	nextID int
}

func NewTaskStore(tasks []Task) *TaskStore {
	s := &TaskStore{}
	s.resetLocked(tasks)
	return s
}

func (s *TaskStore) List(opts ListOptions) ([]Task, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	search := strings.ToLower(strings.TrimSpace(opts.Search))
	matched := make([]Task, 0, len(s.tasks))

	for _, task := range s.tasks {
		if opts.Done != nil && task.Done != *opts.Done {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(task.Title), search) {
			continue
		}
		matched = append(matched, task)
	}

	total := len(matched)
	start := min(opts.Offset, total)
	end := total

	if opts.Limit > 0 {
		end = min(start+opts.Limit, total)
	}

	return append([]Task(nil), matched[start:end]...), total
}

func (s *TaskStore) Get(id int) (Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, task := range s.tasks {
		if task.ID == id {
			return task, true
		}
	}
	return Task{}, false
}

func (s *TaskStore) Create(title string) Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	task := Task{
		ID:    s.nextID,
		Title: title,
		Done:  false,
	}

	s.nextID++
	s.tasks = append(s.tasks, task)

	return task
}

func (s *TaskStore) Update(id int, input UpdateTaskInput) (Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.tasks {
		if s.tasks[i].ID != id {
			continue
		}

		if input.Title != nil {
			s.tasks[i].Title = *input.Title
		}

		if input.Done != nil {
			s.tasks[i].Done = *input.Done
		}

		return s.tasks[i], true
	}

	return Task{}, false
}

func (s *TaskStore) Delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, task := range s.tasks {
		if task.ID == id {
			s.tasks = append(s.tasks[:i], s.tasks[i+1:]...)
			return true
		}
	}
	return false
}

func (s *TaskStore) Stats() TaskStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := TaskStats{Total: len(s.tasks)}

	for _, task := range s.tasks {
		if task.Done {
			stats.Done++
		}
	}
	stats.Open = stats.Total - stats.Done

	return stats
}

func (s *TaskStore) Reset(tasks []Task) []Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.resetLocked(tasks)
	return append([]Task(nil), s.tasks...)
}

func (s *TaskStore) resetLocked(tasks []Task) {
	s.tasks = append([]Task(nil), tasks...)
	s.nextID = 1
	for _, task := range s.tasks {
		s.nextID = max(s.nextID, task.ID+1)
	}
	sort.Slice(s.tasks, func(i, j int) bool { return s.tasks[i].ID < s.tasks[j].ID })
}
