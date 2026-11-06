// Package handler renders HTML pages for the Tickasis TO-DO app.
// It delegates all business logic to the service layer.
package handler

import (
	"html/template"
	"net/http"

	"github.com/amitrana/golive/middleware"
	"github.com/amitrana/golive/service"
)

// templates is parsed once at startup with all custom helper functions.
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
		// slice returns s[start:end] safely — used for avatar initial
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

// ── Auth pages ────────────────────────────────────────────────────────────

// SigninPage renders GET /signin
func SigninPage(w http.ResponseWriter, r *http.Request) {
	if middleware.IsLoggedIn(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	renderPage(w, "signin.html", map[string]string{"Error": ""})
}

// SignupPage renders GET /signup
func SignupPage(w http.ResponseWriter, r *http.Request) {
	if middleware.IsLoggedIn(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	renderPage(w, "signup.html", map[string]string{"Error": ""})
}

// ── Todo page ─────────────────────────────────────────────────────────────

// todoPageData holds all data passed to index.html
type todoPageData struct {
	Todos    []service.Todo
	Total    int
	Done     int
	UserName string
}

// Index renders GET / — the main todo dashboard for the logged-in user.
func Index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	userID, userName := middleware.UserFromCtx(r)

	todos, err := service.ListTodos(userID)
	if err != nil {
		http.Error(w, "Failed to fetch todos", http.StatusInternalServerError)
		return
	}

	done := service.CountDone(todos)

	renderPage(w, "index.html", todoPageData{
		Todos:    todos,
		Total:    len(todos),
		Done:     done,
		UserName: userName,
	})
}

// ── Form handlers (POST → redirect) ──────────────────────────────────────
// These handle HTML form submissions. For JSON API, use /api/* routes.

// HandleSignup processes POST /signup from the HTML form.
func HandleSignup(w http.ResponseWriter, r *http.Request) {
	session, err := service.Signup(service.SignupInput{
		Name:     r.FormValue("name"),
		Email:    r.FormValue("email"),
		Password: r.FormValue("password"),
		Confirm:  r.FormValue("confirm"),
	})

	if err != nil {
		renderPage(w, "signup.html", map[string]string{"Error": err.Error()})
		return
	}

	setSessionCookie(w, session)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// HandleSignin processes POST /signin from the HTML form.
func HandleSignin(w http.ResponseWriter, r *http.Request) {
	session, err := service.Signin(service.SigninInput{
		Email:    r.FormValue("email"),
		Password: r.FormValue("password"),
	})

	if err != nil {
		renderPage(w, "signin.html", map[string]string{"Error": err.Error()})
		return
	}

	setSessionCookie(w, session)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// HandleSignout processes POST /signout from the HTML form.
func HandleSignout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(middleware.SessionCookie)
	if err == nil {
		service.Signout(cookie.Value)
	}
	middleware.ClearCookie(w)
	http.Redirect(w, r, "/signin", http.StatusSeeOther)
}

// HandleAddTodo processes POST /todo/add from the HTML form.
func HandleAddTodo(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserFromCtx(r)
	service.AddTodo(service.AddTodoInput{
		UserID:      userID,
		Subject:     r.FormValue("subject"),
		Description: r.FormValue("description"),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// HandleToggleTodo processes POST /todo/toggle from the HTML form.
func HandleToggleTodo(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserFromCtx(r)
	id := 0
	if v := r.FormValue("id"); v != "" {
		for _, c := range v {
			if c >= '0' && c <= '9' {
				id = id*10 + int(c-'0')
			} else {
				id = 0
				break
			}
		}
	}
	if id > 0 {
		service.ToggleTodo(id, userID)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// HandleDeleteTodo processes POST /todo/delete from the HTML form.
func HandleDeleteTodo(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserFromCtx(r)
	id := 0
	if v := r.FormValue("id"); v != "" {
		for _, c := range v {
			if c >= '0' && c <= '9' {
				id = id*10 + int(c-'0')
			} else {
				id = 0
				break
			}
		}
	}
	if id > 0 {
		service.DeleteTodo(id, userID)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ── Helpers ───────────────────────────────────────────────────────────────

func renderPage(w http.ResponseWriter, tmpl string, data interface{}) {
	if err := templates.ExecuteTemplate(w, tmpl, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

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
