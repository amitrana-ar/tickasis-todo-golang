package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/amitrana/tasksis/auth-service/db"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12
const sessionTTL = 7 * 24 * time.Hour
const sessionCookie = "golive_session"

// ── Types ─────────────────────────────────────────────────────────────────

type signupRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Confirm  string `json:"confirm"`
}

type signinRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Message  string `json:"message"`
	UserID   int    `json:"user_id"`
	UserName string `json:"user_name"`
}

type sessionResponse struct {
	UserID   int    `json:"user_id"`
	UserName string `json:"user_name"`
}

// ── Errors ────────────────────────────────────────────────────────────────

var (
	errEmailTaken      = errors.New("an account with that email already exists")
	errInvalidPassword = errors.New("password must be at least 8 characters")
	errPasswordMatch   = errors.New("passwords do not match")
	errInvalidCreds    = errors.New("invalid email or password")
	errMissingFields   = errors.New("all fields are required")
)

// ── Main ──────────────────────────────────────────────────────────────────

func main() {
	db.Connect()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	mux := http.NewServeMux()

	// Health check — used by k8s liveness/readiness probes
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","service":"auth-service"}`))
	})

	// Auth endpoints
	mux.HandleFunc("/signup", handleSignup)
	mux.HandleFunc("/signin", handleSignin)
	mux.HandleFunc("/signout", handleSignout)
	mux.HandleFunc("/me", requireAuth(handleMe))
	mux.HandleFunc("/validate", handleValidate) // internal: called by other services

	log.Printf("[auth-service] started on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

// ── Handlers ──────────────────────────────────────────────────────────────

// POST /signup
func handleSignup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
		return
	}

	var req signupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	name := strings.TrimSpace(req.Name)
	email := strings.ToLower(strings.TrimSpace(req.Email))

	if name == "" || email == "" || req.Password == "" {
		jsonErr(w, http.StatusBadRequest, "missing_fields", errMissingFields.Error())
		return
	}
	if len(req.Password) < 8 {
		jsonErr(w, http.StatusBadRequest, "weak_password", errInvalidPassword.Error())
		return
	}
	if req.Password != req.Confirm {
		jsonErr(w, http.StatusBadRequest, "password_mismatch", errPasswordMatch.Error())
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal_error", "failed to hash password")
		return
	}

	var userID int
	err = db.Pool.QueryRow(context.Background(),
		`INSERT INTO users (name, email, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		name, email, string(hash),
	).Scan(&userID)

	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			jsonErr(w, http.StatusConflict, "email_taken", errEmailTaken.Error())
			return
		}
		log.Printf("[auth-service] signup error: %v", err)
		jsonErr(w, http.StatusInternalServerError, "internal_error", "signup failed")
		return
	}

	token, expires, err := createSession(userID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "session_error", "could not create session")
		return
	}

	setSessionCookie(w, token, expires)
	jsonOK(w, http.StatusCreated, authResponse{
		Message:  "account created",
		UserID:   userID,
		UserName: name,
	})
}

// POST /signin
func handleSignin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
		return
	}

	var req signinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || req.Password == "" {
		jsonErr(w, http.StatusBadRequest, "missing_fields", errMissingFields.Error())
		return
	}

	var userID int
	var userName, hash string
	err := db.Pool.QueryRow(context.Background(),
		`SELECT id, name, password_hash FROM users WHERE email = $1`, email,
	).Scan(&userID, &userName, &hash)

	if err != nil {
		bcrypt.CompareHashAndPassword([]byte("$2a$12$dummy"), []byte(req.Password)) // timing safety
		jsonErr(w, http.StatusUnauthorized, "invalid_credentials", errInvalidCreds.Error())
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		jsonErr(w, http.StatusUnauthorized, "invalid_credentials", errInvalidCreds.Error())
		return
	}

	token, expires, err := createSession(userID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "session_error", "could not create session")
		return
	}

	setSessionCookie(w, token, expires)
	jsonOK(w, http.StatusOK, authResponse{
		Message:  "signed in",
		UserID:   userID,
		UserName: userName,
	})
}

// POST /signout
func handleSignout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookie)
	if err == nil {
		db.Pool.Exec(context.Background(),
			`DELETE FROM sessions WHERE token = $1`, cookie.Value)
	}
	clearCookie(w)
	jsonOK(w, http.StatusOK, map[string]string{"message": "signed out"})
}

// GET /me  (protected)
func handleMe(w http.ResponseWriter, r *http.Request) {
	userID, userName := userFromCtx(r)
	jsonOK(w, http.StatusOK, sessionResponse{
		UserID:   userID,
		UserName: userName,
	})
}

// GET /validate?token=xxx  — internal endpoint called by frontend/todo-service
// Returns user info if token is valid, 401 otherwise
func handleValidate(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			jsonErr(w, http.StatusUnauthorized, "unauthorized", "no session token")
			return
		}
		token = cookie.Value
	}

	var userID int
	var userName string
	err := db.Pool.QueryRow(context.Background(),
		`SELECT u.id, u.name
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token = $1 AND s.expires_at > NOW()`,
		token,
	).Scan(&userID, &userName)

	if err != nil {
		jsonErr(w, http.StatusUnauthorized, "unauthorized", "invalid or expired session")
		return
	}

	jsonOK(w, http.StatusOK, sessionResponse{
		UserID:   userID,
		UserName: userName,
	})
}

// ── Middleware ─────────────────────────────────────────────────────────────

type ctxKey string

const (
	ctxUserID   ctxKey = "userID"
	ctxUserName ctxKey = "userName"
)

func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			jsonErr(w, http.StatusUnauthorized, "unauthorized", "session required")
			return
		}
		var userID int
		var userName string
		err = db.Pool.QueryRow(context.Background(),
			`SELECT u.id, u.name FROM sessions s
			 JOIN users u ON u.id = s.user_id
			 WHERE s.token = $1 AND s.expires_at > NOW()`,
			cookie.Value,
		).Scan(&userID, &userName)
		if err != nil {
			clearCookie(w)
			jsonErr(w, http.StatusUnauthorized, "unauthorized", "invalid or expired session")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserID, userID)
		ctx = context.WithValue(ctx, ctxUserName, userName)
		next(w, r.WithContext(ctx))
	}
}

func userFromCtx(r *http.Request) (int, string) {
	id, _ := r.Context().Value(ctxUserID).(int)
	name, _ := r.Context().Value(ctxUserName).(string)
	return id, name
}

// ── Helpers ───────────────────────────────────────────────────────────────

func createSession(userID int) (string, time.Time, error) {
	token := uuid.NewString()
	expires := time.Now().Add(sessionTTL)
	_, err := db.Pool.Exec(context.Background(),
		`INSERT INTO sessions (token, user_id, expires_at) VALUES ($1, $2, $3)`,
		token, userID, expires,
	)
	return token, expires, err
}

func setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
	})
}

func clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:    sessionCookie,
		Value:   "",
		Path:    "/",
		MaxAge:  -1,
		Expires: time.Unix(0, 0),
	})
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
