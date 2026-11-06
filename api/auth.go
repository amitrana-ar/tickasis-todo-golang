// Package api provides JSON REST API handlers for the Tickasis TO-DO app.
// All endpoints consume and produce application/json.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/amitrana/golive/middleware"
	"github.com/amitrana/golive/service"
)

// ── Request / Response types ──────────────────────────────────────────────

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

// ── Handlers ──────────────────────────────────────────────────────────────

// Signup godoc
// POST /api/auth/signup
// Body: {"name":"Amit Rana","email":"amit@example.com","password":"pass1234","confirm":"pass1234"}
// Response 201: {"message":"account created","user_id":1,"user_name":"Amit Rana"}
func Signup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
		return
	}

	var req signupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	session, err := service.Signup(service.SignupInput{
		Name:     req.Name,
		Email:    req.Email,
		Password: req.Password,
		Confirm:  req.Confirm,
	})

	if err != nil {
		switch {
		case errors.Is(err, service.ErrMissingFields):
			jsonErr(w, http.StatusBadRequest, "missing_fields", err.Error())
		case errors.Is(err, service.ErrInvalidPassword):
			jsonErr(w, http.StatusBadRequest, "weak_password", err.Error())
		case errors.Is(err, service.ErrPasswordMismatch):
			jsonErr(w, http.StatusBadRequest, "password_mismatch", err.Error())
		case errors.Is(err, service.ErrEmailTaken):
			jsonErr(w, http.StatusConflict, "email_taken", err.Error())
		default:
			jsonErr(w, http.StatusInternalServerError, "internal_error", "signup failed")
		}
		return
	}

	setSessionCookie(w, session)
	jsonOK(w, http.StatusCreated, authResponse{
		Message:  "account created",
		UserID:   session.UserID,
		UserName: session.UserName,
	})
}

// Signin godoc
// POST /api/auth/signin
// Body: {"email":"amit@example.com","password":"pass1234"}
// Response 200: {"message":"signed in","user_id":1,"user_name":"Amit Rana"}
func Signin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
		return
	}

	var req signinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	session, err := service.Signin(service.SigninInput{
		Email:    req.Email,
		Password: req.Password,
	})

	if err != nil {
		switch {
		case errors.Is(err, service.ErrMissingFields):
			jsonErr(w, http.StatusBadRequest, "missing_fields", err.Error())
		case errors.Is(err, service.ErrInvalidCreds):
			jsonErr(w, http.StatusUnauthorized, "invalid_credentials", err.Error())
		default:
			jsonErr(w, http.StatusInternalServerError, "internal_error", "signin failed")
		}
		return
	}

	setSessionCookie(w, session)
	jsonOK(w, http.StatusOK, authResponse{
		Message:  "signed in",
		UserID:   session.UserID,
		UserName: session.UserName,
	})
}

// Signout godoc
// POST /api/auth/signout
// Response 200: {"message":"signed out"}
func Signout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
		return
	}

	cookie, err := r.Cookie(middleware.SessionCookie)
	if err == nil {
		service.Signout(cookie.Value)
	}

	middleware.ClearCookie(w)
	jsonOK(w, http.StatusOK, map[string]string{"message": "signed out"})
}

// Me godoc
// GET /api/auth/me
// Response 200: {"user_id":1,"user_name":"Amit Rana"}
// Response 401: if not authenticated
func Me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET")
		return
	}

	userID, userName := middleware.UserFromCtx(r)
	jsonOK(w, http.StatusOK, map[string]interface{}{
		"user_id":   userID,
		"user_name": userName,
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────

func setSessionCookie(w http.ResponseWriter, s *service.SessionInfo) {
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookie,
		Value:    s.Token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  s.Expires,
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
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}
