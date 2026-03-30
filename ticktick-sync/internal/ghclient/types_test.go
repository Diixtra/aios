package ghclient

import "testing"

func TestMakeAndParseTickTickMarker(t *testing.T) {
	marker := MakeTickTickMarker("proj123", "task456")
	ref := ParseTickTickMarker("Some issue body\n\n" + marker)
	if ref == nil {
		t.Fatal("expected non-nil TickTickRef")
	}
	if ref.ProjectID != "proj123" {
		t.Errorf("ProjectID = %q, want %q", ref.ProjectID, "proj123")
	}
	if ref.TaskID != "task456" {
		t.Errorf("TaskID = %q, want %q", ref.TaskID, "task456")
	}
}

func TestMakeAndParseGitHubMarker(t *testing.T) {
	marker := MakeGitHubMarker("Diixtra/aios", 42)
	ref := ParseGitHubMarker("Task description\n\n" + marker)
	if ref == nil {
		t.Fatal("expected non-nil GitHubRef")
	}
	if ref.Repo != "Diixtra/aios" {
		t.Errorf("Repo = %q, want %q", ref.Repo, "Diixtra/aios")
	}
	if ref.Number != 42 {
		t.Errorf("Number = %d, want %d", ref.Number, 42)
	}
}

func TestParseTickTickMarker_NoMatch(t *testing.T) {
	ref := ParseTickTickMarker("no marker here")
	if ref != nil {
		t.Errorf("expected nil, got %+v", ref)
	}
}

func TestParseGitHubMarker_NoMatch(t *testing.T) {
	ref := ParseGitHubMarker("no marker here")
	if ref != nil {
		t.Errorf("expected nil, got %+v", ref)
	}
}

func TestAppendMarker_Empty(t *testing.T) {
	result := AppendMarker("", "<!-- test -->")
	if result != "<!-- test -->" {
		t.Errorf("got %q, want %q", result, "<!-- test -->")
	}
}

func TestAppendMarker_Existing(t *testing.T) {
	text := "body\n\n<!-- test -->"
	result := AppendMarker(text, "<!-- test -->")
	if result != text {
		t.Errorf("marker was duplicated: %q", result)
	}
}

func TestAppendMarker_New(t *testing.T) {
	result := AppendMarker("existing body", "<!-- test -->")
	want := "existing body\n\n<!-- test -->"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}
