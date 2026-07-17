package main

// Task is a single to-do item managed by the API
type Task struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

// CreateTaskInput is the JSON body accepted by POST /tasks
type CreateTaskInput struct {
	Title string `json:"title"`
}

// UpdateTaskInput uses pointers so the API can distinguish omitted fields from explicit values such as done:false
type UpdateTaskInput struct {
	Title *string `json:"title,omitempty"`
	Done  *bool   `json:"done,omitempty"`
}

// TaskStats is returned by GET /stats
type TaskStats struct {
	Total int `json:"total"`
	Done  int `json:"done"`
	Open  int `json:"open"`
}
