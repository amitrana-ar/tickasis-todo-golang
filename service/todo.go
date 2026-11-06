package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/amitrana/golive/db"
)

// ── Errors ────────────────────────────────────────────────────────────────

var (
	ErrSubjectRequired = errors.New("task subject is required")
	ErrTodoNotFound    = errors.New("todo not found")
)

// ── Types ─────────────────────────────────────────────────────────────────

// Todo represents a single todo item returned by the service layer.
type Todo struct {
	ID          int       `json:"id"`
	Subject     string    `json:"subject"`
	Description string    `json:"description"`
	Done        bool      `json:"done"`
	CreatedAt   time.Time `json:"created_at"`
}

// AddTodoInput holds validated fields for creating a task.
type AddTodoInput struct {
	UserID      int
	Subject     string
	Description string
}

// ── Todo Service ──────────────────────────────────────────────────────────

// ListTodos returns all todos for the given user, ordered by creation time.
func ListTodos(userID int) ([]Todo, error) {
	rows, err := db.Pool.Query(context.Background(),
		`SELECT id, subject, description, done, created_at
		 FROM todos
		 WHERE user_id = $1
		 ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var todos []Todo
	for rows.Next() {
		var t Todo
		if err := rows.Scan(&t.ID, &t.Subject, &t.Description, &t.Done, &t.CreatedAt); err != nil {
			return nil, err
		}
		todos = append(todos, t)
	}
	return todos, nil
}

// AddTodo creates a new todo for the given user.
func AddTodo(input AddTodoInput) (*Todo, error) {
	subject := strings.TrimSpace(input.Subject)
	description := strings.TrimSpace(input.Description)

	if subject == "" {
		return nil, ErrSubjectRequired
	}

	var t Todo
	err := db.Pool.QueryRow(context.Background(),
		`INSERT INTO todos (user_id, subject, description)
		 VALUES ($1, $2, $3)
		 RETURNING id, subject, description, done, created_at`,
		input.UserID, subject, description,
	).Scan(&t.ID, &t.Subject, &t.Description, &t.Done, &t.CreatedAt)

	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ToggleTodo flips the done status of a todo owned by userID.
// Returns ErrTodoNotFound if the todo doesn't exist or belongs to another user.
func ToggleTodo(todoID, userID int) (*Todo, error) {
	var t Todo
	err := db.Pool.QueryRow(context.Background(),
		`UPDATE todos SET done = NOT done
		 WHERE id = $1 AND user_id = $2
		 RETURNING id, subject, description, done, created_at`,
		todoID, userID,
	).Scan(&t.ID, &t.Subject, &t.Description, &t.Done, &t.CreatedAt)

	if err != nil {
		return nil, ErrTodoNotFound
	}
	return &t, nil
}

// DeleteTodo removes a todo owned by userID.
// Returns ErrTodoNotFound if the todo doesn't exist or belongs to another user.
func DeleteTodo(todoID, userID int) error {
	result, err := db.Pool.Exec(context.Background(),
		`DELETE FROM todos WHERE id = $1 AND user_id = $2`,
		todoID, userID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrTodoNotFound
	}
	return nil
}

// CountDone returns the number of completed todos for a user.
func CountDone(todos []Todo) int {
	n := 0
	for _, t := range todos {
		if t.Done {
			n++
		}
	}
	return n
}
