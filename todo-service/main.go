package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/amitrana/tasksis/todo-service/db"
)

// ── Types ─────────────────────────────────────────────────────────────────

type Todo struct {
	ID          int       `json:"id"`
	Subject     string    `json:"subject"`
	Description string    `json:"description"`
	Done        bool      `json:"done"`
	CreatedAt   time.Time `json:"created_at"`
}

type addTodoRequest struct {
	Subject     string `json:"subject"`
	Description string `json:"description"`
}

// userInfo is populated from the validated session (forwarded by frontend)
type userInfo struct {
	UserID   int
	UserName string
}

// ── Main ──────────────────────────────────────────────────────────────────

func main() {
	db.Connect()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	mux := http.NewServeMux()

	// Health check — used by k8s liveness/readiness probes
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","service":"todo-service"}`))
	})

	// Todo endpoints — all require X-User-ID and X-User-Name headers
	// (set by frontend after validating session with auth-service)
	mux.HandleFunc("/todos", requireUser(handleTodos))
	mux.HandleFunc("/todos/", requireUser(handleTodoByID))

	log.Printf("[todo-service] started on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

// ── Route dispatchers ─────────────────────────────────────────────────────

// handleTodos dispatches GET /todos and POST /todos
func handleTodos(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listTodos(w, r)
	case http.MethodPost:
		addTodo(w, r)
	default:
		jsonErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET or POST")
	}
}

// handleTodoByID dispatches PATCH /todos/{id}/toggle and DELETE /todos/{id}
func handleTodoByID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPatch:
		toggleTodo(w, r)
	case http.MethodDelete:
		deleteTodo(w, r)
	default:
		jsonErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "use PATCH or DELETE")
	}
}

// ── Handlers ──────────────────────────────────────────────────────────────

// GET /todos
func listTodos(w http.ResponseWriter, r *http.Request) {
	u := userFromCtx(r)

	rows, err := db.Pool.Query(context.Background(),
		`SELECT id, subject, description, done, created_at
		 FROM todos WHERE user_id = $1 ORDER BY created_at ASC`,
		u.UserID,
	)
	if err != nil {
		log.Printf("[todo-service] list error: %v", err)
		jsonErr(w, http.StatusInternalServerError, "fetch_failed", "could not fetch todos")
		return
	}
	defer rows.Close()

	todos := []Todo{}
	for rows.Next() {
		var t Todo
		if err := rows.Scan(&t.ID, &t.Subject, &t.Description, &t.Done, &t.CreatedAt); err != nil {
			jsonErr(w, http.StatusInternalServerError, "scan_failed", "could not read todos")
			return
		}
		todos = append(todos, t)
	}

	done := 0
	for _, t := range todos {
		if t.Done {
			done++
		}
	}

	jsonOK(w, http.StatusOK, map[string]interface{}{
		"todos":   todos,
		"total":   len(todos),
		"done":    done,
		"pending": len(todos) - done,
	})
}

// POST /todos
func addTodo(w http.ResponseWriter, r *http.Request) {
	u := userFromCtx(r)

	var req addTodoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	subject := strings.TrimSpace(req.Subject)
	if subject == "" {
		jsonErr(w, http.StatusBadRequest, "missing_subject", "subject is required")
		return
	}

	var t Todo
	err := db.Pool.QueryRow(context.Background(),
		`INSERT INTO todos (user_id, subject, description)
		 VALUES ($1, $2, $3)
		 RETURNING id, subject, description, done, created_at`,
		u.UserID, subject, strings.TrimSpace(req.Description),
	).Scan(&t.ID, &t.Subject, &t.Description, &t.Done, &t.CreatedAt)

	if err != nil {
		log.Printf("[todo-service] add error: %v", err)
		jsonErr(w, http.StatusInternalServerError, "create_failed", "could not create todo")
		return
	}

	jsonOK(w, http.StatusCreated, t)
}

// PATCH /todos/{id}/toggle
func toggleTodo(w http.ResponseWriter, r *http.Request) {
	u := userFromCtx(r)
	id, err := idFromPath(r.URL.Path, "/todos/", "/toggle")
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid_id", "todo ID must be a number")
		return
	}

	var t Todo
	err = db.Pool.QueryRow(context.Background(),
		`UPDATE todos SET done = NOT done
		 WHERE id = $1 AND user_id = $2
		 RETURNING id, subject, description, done, created_at`,
		id, u.UserID,
	).Scan(&t.ID, &t.Subject, &t.Description, &t.Done, &t.CreatedAt)

	if err != nil {
		jsonErr(w, http.StatusNotFound, "not_found", "todo not found")
		return
	}

	jsonOK(w, http.StatusOK, t)
}

// DELETE /todos/{id}
func deleteTodo(w http.ResponseWriter, r *http.Request) {
	u := userFromCtx(r)
	id, err := idFromPath(r.URL.Path, "/todos/", "")
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid_id", "todo ID must be a number")
		return
	}

	result, err := db.Pool.Exec(context.Background(),
		`DELETE FROM todos WHERE id = $1 AND user_id = $2`, id, u.UserID)
	if err != nil || result.RowsAffected() == 0 {
		jsonErr(w, http.StatusNotFound, "not_found", "todo not found")
		return
	}

	jsonOK(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// ── Middleware ─────────────────────────────────────────────────────────────

type ctxKey string

const ctxUser ctxKey = "user"

// requireUser checks that X-User-ID and X-User-Name headers are present.
// These are set by the frontend service after it validates the session
// with auth-service. Direct calls without these headers are rejected.
func requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := r.Header.Get("X-User-ID")
		userName := r.Header.Get("X-User-Name")

		if userIDStr == "" || userName == "" {
			jsonErr(w, http.StatusUnauthorized, "unauthorized", "missing user identity headers")
			return
		}

		userID, err := strconv.Atoi(userIDStr)
		if err != nil || userID <= 0 {
			jsonErr(w, http.StatusUnauthorized, "unauthorized", "invalid user ID header")
			return
		}

		ctx := context.WithValue(r.Context(), ctxUser, userInfo{
			UserID:   userID,
			UserName: userName,
		})
		next(w, r.WithContext(ctx))
	}
}

func userFromCtx(r *http.Request) userInfo {
	u, _ := r.Context().Value(ctxUser).(userInfo)
	return u
}

// ── Helpers ───────────────────────────────────────────────────────────────

// idFromPath extracts a numeric ID from a URL path segment.
// e.g. idFromPath("/todos/42/toggle", "/todos/", "/toggle") → 42
func idFromPath(path, prefix, suffix string) (int, error) {
	s := strings.TrimPrefix(path, prefix)
	s = strings.TrimSuffix(s, suffix)
	return strconv.Atoi(strings.TrimSpace(s))
}

func jsonOK(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   code,
		"message": message,
	})
}
