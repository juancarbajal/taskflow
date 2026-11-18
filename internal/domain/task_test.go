package domain

import (
	"errors"
	"testing"
)

func TestTaskValidate(t *testing.T) {
	t.Run("valid task defaults", func(t *testing.T) {
		task := &Task{
			Title:         "  Implementar SQLite WAL  ",
			DelegableToAI: true,
			Tags:          []string{" go ", "", "sqlite"},
		}
		if err := task.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if task.Title != "Implementar SQLite WAL" {
			t.Errorf("expected trimmed title, got %q", task.Title)
		}
		if task.Status != StatusPending {
			t.Errorf("expected default status %q, got %q", StatusPending, task.Status)
		}
		if len(task.Tags) != 2 || task.Tags[0] != "go" || task.Tags[1] != "sqlite" {
			t.Errorf("unexpected tags after normalize: %+v", task.Tags)
		}
	})

	t.Run("empty title fails validation", func(t *testing.T) {
		task := &Task{Title: "   "}
		err := task.Validate()
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("invalid status fails validation", func(t *testing.T) {
		task := &Task{Title: "Valid title", Status: Status("invalid")}
		err := task.Validate()
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})
}
