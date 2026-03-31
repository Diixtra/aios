package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Diixtra/aios/webhook/internal/paperless"
)

// Writer creates and updates stub notes in the Obsidian vault.
type Writer struct {
	vaultPath string
}

// NewWriter creates a vault writer rooted at the given path.
func NewWriter(vaultPath string) *Writer {
	return &Writer{vaultPath: vaultPath}
}

// WriteStub creates a stub markdown note for a Paperless document.
// Returns the absolute path of the written file.
// If a stub already exists for the same document, it is overwritten.
func (w *Writer) WriteStub(doc *paperless.Document) (string, error) {
	correspondent := doc.Correspondent
	if correspondent == "" {
		correspondent = "Uncategorised"
	}

	filename := sanitiseFilename(doc.Title) + ".md"
	dir := filepath.Join(w.vaultPath, "Knowledge", "Paperless", correspondent)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create directory %s: %w", dir, err)
	}

	path := filepath.Join(dir, filename)

	tagsFormatted := "[]"
	if len(doc.Tags) > 0 {
		tagsFormatted = "[" + strings.Join(doc.Tags, ", ") + "]"
	}

	content := fmt.Sprintf(`---
title: %s
type: paperless-document
source: paperless
paperless_id: %d
paperless_url: %s
correspondent: %s
tags: %s
created: %s
added: %s
entity: []
status: active
---

# %s

[View in Paperless](%s)

## Content

%s
`,
		doc.Title,
		doc.ID,
		doc.OriginalURL,
		correspondent,
		tagsFormatted,
		doc.Created.Format(time.DateOnly),
		doc.Added.Format(time.DateOnly),
		doc.Title,
		doc.OriginalURL,
		doc.Content,
	)

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write stub %s: %w", path, err)
	}

	return path, nil
}

// AddPaperlessLink adds a Paperless URL to a vault note's frontmatter.
// If the note already has a paperless list, the URL is appended (if not duplicate).
// If not, a new paperless list is created in the frontmatter.
func (w *Writer) AddPaperlessLink(notePath, paperlessURL string) error {
	content, err := os.ReadFile(notePath)
	if err != nil {
		return fmt.Errorf("read note %s: %w", notePath, err)
	}

	s := string(content)

	// Check for duplicate
	if strings.Contains(s, paperlessURL) {
		return nil
	}

	// Find frontmatter boundaries
	if !strings.HasPrefix(s, "---\n") {
		return fmt.Errorf("note %s has no frontmatter", notePath)
	}
	endIdx := strings.Index(s[4:], "\n---")
	if endIdx == -1 {
		return fmt.Errorf("note %s has unclosed frontmatter", notePath)
	}
	endIdx += 4 // offset for the initial "---\n"

	frontmatter := s[4:endIdx]
	body := s[endIdx+4:] // skip "\n---"

	entry := fmt.Sprintf("  - %s", paperlessURL)

	if strings.Contains(frontmatter, "paperless:") {
		frontmatter = insertAfterPaperlessList(frontmatter, entry)
	} else {
		frontmatter = frontmatter + "\npaperless:\n" + entry
	}

	result := "---\n" + frontmatter + "\n---" + body
	return os.WriteFile(notePath, []byte(result), 0o644)
}

func insertAfterPaperlessList(frontmatter, entry string) string {
	lines := strings.Split(frontmatter, "\n")
	var result []string
	inPaperless := false
	inserted := false

	for _, line := range lines {
		if strings.TrimSpace(line) == "paperless:" {
			inPaperless = true
			result = append(result, line)
			continue
		}

		if inPaperless && strings.HasPrefix(line, "  - ") {
			result = append(result, line)
			continue
		}

		if inPaperless && !strings.HasPrefix(line, "  - ") {
			result = append(result, entry)
			inserted = true
			inPaperless = false
		}

		result = append(result, line)
	}

	// If paperless was the last key in frontmatter
	if inPaperless && !inserted {
		result = append(result, entry)
	}

	return strings.Join(result, "\n")
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9\-]+`)
var multiDash = regexp.MustCompile(`-{2,}`)

// sanitiseFilename converts a title to a safe, lowercase, hyphenated filename (without extension).
func sanitiseFilename(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphanumeric.ReplaceAllString(s, "")
	s = multiDash.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 100 {
		s = s[:100]
	}
	return s
}
