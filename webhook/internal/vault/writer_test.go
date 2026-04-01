package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Diixtra/aios/webhook/internal/document"
)

func TestWriter_WriteStub(t *testing.T) {
	tmpDir := t.TempDir()
	w := NewWriter(tmpDir)

	doc := &document.Document{
		ID:            42,
		Title:         "Self Assessment Tax Return 2024-25",
		Content:       "Dear Mr. Sherlock, your tax return...",
		Correspondent: "HMRC",
		Tags:          []string{"tax", "self-assessment"},
		Created:       time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		Added:         time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC),
		OriginalURL:   "https://paperless.lab.kazie.co.uk/documents/42",
	}

	path, err := w.WriteStub(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, "Knowledge", "Paperless", "HMRC", "self-assessment-tax-return-2024-25.md")
	if path != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read stub: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "title: Self Assessment Tax Return 2024-25") {
		t.Error("missing title in frontmatter")
	}
	if !strings.Contains(s, "type: paperless-document") {
		t.Error("missing type in frontmatter")
	}
	if !strings.Contains(s, "paperless_id: 42") {
		t.Error("missing paperless_id in frontmatter")
	}
	if !strings.Contains(s, "paperless_url: https://paperless.lab.kazie.co.uk/documents/42") {
		t.Error("missing paperless_url")
	}
	if !strings.Contains(s, "correspondent: HMRC") {
		t.Error("missing correspondent")
	}
	if !strings.Contains(s, "tags: [tax, self-assessment]") {
		t.Error("missing tags")
	}
	if !strings.Contains(s, "Dear Mr. Sherlock, your tax return...") {
		t.Error("missing OCR content in body")
	}
	if !strings.Contains(s, "[View in Paperless]") {
		t.Error("missing Paperless link")
	}
}

func TestWriter_WriteStub_NoCorrespondent(t *testing.T) {
	tmpDir := t.TempDir()
	w := NewWriter(tmpDir)

	doc := &document.Document{
		ID:          99,
		Title:       "Unknown Document",
		Content:     "Some content",
		Tags:        []string{},
		Created:     time.Now(),
		Added:       time.Now(),
		OriginalURL: "https://paperless.lab.kazie.co.uk/documents/99",
	}

	path, err := w.WriteStub(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, "Knowledge", "Paperless", "Uncategorised", "unknown-document.md")
	if path != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, path)
	}
}

func TestWriter_WriteStub_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	w := NewWriter(tmpDir)

	doc := &document.Document{
		ID:          42,
		Title:       "Test Doc",
		Content:     "Version 1",
		Tags:        []string{},
		Created:     time.Now(),
		Added:       time.Now(),
		OriginalURL: "https://paperless.lab.kazie.co.uk/documents/42",
	}

	path1, _ := w.WriteStub(doc)

	doc.Content = "Version 2"
	path2, _ := w.WriteStub(doc)

	if path1 != path2 {
		t.Error("paths should be identical for same document")
	}

	content, _ := os.ReadFile(path2)
	if !strings.Contains(string(content), "Version 2") {
		t.Error("stub should be overwritten with new content")
	}
}

func TestSanitiseFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello-world"},
		{"Invoice #123 (March)", "invoice-123-march"},
		{"  Multiple   Spaces  ", "multiple-spaces"},
		{"Special/Chars\\Here", "specialcharshere"},
		{strings.Repeat("a", 150), strings.Repeat("a", 100)},
	}

	for _, tt := range tests {
		got := sanitiseFilename(tt.input)
		if got != tt.expected {
			t.Errorf("sanitiseFilename(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestWriter_AddPaperlessLink(t *testing.T) {
	tmpDir := t.TempDir()
	w := NewWriter(tmpDir)

	notePath := filepath.Join(tmpDir, "Projects", "tax-project.md")
	os.MkdirAll(filepath.Dir(notePath), 0o755)
	os.WriteFile(notePath, []byte("---\ntitle: Tax Project 2025\ntype: project\nstatus: active\n---\n\n# Tax Project 2025\n\nSome content here.\n"), 0o644)

	err := w.AddPaperlessLink(notePath, "https://paperless.lab.kazie.co.uk/documents/42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(notePath)
	s := string(content)

	if !strings.Contains(s, "paperless:") {
		t.Error("missing paperless key in frontmatter")
	}
	if !strings.Contains(s, "  - https://paperless.lab.kazie.co.uk/documents/42") {
		t.Error("missing paperless URL in frontmatter")
	}
	if !strings.Contains(s, "Some content here.") {
		t.Error("original content lost")
	}
}

func TestWriter_AddPaperlessLink_ExistingList(t *testing.T) {
	tmpDir := t.TempDir()
	w := NewWriter(tmpDir)

	notePath := filepath.Join(tmpDir, "note.md")
	os.WriteFile(notePath, []byte("---\ntitle: Test\npaperless:\n  - https://paperless.lab.kazie.co.uk/documents/10\n---\n\nContent.\n"), 0o644)

	err := w.AddPaperlessLink(notePath, "https://paperless.lab.kazie.co.uk/documents/42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(notePath)
	s := string(content)

	if !strings.Contains(s, "  - https://paperless.lab.kazie.co.uk/documents/10") {
		t.Error("existing link lost")
	}
	if !strings.Contains(s, "  - https://paperless.lab.kazie.co.uk/documents/42") {
		t.Error("new link not added")
	}
}

func TestWriter_AddPaperlessLink_Duplicate(t *testing.T) {
	tmpDir := t.TempDir()
	w := NewWriter(tmpDir)

	notePath := filepath.Join(tmpDir, "note.md")
	os.WriteFile(notePath, []byte("---\ntitle: Test\npaperless:\n  - https://paperless.lab.kazie.co.uk/documents/42\n---\n\nContent.\n"), 0o644)

	err := w.AddPaperlessLink(notePath, "https://paperless.lab.kazie.co.uk/documents/42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(notePath)
	if strings.Count(string(content), "documents/42") != 1 {
		t.Error("duplicate link added")
	}
}
