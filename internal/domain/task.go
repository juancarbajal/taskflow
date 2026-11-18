package domain

import (
	"errors"
	"strings"
	"time"
)

// Status represents the lifecycle state of a Task.
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusReview     Status = "review"
	StatusDone       Status = "done"
)

// IsValid checks whether the status is one of the four allowed values in SPEC-001.
func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusInProgress, StatusReview, StatusDone:
		return true
	default:
		return false
	}
}

// Sentinel domain errors.
var (
	ErrTaskNotFound            = errors.New("task not found")
	ErrTaskNotClaimable        = errors.New("task does not exist, is not delegable to AI, or was already claimed by another agent")
	ErrInvalidStatusTransition = errors.New("invalid task status transition")
	ErrValidation              = errors.New("task validation error")
)

// Task represents a personal or AI-delegable task in taskflow.
type Task struct {
	ID            int64     `json:"id"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Status        Status    `json:"status"`
	DelegableToAI bool      `json:"delegable_to_ai"`
	Tags          []string  `json:"tags"`
	AssignedAgent string    `json:"assigned_agent"`
	AINotes       string    `json:"ai_notes"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Normalize cleans up whitespace, sets default Status and ensures Tags is non-nil.
func (t *Task) Normalize() {
	t.Title = strings.TrimSpace(t.Title)
	t.Description = strings.TrimSpace(t.Description)
	t.AssignedAgent = strings.TrimSpace(t.AssignedAgent)
	if t.Status == "" {
		t.Status = StatusPending
	}
	if t.Tags == nil {
		t.Tags = []string{}
	} else {
		cleaned := make([]string, 0, len(t.Tags))
		for _, tag := range t.Tags {
			trimmed := strings.TrimSpace(tag)
			if trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		t.Tags = cleaned
	}
}

// Validate verifies domain invariants for a Task.
func (t *Task) Validate() error {
	t.Normalize()
	if t.Title == "" {
		return errors.Join(ErrValidation, errors.New("title cannot be empty"))
	}
	if !t.Status.IsValid() {
		return errors.Join(ErrValidation, errors.New("invalid status"))
	}
	return nil
}
