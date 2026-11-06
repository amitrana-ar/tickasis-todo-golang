package main

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

)

// ── Config ────────────────────────────────────────────────────────────────

var (
	authServiceURL = getenv("AUTH_SERVICE_URL", "http://auth-service:8081")
	todoServiceURL = getenv("TODO_SERVICE_URL", "http://todo-service:8082")
	sessionCookie  = "golive_session"
)

// ── Types ─────────────────────────────────────────────────────────────────

type sessionUser struct {
	UserID   int    `json:"user_id"`
	UserName string `json:"user_name"`
}

type Todo struct {
	ID          int       `json:"id"`
	Subject     string    `json:"subject"`
	Description string    `json:"description"`
	Done        bool      `json:"done"`
	CreatedAt   time.Time `json:"created_at"`
}

type todosResponse struct {
	Todos   []Todo `json:"todos"`
	Total   int    `json:"total"`
	Done    int    `json:"done"`
	Pending int    `json:"pending"`
}

type ctxKey string

const ctxUser ctxKey = "user"

// ── Templates ─────────────────────────────────────────────────────────────

var templates = template.Must(
	template.New("").Funcs(template.FuncMap{
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"slice": func(s string, start, end int) string {
			r := []rune(s)
			if start < 0 {
				start = 0
			}
			if end > len(r) {
				end = len(r)
			}
			if start >= end {
				return ""
			}
			return string(r[start:end])
		},
	}).ParseGlob("templates/*.html"),
)

// ── Main ──────────────────────────────────────────────────────────────────

func main() {
	port := getenv("PORT", "8080")

	mux := http.NewServeMux()

	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","service":"frontend"}`))
	})

	// Auth pages (public)
	mux.HandleFunc("/signup", routeMethod(handleSignupPage, handleSignup))
	mux.HandleFunc("/signin", routeMethod(handleSigninPage, handleSignin))
	mux.HandleFunc("/signout", handleSignout)

	// Todo dashboard + form actions (protected)
	mux.HandleFunc("/", requireSession(handleIndex))
	mux.HandleFunc("/todo/add", requireSession(handleAddTodo))
	mux.HandleFunc("/todo/toggle", requireSession(handleToggleTodo))
	mux.HandleFunc("/todo/delete", requireSession(handleDeleteTodo))

	log.Printf("[frontend] started on :%s", port)
	log.Printf("[frontend] auth-service → %s", authServiceURL)
	log.Printf("[frontend] todo-service → %s", todoServiceURL)

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

// ── Auth page handlers ────────────────────────────────────────────────────

func handleSignupPage(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "signup.html", map[string]string{"Error": ""})
}

func handleSigninPage(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "signin.html", map[string]string{"Error": ""})
}

func handleSignup(w http.ResponseWriter, r *http.Request) {
	body, _ := json.Marshal(map[string]string{
		"name":     r.FormValue("name"),
		"email":    r.FormValue("email"),
		"password": r.FormValue("password"),
		"confirm":  r.FormValue("confirm"),
	})

	resp, err := http.Post(authServiceURL+"/signup", "application/json", bytes.NewReader(body))
	if err != nil {
		renderPage(w, "signup.html", map[string]string{"Error": "Auth service unavailable."})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var e map[string]string
		json.NewDecoder(resp.Body).Decode(&e)
		renderPage(w, "signup.html", map[string]string{"Error": e["message"]})
		return
	}

	forwardCookies(w, resp)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleSignin(w http.ResponseWriter, r *http.Request) {
	body, _ := json.Marshal(map[string]string{
		"email":    r.FormValue("email"),
		"password": r.FormValue("password"),
	})

	resp, err := http.Post(authServiceURL+"/signin", "application/json", bytes.NewReader(body))
	if err != nil {
		renderPage(w, "signin.html", map[string]string{"Error": "Auth service unavailable."})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var e map[string]string
		json.NewDecoder(resp.Body).Decode(&e)
		renderPage(w, "signin.html", map[string]string{"Error": e["message"]})
		return
	}

	forwardCookies(w, resp)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleSignout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		req, _ := http.NewRequest(http.MethodPost, authServiceURL+"/signout", nil)
		req.AddCookie(cookie)
		http.DefaultClient.Do(req)
	}
	clearCookie(w)
	http.Redirect(w, r, "/signin", http.StatusSeeOther)
}

// ── Todo page handlers ────────────────────────────────────────────────────

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	user := userFromCtx(r)

	req, _ := http.NewRequest(http.MethodGet, todoServiceURL+"/todos", nil)
	req.Header.Set("X-User-ID", strconv.Itoa(user.UserID))
	req.Header.Set("X-User-Name", user.UserName)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "todo service unavailable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	var data todosResponse
	json.NewDecoder(resp.Body).Decode(&data)
	if data.Todos == nil {
		data.Todos = []Todo{}
	}

	renderPage(w, "index.html", struct {
		Todos    []Todo
		Total    int
		Done     int
		UserName string
	}{data.Todos, data.Total, data.Done, user.UserName})
}

func handleAddTodo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	user := userFromCtx(r)
	body, _ := json.Marshal(map[string]string{
		"subject":     r.FormValue("subject"),
		"description": r.FormValue("description"),
	})
	req, _ := http.NewRequest(http.MethodPost, todoServiceURL+"/todos", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", strconv.Itoa(user.UserID))
	req.Header.Set("X-User-Name", user.UserName)
	http.DefaultClient.Do(req)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleToggleTodo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	user := userFromCtx(r)
	id := r.FormValue("id")
	req, _ := http.NewRequest(http.MethodPatch, todoServiceURL+"/todos/"+id+"/toggle", nil)
	req.Header.Set("X-User-ID", strconv.Itoa(user.UserID))
	req.Header.Set("X-User-Name", user.UserName)
	http.DefaultClient.Do(req)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleDeleteTodo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	user := userFromCtx(r)
	id := r.FormValue("id")
	req, _ := http.NewRequest(http.MethodDelete, todoServiceURL+"/todos/"+id, nil)
	req.Header.Set("X-User-ID", strconv.Itoa(user.UserID))
	req.Header.Set("X-User-Name", user.UserName)
	http.DefaultClient.Do(req)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ── Middleware ─────────────────────────────────────────────────────────────

// requireSession validates the session cookie by calling auth-service /validate.
// On success: injects the user into context. On failure: redirects to /signin.
func requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			http.Redirect(w, r, "/signin", http.StatusSeeOther)
			return
		}

		req, _ := http.NewRequest(http.MethodGet,
			authServiceURL+"/validate?token="+cookie.Value, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			clearCookie(w)
			http.Redirect(w, r, "/signin", http.StatusSeeOther)
			if resp != nil {
				resp.Body.Close()
			}
			return
		}
		defer resp.Body.Close()

		var user sessionUser
		if err := json.NewDecoder(resp.Body).Decode(&user); err != nil || user.UserID == 0 {
			clearCookie(w)
			http.Redirect(w, r, "/signin", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), ctxUser, user)
		next(w, r.WithContext(ctx))
	}
}

func userFromCtx(r *http.Request) sessionUser {
	u, _ := r.Context().Value(ctxUser).(sessionUser)
	return u
}

// ── Helpers ───────────────────────────────────────────────────────────────

func renderPage(w http.ResponseWriter, tmpl string, data interface{}) {
	if err := templates.ExecuteTemplate(w, tmpl, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func forwardCookies(w http.ResponseWriter, resp *http.Response) {
	for _, c := range resp.Cookies() {
		http.SetCookie(w, c)
	}
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

// routeMethod calls getHandler for GET, postHandler for POST.
func routeMethod(getHandler, postHandler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postHandler(w, r)
		} else {
			getHandler(w, r)
		}
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}


