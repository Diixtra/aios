package document

import "time"

// Document represents a Paperless-ngx document with resolved names.
type Document struct {
	ID            int       `json:"id"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	Correspondent string    `json:"correspondent"`
	Tags          []string  `json:"tags"`
	Created       time.Time `json:"created"`
	Added         time.Time `json:"added"`
	OriginalURL   string    `json:"original_url"`
}
