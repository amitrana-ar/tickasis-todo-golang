// Package middleware provides HTTP middleware for the Tickasis TO-DO app.
package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/amitrana/golive/db"
)

// contextKey is unexported to avoid collisions with other packages.
type contextKey string

const (
	UserIDKey   contextKey = "userID"
	UserNameKey contextKey = "userName"
)

// SessionCookie is the name of the session cookie.
const SessionCookie = "golive_session"

// RequireAuth validates the session cookie.
// On success: injects userID + userName into request context, calls next.
// On failure: 401 JSON for /api/ routes, redirect to /signin for page routes.
func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookie)
		if err != nil {
			rejectUnauthorized(w, r)
			return
		}

		var userID int
		var userName string

		err = db.Pool.QueryRow(context.Background(),
			`SELECT u.id, u.name
			 FROM sessions s
			 JOIN users u ON u.id = s.user_id
			 WHERE s.token = $1 AND s.expires_at > NOW()`,
			cookie.Value,
		).Scan(&userID, &userName)

		if err != nil {
			ClearCookie(w)
			rejectUnauthorized(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		ctx = context.WithValue(ctx, UserNameKey, userName)
		next(w, r.WithContext(ctx))
	}
}

// UserFromCtx extracts userID and userName injected by RequireAuth.
func UserFromCtx(r *http.Request) (int, string) {
	userID, _ := r.Context().Value(UserIDKey).(int)
	userName, _ := r.Context().Value(UserNameKey).(string)
	return userID, userName
}

// IsLoggedIn checks whether the request carries a valid active session.
func IsLoggedIn(r *http.Request) bool {
	cookie, err := r.Cookie(SessionCookie)
	if err != nil {
		return false
	}
	var id int
	err = db.Pool.QueryRow(context.Background(),
		`SELECT user_id FROM sessions WHERE token = $1 AND expires_at > NOW()`,
		cookie.Value,
	).Scan(&id)
	return err == nil
}

// ClearCookie expires the session cookie in the browser.
func ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:    SessionCookie,
		Value:   "",
		Path:    "/",
		MaxAge:  -1,
		Expires: time.Unix(0, 0),
	})
}

// rejectUnauthorized returns 401 JSON for API calls, redirect for browsers.
func rejectUnauthorized(w http.ResponseWriter, r *http.Request) {
	if isAPIRequest(r) {
		jsonError(w, http.StatusUnauthorized, "unauthorized", "valid session required")
		return
	}
	http.Redirect(w, r, "/signin", http.StatusSeeOther)
}

// isAPIRequest returns true if the request is targeting an /api/ route.
func isAPIRequest(r *http.Request) bool {
	return len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api"
}

// jsonError writes a JSON error response.
func jsonError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(`{"error":"` + code + `","message":"` + message + `"}`))
}
