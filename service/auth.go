// Package service contains business logic for the Tickasis TO-DO app.
// It has no dependency on net/http — pure logic only.
package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/amitrana/golive/db"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// ── Errors ────────────────────────────────────────────────────────────────

var (
	ErrEmailTaken       = errors.New("an account with that email already exists")
	ErrInvalidPassword  = errors.New("password must be at least 8 characters")
	ErrPasswordMismatch = errors.New("passwords do not match")
	ErrInvalidCreds     = errors.New("invalid email or password")
	ErrMissingFields    = errors.New("all fields are required")
)

const bcryptCost = 12
const sessionTTL = 7 * 24 * time.Hour

// ── Types ─────────────────────────────────────────────────────────────────

// SignupInput holds validated fields for creating a new account.
type SignupInput struct {
	Name     string
	Email    string
	Password string
	Confirm  string
}

// SigninInput holds credentials for signing in.
type SigninInput struct {
	Email    string
	Password string
}

// SessionInfo holds user data returned after a successful auth.
type SessionInfo struct {
	Token    string
	UserID   int
	UserName string
	Expires  time.Time
}

// ── Auth Service ──────────────────────────────────────────────────────────

// Signup validates input, hashes the password, inserts the user,
// and creates a session. Returns session info on success.
func Signup(input SignupInput) (*SessionInfo, error) {
	name := strings.TrimSpace(input.Name)
	email := strings.ToLower(strings.TrimSpace(input.Email))
	password := input.Password
	confirm := input.Confirm

	if name == "" || email == "" || password == "" {
		return nil, ErrMissingFields
	}
	if len(password) < 8 {
		return nil, ErrInvalidPassword
	}
	if password != confirm {
		return nil, ErrPasswordMismatch
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, err
	}

	var userID int
	err = db.Pool.QueryRow(context.Background(),
		`INSERT INTO users (name, email, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		name, email, string(hash),
	).Scan(&userID)

	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			return nil, ErrEmailTaken
		}
		return nil, err
	}

	return createSession(userID, name)
}

// Signin validates credentials and returns a new session on success.
func Signin(input SigninInput) (*SessionInfo, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	password := input.Password

	if email == "" || password == "" {
		return nil, ErrMissingFields
	}

	var userID int
	var userName string
	var hash string

	err := db.Pool.QueryRow(context.Background(),
		`SELECT id, name, password_hash FROM users WHERE email = $1`, email,
	).Scan(&userID, &userName, &hash)

	if err != nil {
		// Dummy bcrypt call to prevent timing-based user enumeration
		bcrypt.CompareHashAndPassword([]byte("$2a$12$dummy"), []byte(password))
		return nil, ErrInvalidCreds
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, ErrInvalidCreds
	}

	return createSession(userID, userName)
}

// Signout deletes the session token from the database.
func Signout(token string) error {
	if token == "" {
		return nil
	}
	_, err := db.Pool.Exec(context.Background(),
		`DELETE FROM sessions WHERE token = $1`, token)
	return err
}

// ValidateSession checks if a token is valid and returns user info.
func ValidateSession(token string) (userID int, userName string, err error) {
	err = db.Pool.QueryRow(context.Background(),
		`SELECT u.id, u.name
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.token = $1 AND s.expires_at > NOW()`,
		token,
	).Scan(&userID, &userName)
	return
}

// ── Internal ──────────────────────────────────────────────────────────────

// createSession inserts a new session row and returns session info.
func createSession(userID int, userName string) (*SessionInfo, error) {
	token := uuid.NewString()
	expires := time.Now().Add(sessionTTL)

	_, err := db.Pool.Exec(context.Background(),
		`INSERT INTO sessions (token, user_id, expires_at) VALUES ($1, $2, $3)`,
		token, userID, expires,
	)
	if err != nil {
		return nil, err
	}

	return &SessionInfo{
		Token:    token,
		UserID:   userID,
		UserName: userName,
		Expires:  expires,
	}, nil
}
