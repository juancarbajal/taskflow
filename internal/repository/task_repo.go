package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"taskflow/internal/domain"
)

// TaskRepository manages Task persistence in SQLite.
type TaskRepository struct {
	db *sql.DB
}

// NewTaskRepository constructs a TaskRepository backed by the given SQLite *sql.DB.
func NewTaskRepository(db *sql.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

// Create inserts a new task and returns its auto-incremented ID.
func (r *TaskRepository) Create(ctx context.Context, task *domain.Task) (int64, error) {
	if err := task.Validate(); err != nil {
		return 0, fmt.Errorf("repository.Create: %w", err)
	}

	tagsJSON, err := marshalTags(task.Tags)
	if err != nil {
		return 0, fmt.Errorf("repository.Create marshal tags: %w", err)
	}

	const query = `
		INSERT INTO tasks (title, description, status, delegable_to_ai, tags, assigned_agent, ai_notes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`
	res, err := r.db.ExecContext(ctx, query,
		task.Title,
		task.Description,
		string(task.Status),
		task.DelegableToAI,
		tagsJSON,
		task.AssignedAgent,
		task.AINotes,
	)
	if err != nil {
		return 0, fmt.Errorf("repository.Create exec: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("repository.Create lastInsertId: %w", err)
	}
	task.ID = id
	return id, nil
}

// GetByID retrieves a single task by its ID.
func (r *TaskRepository) GetByID(ctx context.Context, id int64) (*domain.Task, error) {
	const query = `
		SELECT id, title, description, status, delegable_to_ai, tags, assigned_agent, ai_notes, created_at, updated_at
		FROM tasks
		WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)
	task, err := scanTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrTaskNotFound
		}
		return nil, fmt.Errorf("repository.GetByID: %w", err)
	}
	return task, nil
}

// ListAll returns all tasks ordered by ID ascending.
func (r *TaskRepository) ListAll(ctx context.Context) ([]domain.Task, error) {
	const query = `
		SELECT id, title, description, status, delegable_to_ai, tags, assigned_agent, ai_notes, created_at, updated_at
		FROM tasks
		ORDER BY id ASC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("repository.ListAll query: %w", err)
	}
	defer rows.Close()

	tasks := make([]domain.Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("repository.ListAll scan: %w", err)
		}
		tasks = append(tasks, *task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository.ListAll rows: %w", err)
	}
	return tasks, nil
}

// ListAvailableForAI returns all tasks with status='pending' AND delegable_to_ai=1,
// optionally filtered by tagFilter (SPEC-001 §4.2).
func (r *TaskRepository) ListAvailableForAI(ctx context.Context, tagFilter string) ([]domain.Task, error) {
	tagFilter = strings.TrimSpace(tagFilter)

	query := `
		SELECT id, title, description, status, delegable_to_ai, tags, assigned_agent, ai_notes, created_at, updated_at
		FROM tasks
		WHERE status = 'pending' AND delegable_to_ai = 1
	`
	var args []any
	if tagFilter != "" {
		query += ` AND tags LIKE ?`
		args = append(args, fmt.Sprintf("%%\"%s\"%%", tagFilter))
	}
	query += ` ORDER BY id ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repository.ListAvailableForAI query: %w", err)
	}
	defer rows.Close()

	tasks := make([]domain.Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("repository.ListAvailableForAI scan: %w", err)
		}
		tasks = append(tasks, *task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository.ListAvailableForAI rows: %w", err)
	}
	return tasks, nil
}

// ClaimTask atomically transitions a task from 'pending' (with delegable_to_ai=1)
// to 'in_progress' and assigns the agent name.
// If RowsAffected == 0, it returns domain.ErrTaskNotClaimable (SPEC-001 §4.2, CA-1).
func (r *TaskRepository) ClaimTask(ctx context.Context, taskID int64, agentName string) error {
	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		return fmt.Errorf("repository.ClaimTask: %w: agent_name cannot be empty", domain.ErrValidation)
	}

	const query = `
		UPDATE tasks
		SET status = 'in_progress',
		    assigned_agent = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
		  AND status = 'pending'
		  AND delegable_to_ai = 1
	`
	res, err := r.db.ExecContext(ctx, query, agentName, taskID)
	if err != nil {
		return fmt.Errorf("repository.ClaimTask exec: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository.ClaimTask rowsAffected: %w", err)
	}
	if affected == 0 {
		return domain.ErrTaskNotClaimable
	}
	return nil
}

// SubmitTaskResult atomically updates ai_notes and moves a task from 'in_progress'
// to 'review' (when requiresReview is true) or 'done' (when requiresReview is false).
func (r *TaskRepository) SubmitTaskResult(ctx context.Context, taskID int64, aiNotes string, requiresReview bool) error {
	targetStatus := domain.StatusDone
	if requiresReview {
		targetStatus = domain.StatusReview
	}

	const query = `
		UPDATE tasks
		SET status = ?,
		    ai_notes = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
		  AND status = 'in_progress'
	`
	res, err := r.db.ExecContext(ctx, query, string(targetStatus), aiNotes, taskID)
	if err != nil {
		return fmt.Errorf("repository.SubmitTaskResult exec: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository.SubmitTaskResult rowsAffected: %w", err)
	}
	if affected == 0 {
		return domain.ErrInvalidStatusTransition
	}
	return nil
}

// Update modifies a task's editable fields from the TUI form.
func (r *TaskRepository) Update(ctx context.Context, task *domain.Task) error {
	if err := task.Validate(); err != nil {
		return fmt.Errorf("repository.Update: %w", err)
	}
	tagsJSON, err := marshalTags(task.Tags)
	if err != nil {
		return fmt.Errorf("repository.Update marshal tags: %w", err)
	}

	const query = `
		UPDATE tasks
		SET title = ?,
		    description = ?,
		    status = ?,
		    delegable_to_ai = ?,
		    tags = ?,
		    assigned_agent = ?,
		    ai_notes = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	res, err := r.db.ExecContext(ctx, query,
		task.Title,
		task.Description,
		string(task.Status),
		task.DelegableToAI,
		tagsJSON,
		task.AssignedAgent,
		task.AINotes,
		task.ID,
	)
	if err != nil {
		return fmt.Errorf("repository.Update exec: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository.Update rowsAffected: %w", err)
	}
	if affected == 0 {
		return domain.ErrTaskNotFound
	}
	return nil
}

// ToggleDelegableToAI flips delegable_to_ai between true and false.
func (r *TaskRepository) ToggleDelegableToAI(ctx context.Context, taskID int64) error {
	const query = `
		UPDATE tasks
		SET delegable_to_ai = CASE WHEN delegable_to_ai = 1 THEN 0 ELSE 1 END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	res, err := r.db.ExecContext(ctx, query, taskID)
	if err != nil {
		return fmt.Errorf("repository.ToggleDelegableToAI exec: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository.ToggleDelegableToAI rowsAffected: %w", err)
	}
	if affected == 0 {
		return domain.ErrTaskNotFound
	}
	return nil
}

// ApproveTask transitions a task (typically in 'review') to 'done'.
func (r *TaskRepository) ApproveTask(ctx context.Context, taskID int64) error {
	const query = `
		UPDATE tasks
		SET status = 'done',
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	res, err := r.db.ExecContext(ctx, query, taskID)
	if err != nil {
		return fmt.Errorf("repository.ApproveTask exec: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository.ApproveTask rowsAffected: %w", err)
	}
	if affected == 0 {
		return domain.ErrTaskNotFound
	}
	return nil
}

// ReturnToPending moves a task back to 'pending' and clears assigned_agent (SPEC-001 §4.1.2).
func (r *TaskRepository) ReturnToPending(ctx context.Context, taskID int64) error {
	return r.MoveToStatus(ctx, taskID, domain.StatusPending, "")
}

// MoveToStatus transitions a task to any valid status from the TUI (SPEC-002).
// Moving to 'pending' clears assigned_agent; moving to 'in_progress' sets assigned_agent
// to actor if it was previously empty.
func (r *TaskRepository) MoveToStatus(ctx context.Context, taskID int64, status domain.Status, actor string) error {
	if !status.IsValid() {
		return fmt.Errorf("repository.MoveToStatus: %w: invalid status %q", domain.ErrValidation, status)
	}
	actor = strings.TrimSpace(actor)

	var (
		res sql.Result
		err error
	)
	switch status {
	case domain.StatusPending:
		res, err = r.db.ExecContext(ctx, `
			UPDATE tasks
			SET status = 'pending',
			    assigned_agent = '',
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, taskID)
	case domain.StatusInProgress:
		res, err = r.db.ExecContext(ctx, `
			UPDATE tasks
			SET status = 'in_progress',
			    assigned_agent = CASE WHEN assigned_agent = '' THEN ? ELSE assigned_agent END,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, actor, taskID)
	default:
		res, err = r.db.ExecContext(ctx, `
			UPDATE tasks
			SET status = ?,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, string(status), taskID)
	}
	if err != nil {
		return fmt.Errorf("repository.MoveToStatus exec: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository.MoveToStatus rowsAffected: %w", err)
	}
	if affected == 0 {
		return domain.ErrTaskNotFound
	}
	return nil
}

// Delete removes a task by ID.
func (r *TaskRepository) Delete(ctx context.Context, taskID int64) error {
	const query = `DELETE FROM tasks WHERE id = ?`
	res, err := r.db.ExecContext(ctx, query, taskID)
	if err != nil {
		return fmt.Errorf("repository.Delete exec: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository.Delete rowsAffected: %w", err)
	}
	if affected == 0 {
		return domain.ErrTaskNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTask(s rowScanner) (*domain.Task, error) {
	var (
		t         domain.Task
		statusStr string
		tagsRaw   sql.NullString
		createdAt any
		updatedAt any
	)
	err := s.Scan(
		&t.ID,
		&t.Title,
		&t.Description,
		&statusStr,
		&t.DelegableToAI,
		&tagsRaw,
		&t.AssignedAgent,
		&t.AINotes,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}
	t.Status = domain.Status(statusStr)
	t.Tags = unmarshalTags(tagsRaw.String)
	t.CreatedAt = parseSQLiteTime(createdAt)
	t.UpdatedAt = parseSQLiteTime(updatedAt)
	return &t, nil
}

func marshalTags(tags []string) (string, error) {
	if len(tags) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return "[]", err
	}
	return string(b), nil
}

func unmarshalTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil || tags == nil {
		return []string{}
	}
	return tags
}

func parseSQLiteTime(val any) time.Time {
	switch v := val.(type) {
	case time.Time:
		return v
	case string:
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05Z"} {
			if parsed, err := time.Parse(layout, v); err == nil {
				return parsed
			}
		}
	case []byte:
		return parseSQLiteTime(string(v))
	}
	return time.Time{}
}
