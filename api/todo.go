package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/amitrana/golive/middleware"
	"github.com/amitrana/golive/service"
)

// ── Request types ─────────────────────────────────────────────────────────

type addTodoRequest struct {
	Subject     string `json:"subject"`
	Description string `json:"description"`
}

// ── Handlers ──────────────────────────────────────────────────────────────

// ListTodos godoc
// GET /api/todos
// Response 200: [{"id":1,"subject":"...","description":"...","done":false,"created_at":"..."}]
func ListTodos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET")
		return
	}

	userID, _ := middleware.UserFromCtx(r)

	todos, err := service.ListTodos(userID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "fetch_failed", "could not fetch todos")
		return
	}

	// Return empty array instead of null when no todos
	if todos == nil {
		todos = []service.Todo{}
	}

	done := service.CountDone(todos)
	jsonOK(w, http.StatusOK, map[string]interface{}{
		"todos":   todos,
		"total":   len(todos),
		"done":    done,
		"pending": len(todos) - done,
	})
}

// AddTodo godoc
// POST /api/todos
// Body: {"subject":"Learn Go","description":"Read the docs"}
// Response 201: {"id":1,"subject":"Learn Go","description":"Read the docs","done":false,"created_at":"..."}
func AddTodo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
		return
	}

	var req addTodoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	userID, _ := middleware.UserFromCtx(r)

	todo, err := service.AddTodo(service.AddTodoInput{
		UserID:      userID,
		Subject:     req.Subject,
		Description: req.Description,
	})

	if err != nil {
		if errors.Is(err, service.ErrSubjectRequired) {
			jsonErr(w, http.StatusBadRequest, "missing_subject", err.Error())
			return
		}
		jsonErr(w, http.StatusInternalServerError, "create_failed", "could not create todo")
		return
	}

	jsonOK(w, http.StatusCreated, todo)
}

// ToggleTodo godoc
// PATCH /api/todos/{id}/toggle
// Response 200: {"id":1,"subject":"...","done":true,...}
func ToggleTodo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		jsonErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "use PATCH")
		return
	}

	todoID, err := idFromPath(r.URL.Path, "/api/todos/", "/toggle")
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid_id", "todo ID must be a number")
		return
	}

	userID, _ := middleware.UserFromCtx(r)

	todo, err := service.ToggleTodo(todoID, userID)
	if err != nil {
		if errors.Is(err, service.ErrTodoNotFound) {
			jsonErr(w, http.StatusNotFound, "not_found", "todo not found")
			return
		}
		jsonErr(w, http.StatusInternalServerError, "toggle_failed", "could not toggle todo")
		return
	}

	jsonOK(w, http.StatusOK, todo)
}

// DeleteTodo godoc
// DELETE /api/todos/{id}
// Response 200: {"message":"deleted"}
func DeleteTodo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		jsonErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "use DELETE")
		return
	}

	todoID, err := idFromPath(r.URL.Path, "/api/todos/", "")
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid_id", "todo ID must be a number")
		return
	}

	userID, _ := middleware.UserFromCtx(r)

	if err := service.DeleteTodo(todoID, userID); err != nil {
		if errors.Is(err, service.ErrTodoNotFound) {
			jsonErr(w, http.StatusNotFound, "not_found", "todo not found")
			return
		}
		jsonErr(w, http.StatusInternalServerError, "delete_failed", "could not delete todo")
		return
	}

	jsonOK(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// ── Helpers ───────────────────────────────────────────────────────────────

// idFromPath extracts a numeric ID from a URL path.
// e.g. idFromPath("/api/todos/42/toggle", "/api/todos/", "/toggle") → 42
func idFromPath(path, prefix, suffix string) (int, error) {
	path = strings.TrimPrefix(path, prefix)
	path = strings.TrimSuffix(path, suffix)
	return strconv.Atoi(strings.TrimSpace(path))
}
