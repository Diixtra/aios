package ticktick

import "time"

type Task struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"projectId"`
	Title        string    `json:"title"`
	Content      string    `json:"content"`
	Status       int       `json:"status"`
	Tags         []string  `json:"tags"`
	ModifiedTime time.Time `json:"modifiedTime"`
	CreatedDate  string    `json:"createdDate"`
}

func (t Task) IsCompleted() bool {
	return t.Status == 2
}

func (t Task) HasTag(tag string) bool {
	for _, tt := range t.Tags {
		if tt == tag {
			return true
		}
	}
	return false
}

type CreateTaskRequest struct {
	Title     string   `json:"title"`
	Content   string   `json:"content,omitempty"`
	ProjectID string   `json:"projectId"`
	Tags      []string `json:"tags,omitempty"`
}

type UpdateTaskRequest struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Status    int    `json:"status,omitempty"`
}

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ProjectData struct {
	Project Project `json:"project"`
	Tasks   []Task  `json:"tasks"`
}
