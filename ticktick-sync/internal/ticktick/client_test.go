package ticktick

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListTasks(t *testing.T) {
	tasks := []Task{
		{ID: "t1", ProjectID: "p1", Title: "Agent task", Tags: []string{"agent"}, Status: 0},
		{ID: "t2", ProjectID: "p1", Title: "Normal task", Tags: []string{"work"}, Status: 0},
	}
	data := ProjectData{
		Project: Project{ID: "p1", Name: "Test"},
		Tasks:   tasks,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open/v1/project/p1/data" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing or wrong auth header")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(data)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	got, err := c.ListTasks(context.Background(), "p1")
	if err != nil {
		t.Fatalf("ListTasks error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d tasks, want 2", len(got))
	}
	if got[0].ID != "t1" {
		t.Errorf("first task ID = %q, want %q", got[0].ID, "t1")
	}
}

func TestListAgentTasks(t *testing.T) {
	tasks := []Task{
		{ID: "t1", ProjectID: "p1", Title: "Agent task", Tags: []string{"agent"}, Status: 0},
		{ID: "t2", ProjectID: "p1", Title: "Normal task", Tags: []string{"work"}, Status: 0},
		{ID: "t3", ProjectID: "p1", Title: "Done agent", Tags: []string{"agent"}, Status: 2},
	}
	data := ProjectData{
		Project: Project{ID: "p1", Name: "Test"},
		Tasks:   tasks,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(data)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	got, err := c.ListAgentTasks(context.Background(), "p1")
	if err != nil {
		t.Fatalf("ListAgentTasks error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d agent tasks, want 2", len(got))
	}
}

func TestCreateTask(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/open/v1/task" {
			t.Errorf("path = %s, want /open/v1/task", r.URL.Path)
		}
		var req CreateTaskRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Title != "Test task" {
			t.Errorf("title = %q, want %q", req.Title, "Test task")
		}
		resp := Task{ID: "new-id", ProjectID: req.ProjectID, Title: req.Title, Tags: req.Tags}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	task, err := c.CreateTask(context.Background(), CreateTaskRequest{
		Title:     "Test task",
		Content:   "Description",
		ProjectID: "p1",
		Tags:      []string{"agent"},
	})
	if err != nil {
		t.Fatalf("CreateTask error: %v", err)
	}
	if task.ID != "new-id" {
		t.Errorf("ID = %q, want %q", task.ID, "new-id")
	}
}

func TestCompleteTask(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/open/v1/project/p1/task/t1/complete" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	err := c.CompleteTask(context.Background(), "p1", "t1")
	if err != nil {
		t.Fatalf("CompleteTask error: %v", err)
	}
}
