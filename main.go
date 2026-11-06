package main

import (
	"log"
	"net/http"

	"github.com/amitrana/golive/api"
	"github.com/amitrana/golive/db"
	"github.com/amitrana/golive/handler"
	"github.com/amitrana/golive/middleware"
)

func main() {
	// Connect to PostgreSQL and initialise schema
	db.Connect()

	mux := http.NewServeMux()

	// ── Static files ──────────────────────────────────────────────────────
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// ── HTML page routes (browser, form-based) ────────────────────────────
	//
	//  GET  /signup          → show signup form
	//  POST /signup          → process signup form
	//  GET  /signin          → show signin form
	//  POST /signin          → process signin form
	//  POST /signout         → sign out and redirect
	//  GET  /                → todo dashboard (protected)
	//  POST /todo/add        → add task via form (protected)
	//  POST /todo/toggle     → toggle task via form (protected)
	//  POST /todo/delete     → delete task via form (protected)

	mux.HandleFunc("/signup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleSignup(w, r)
		} else {
			handler.SignupPage(w, r)
		}
	})

	mux.HandleFunc("/signin", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleSignin(w, r)
		} else {
			handler.SigninPage(w, r)
		}
	})

	mux.HandleFunc("/signout", handler.HandleSignout)

	mux.HandleFunc("/", middleware.RequireAuth(handler.Index))
	mux.HandleFunc("/todo/add", middleware.RequireAuth(handler.HandleAddTodo))
	mux.HandleFunc("/todo/toggle", middleware.RequireAuth(handler.HandleToggleTodo))
	mux.HandleFunc("/todo/delete", middleware.RequireAuth(handler.HandleDeleteTodo))

	// ── JSON REST API routes (/api/...) ───────────────────────────────────
	//
	//  Auth (public):
	//    POST /api/auth/signup          → create account, returns session cookie + JSON
	//    POST /api/auth/signin          → sign in, returns session cookie + JSON
	//    POST /api/auth/signout         → sign out, clears session cookie
	//    GET  /api/auth/me              → current user info (protected)
	//
	//  Todos (all protected — require valid session cookie):
	//    GET    /api/todos              → list user's todos + stats
	//    POST   /api/todos              → create a new todo
	//    PATCH  /api/todos/{id}/toggle  → toggle done status
	//    DELETE /api/todos/{id}         → delete a todo

	mux.HandleFunc("/api/auth/signup", api.Signup)
	mux.HandleFunc("/api/auth/signin", api.Signin)
	mux.HandleFunc("/api/auth/signout", api.Signout)
	mux.HandleFunc("/api/auth/me", middleware.RequireAuth(api.Me))

	mux.HandleFunc("/api/todos", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			middleware.RequireAuth(api.ListTodos)(w, r)
		case http.MethodPost:
			middleware.RequireAuth(api.AddTodo)(w, r)
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte(`{"error":"method_not_allowed","message":"use GET or POST"}`))
		}
	})

	// /api/todos/{id}/toggle  and  /api/todos/{id}
	mux.HandleFunc("/api/todos/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch:
			middleware.RequireAuth(api.ToggleTodo)(w, r)
		case http.MethodDelete:
			middleware.RequireAuth(api.DeleteTodo)(w, r)
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte(`{"error":"method_not_allowed","message":"use PATCH or DELETE"}`))
		}
	})

	log.Println("Tickasis TO-DO server started on :8080")
	log.Println("  Web UI  →  http://localhost:8080")
	log.Println("  API     →  http://localhost:8080/api/")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
