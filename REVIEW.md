# Tickasis TO-DO — Project Review

**Author:** Kiro AI  
**Project:** Tickasis TO-DO — A Go-based todo web application  
**Last Updated:** November 2026

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Evolution — How It Was Built](#2-evolution--how-it-was-built)
3. [Final Architecture](#3-final-architecture)
4. [Project Structure](#4-project-structure)
5. [Service Details](#5-service-details)
6. [Database Schema](#6-database-schema)
7. [API Reference](#7-api-reference)
8. [Security Implementation](#8-security-implementation)
9. [Docker Setup](#9-docker-setup)
10. [Kubernetes Setup](#10-kubernetes-setup)
11. [Environment Variables](#11-environment-variables)
12. [How to Run](#12-how-to-run)
13. [How to Deploy on Kubernetes](#13-how-to-deploy-on-kubernetes)
14. [Troubleshooting](#14-troubleshooting)

---

## 1. Project Overview

Tickasis TO-DO is a full-stack web application built entirely in **Go** with no frontend JavaScript frameworks. It allows multiple users to manage their personal todo lists with full isolation — Amit's todos are never visible to Sahil and vice versa.

**Tech stack:**

| Layer | Technology |
|-------|-----------|
| Language | Go 1.22 |
| Templates | `html/template` (standard library) |
| HTTP server | `net/http` (standard library) |
| Database | PostgreSQL 16 |
| Auth | bcrypt passwords + UUID session tokens |
| Container | Docker (multi-stage, alpine-based) |
| Orchestration | Kubernetes (kind for local, any k8s for production) |

---

## 2. Evolution — How It Was Built

The project was built in **6 phases**, each adding to the previous.

### Phase 1 — Basic Todo App (Monolith)

**What was built:**
- Single Go binary (`main.go`)
- In-memory todo store (`[]Todo` slice with `sync.Mutex`)
- HTML template with dark gradient UI
- Add, toggle done, delete todos
- Stats bar (total / done / pending)

**Key files created:**
```
main.go
handler/todo.go       ← in-memory store + CRUD handlers
templates/index.html  ← dark gradient UI
go.mod
```

**Problem with this phase:** All todos were lost on restart — stored only in RAM.

---

### Phase 2 — JSON File Persistence

**What was added:**
- Todos saved to `/data/todos.json` on every change
- Atomic writes (write to `.tmp` file then rename) to prevent corruption
- On startup, `init()` loads the file back into memory
- Docker volume mounted at `/data` so file survives container restarts

**Key change in `handler/todo.go`:**
```go
func save() {
    data, _ := json.MarshalIndent(todos, "", "  ")
    tmp := dataFile + ".tmp"
    os.WriteFile(tmp, data, 0644)
    os.Rename(tmp, dataFile)  // atomic
}
```

**Problem with this phase:** JSON file doesn't scale, can't handle concurrent writes from multiple instances.

---

### Phase 3 — PostgreSQL Database

**What was added:**
- Replaced JSON file with PostgreSQL using `pgx/v5` driver
- `db/db.go` — connection pool with 10-retry startup loop
- Schema auto-created on first run
- `DATABASE_URL` environment variable for connection string
- Docker Compose with postgres:16-alpine + named volume

**Why pgx instead of database/sql:**
- Native PostgreSQL driver (faster)
- Better error handling
- Connection pooling built in (`pgxpool`)

**Dockerfile change:**
- Switched final image from `scratch` to `alpine:3.19` to have `wget` for healthchecks and a shell for debugging

---

### Phase 4 — User Authentication

**What was added:**
- Signup, Signin, Signout pages
- `users` table — stores name, email, bcrypt password hash
- `sessions` table — maps UUID token → user
- Session cookie (`golive_session`) — HttpOnly, SameSite=Lax
- `todos` table got `user_id` foreign key
- Every todo query filters by `user_id` — complete data isolation
- **Application renamed from "GoLive Todo" to "Tickasis TO-DO"**

**Security measures implemented:**

| Threat | Defence |
|--------|---------|
| Password theft | bcrypt cost=12 (never store plain text) |
| Session hijacking | HttpOnly cookie (JS can't read it) |
| CSRF attacks | SameSite=Lax cookie |
| Timing attacks | Dummy bcrypt call when user not found |
| Cross-user data access | `WHERE user_id = $1` on every query |
| Cross-user tampering | `WHERE id=$1 AND user_id=$2` on update/delete |

**New tables:**
```sql
CREATE TABLE users (
    id            SERIAL PRIMARY KEY,
    name          TEXT NOT NULL,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE sessions (
    token      TEXT PRIMARY KEY,
    user_id    INT REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ DEFAULT NOW() + INTERVAL '7 days'
);
```

**Task form also updated:**
- Added `subject` field (required, short title)
- Added `description` field (optional, multi-line textarea)

---

### Phase 5 — Separated API Layer (Single Service)

**What was refactored:**
The monolith was restructured into clean layers within the same binary:

```
Before:                     After:
auth/auth.go (everything) → middleware/auth.go  (session validation only)
handler/todo.go           → service/auth.go     (auth business logic, no HTTP)
                          → service/todo.go     (todo business logic, no HTTP)
                          → api/auth.go         (JSON REST API handlers)
                          → api/todo.go         (JSON REST API handlers)
                          → handler/pages.go    (HTML page rendering only)
```

**New JSON REST API endpoints added:**

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/auth/signup` | Create account (JSON) |
| POST | `/api/auth/signin` | Sign in (JSON) |
| POST | `/api/auth/signout` | Sign out (JSON) |
| GET | `/api/auth/me` | Current user (JSON) |
| GET | `/api/todos` | List todos + stats (JSON) |
| POST | `/api/todos` | Create todo (JSON) |
| PATCH | `/api/todos/{id}/toggle` | Toggle done (JSON) |
| DELETE | `/api/todos/{id}` | Delete todo (JSON) |

**Why this matters:** Web UI and API coexist in the same binary. JMeter, mobile apps, or any HTTP client can use the JSON API while the browser uses the HTML form flow.

---

### Phase 6 — Microservices Architecture

**Why:** Each service is now an independent Go program with its own port, its own responsibility, and its own deployment.

**Services created:**

| Service | Port | Responsibility |
|---------|------|---------------|
| `frontend` | 8080 | Serves HTML pages, proxies to auth + todo services |
| `auth-service` | 8081 | Signup, signin, signout, session validation |
| `todo-service` | 8082 | List, add, toggle, delete todos |
| `postgres` | 5432 | Database (internal only) |

**Traffic flow:**
```
Browser
  │
  ▼
frontend :8080  ← ONLY public-facing service
  │
  ├── POST /signup form → calls auth-service:8081/signup
  ├── POST /signin form → calls auth-service:8081/signin
  ├── GET /            → calls auth-service:8081/validate (session check)
  │                       then calls todo-service:8082/todos
  └── POST /todo/add   → calls todo-service:8082/todos
                              │
                         (with X-User-ID, X-User-Name headers)
```

**How auth works between services:**
1. Browser sends `golive_session` cookie to frontend
2. Frontend calls `auth-service/validate?token=<cookie>` on every protected request
3. auth-service returns `{user_id, user_name}` if valid
4. Frontend sets `X-User-ID` and `X-User-Name` headers when calling todo-service
5. todo-service trusts these headers (only frontend can call it — not exposed externally)

**Each service has its own:**
- `main.go`
- `go.mod` + `go.sum`
- `Dockerfile`
- Database schema creation (idempotent — `CREATE TABLE IF NOT EXISTS`)

---

## 3. Final Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Browser / JMeter                      │
└──────────────────────┬──────────────────────────────────┘
                       │ HTTP port 8080 (only public port)
                       ▼
          ┌────────────────────────┐
          │   frontend-service     │
          │   port 8080            │
          │                        │
          │  - Serves HTML pages   │
          │  - Validates sessions  │
          │  - Proxies API calls   │
          └──────┬─────────┬───────┘
                 │         │
    internal     │         │    internal
    HTTP call    │         │    HTTP call
                 ▼         ▼
    ┌────────────────┐  ┌─────────────────┐
    │ auth-service   │  │  todo-service   │
    │ port 8081      │  │  port 8082      │
    │                │  │                 │
    │ /signup        │  │ GET  /todos     │
    │ /signin        │  │ POST /todos     │
    │ /signout       │  │ PATCH /todos/id │
    │ /validate      │  │ DELETE /todos/id│
    │ /me            │  │                 │
    └───────┬────────┘  └────────┬────────┘
            │                    │
            └─────────┬──────────┘
                      ▼
             ┌─────────────────┐
             │   PostgreSQL    │
             │   port 5432     │
             │                 │
             │ users           │
             │ sessions        │
             │ todos           │
             └─────────────────┘
```

---

## 4. Project Structure

```
tasksis-todo-golang/
│
├── frontend/                    ← Standalone service (port 8080)
│   ├── main.go                  ← HTTP handlers + proxy logic
│   ├── go.mod
│   ├── Dockerfile
│   ├── templates/               ← HTML templates (signin, signup, index)
│   └── static/                  ← Static assets
│
├── auth-service/                ← Standalone service (port 8081)
│   ├── main.go                  ← signup, signin, signout, validate, me
│   ├── db/db.go                 ← PostgreSQL connection + users/sessions schema
│   ├── go.mod
│   └── Dockerfile
│
├── todo-service/                ← Standalone service (port 8082)
│   ├── main.go                  ← list, add, toggle, delete todos
│   ├── db/db.go                 ← PostgreSQL connection + todos schema
│   ├── go.mod
│   └── Dockerfile
│
├── k8s/                         ← Kubernetes manifests
│   ├── microservices.yml        ← All 3 deployments + services + postgres + PVC
│   ├── secret.yml               ← Credentials (fill before applying)
│   ├── golang-deployment.yml    ← Old single-service deployment (legacy)
│   ├── postgrey-deployment.yml  ← Old postgres deployment (legacy)
│   └── services.yml             ← Old services (legacy)
│
├── docker-compose.yml           ← Runs all 4 containers locally
├── .env                         ← Real credentials (never commit — in .gitignore)
├── .env.example                 ← Template for .env (safe to commit)
├── .gitignore
│
├── [Legacy single-service files — kept for reference]
├── main.go
├── go.mod / go.sum
├── Dockerfile
├── db/db.go
├── api/auth.go + todo.go
├── service/auth.go + todo.go
├── middleware/auth.go
├── handler/pages.go
└── templates/
```

---

## 5. Service Details

### auth-service (port 8081)

**Endpoints:**

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | ❌ | Liveness check |
| POST | `/signup` | ❌ | Create account, returns session cookie |
| POST | `/signin` | ❌ | Login, returns session cookie |
| POST | `/signout` | ❌ | Delete session, clears cookie |
| GET | `/me` | ✅ cookie | Current user info |
| GET | `/validate?token=xxx` | ❌ | Internal — validates token, returns user info |

**Security:**
- Passwords hashed with bcrypt cost=12
- Timing-safe login (dummy bcrypt call when user not found)
- Session tokens are UUID v4 (random, unguessable)
- Sessions expire after 7 days

---

### todo-service (port 8082)

**Endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness check |
| GET | `/todos` | List user's todos + stats |
| POST | `/todos` | Create new todo |
| PATCH | `/todos/{id}/toggle` | Toggle done/undone |
| DELETE | `/todos/{id}` | Delete todo |

**Authentication:**
- All endpoints require `X-User-ID` and `X-User-Name` headers
- These are set by frontend after validating with auth-service
- Direct calls without headers return `401 Unauthorized`
- All queries include `WHERE user_id = $1` — strict data isolation

**Request/Response examples:**

```bash
# Add todo
POST /todos
Headers: X-User-ID: 1, X-User-Name: Amit Rana
Body: {"subject": "Learn Go", "description": "Read the docs"}

# Response 201
{"id":1,"subject":"Learn Go","description":"Read the docs","done":false,"created_at":"..."}

# List todos
GET /todos
Headers: X-User-ID: 1, X-User-Name: Amit Rana

# Response 200
{"todos":[...],"total":3,"done":1,"pending":2}
```

---

### frontend (port 8080)

**Pages:**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/signup` | Show signup form |
| POST | `/signup` | Submit signup → calls auth-service |
| GET | `/signin` | Show signin form |
| POST | `/signin` | Submit signin → calls auth-service |
| POST | `/signout` | Sign out → calls auth-service, clears cookie |
| GET | `/` | Todo dashboard (protected) |
| POST | `/todo/add` | Add task (protected) |
| POST | `/todo/toggle` | Toggle task (protected) |
| POST | `/todo/delete` | Delete task (protected) |
| GET | `/health` | Liveness check |

**How session validation works in frontend:**
```go
func requireSession(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        cookie, _ := r.Cookie("golive_session")

        // Ask auth-service if this token is valid
        resp, _ := http.Get(authServiceURL + "/validate?token=" + cookie.Value)

        if resp.StatusCode != 200 {
            http.Redirect(w, r, "/signin", ...)
            return
        }

        // Inject user into context, call handler
        json.Decode(resp.Body, &user)
        ctx := context.WithValue(r.Context(), ctxUser, user)
        next(w, r.WithContext(ctx))
    }
}
```

---

## 6. Database Schema

All three services share the same PostgreSQL database. Each service creates only its own tables on startup.

```sql
-- Created by auth-service
CREATE TABLE users (
    id            SERIAL PRIMARY KEY,
    name          TEXT        NOT NULL,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE sessions (
    token      TEXT PRIMARY KEY,
    user_id    INT  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days'
);

CREATE INDEX idx_sessions_token ON sessions(token);

-- Created by todo-service
CREATE TABLE todos (
    id          SERIAL  PRIMARY KEY,
    user_id     INT     NOT NULL,   -- references users.id (no FK across services)
    subject     TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    done        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_todos_user_id ON todos(user_id);
```

> **Note:** `todos.user_id` intentionally has no foreign key to `users` — each microservice owns its own schema. The relationship is enforced at the application level.

---

## 7. API Reference

### Auth Service APIs

```bash
# Signup
curl -X POST http://localhost:8081/signup \
  -H "Content-Type: application/json" \
  -d '{"name":"Amit Rana","email":"amit@example.com","password":"pass1234","confirm":"pass1234"}'

# Response 201
{"message":"account created","user_id":1,"user_name":"Amit Rana"}
# Also sets cookie: golive_session=<uuid>

# Signin
curl -X POST http://localhost:8081/signin \
  -H "Content-Type: application/json" \
  -d '{"email":"amit@example.com","password":"pass1234"}' \
  -c cookies.txt

# Response 200
{"message":"signed in","user_id":1,"user_name":"Amit Rana"}

# Validate session (used internally by frontend)
curl "http://localhost:8081/validate?token=<session-token>"

# Response 200
{"user_id":1,"user_name":"Amit Rana"}

# Signout
curl -X POST http://localhost:8081/signout -b cookies.txt
# Response 200
{"message":"signed out"}
```

### Todo Service APIs

```bash
# List todos
curl http://localhost:8082/todos \
  -H "X-User-ID: 1" -H "X-User-Name: Amit Rana"

# Add todo
curl -X POST http://localhost:8082/todos \
  -H "Content-Type: application/json" \
  -H "X-User-ID: 1" -H "X-User-Name: Amit Rana" \
  -d '{"subject":"Deploy to k8s","description":"Push images and apply manifests"}'

# Toggle done
curl -X PATCH http://localhost:8082/todos/1/toggle \
  -H "X-User-ID: 1" -H "X-User-Name: Amit Rana"

# Delete
curl -X DELETE http://localhost:8082/todos/1 \
  -H "X-User-ID: 1" -H "X-User-Name: Amit Rana"
```

---

## 8. Security Implementation

### Password Security
- bcrypt with cost factor 12 (computationally expensive — slows brute force)
- Never stored in plain text anywhere
- Never logged

### Session Security
- UUID v4 tokens (128-bit random — practically unguessable)
- `HttpOnly` flag — JavaScript cannot read the cookie
- `SameSite=Lax` — prevents CSRF attacks from other domains
- 7-day expiry stored in database
- Deleted from DB on signout (not just cleared from browser)

### Data Isolation
- Every `SELECT`, `UPDATE`, `DELETE` query includes `WHERE user_id = $1`
- Update and delete also check `user_id` to prevent one user modifying another's data:
  ```sql
  DELETE FROM todos WHERE id = $1 AND user_id = $2
  ```

### Network Security (Kubernetes)
- Only `frontend` service is exposed externally (NodePort 30080)
- `auth-service` and `todo-service` are ClusterIP — no external access
- `postgres` is ClusterIP — no external access
- Internal service-to-service calls use k8s DNS (`http://auth-service:8081`)

### Timing Attack Prevention
When a user tries to sign in with a non-existent email:
```go
if err != nil {
    // Run dummy bcrypt even if user not found
    // This makes "user not found" take same time as "wrong password"
    // So attackers can't enumerate valid emails by measuring response time
    bcrypt.CompareHashAndPassword([]byte("$2a$12$dummy"), []byte(password))
    return ErrInvalidCreds
}
```

---

## 9. Docker Setup

### docker-compose.yml structure

```yaml
services:
  postgres        ← starts first, has healthcheck
  auth-service    ← starts after postgres is healthy, port 8081
  todo-service    ← starts after postgres is healthy, port 8082
  frontend        ← starts after auth + todo are healthy, port 8080
```

### Startup order (enforced by `depends_on` + healthchecks)

```
postgres (healthy)
    ↓
auth-service (healthy) + todo-service (healthy)
    ↓
frontend (healthy)
```

### Build each image separately

```bash
docker build -t artech0203/auth-service:latest ./auth-service
docker build -t artech0203/todo-service:latest ./todo-service
docker build -t artech0203/frontend:latest ./frontend
```

### Run everything

```bash
docker compose up --build      # build + start, attached
docker compose up --build -d   # build + start, background
docker compose down            # stop all
docker compose logs -f         # stream all logs
docker compose logs frontend   # logs for one service
```

---

## 10. Kubernetes Setup

### Files

| File | Purpose |
|------|---------|
| `k8s/secret.yml` | Database credentials (fill in before applying) |
| `k8s/microservices.yml` | All deployments, services, PVC, namespace |

### What microservices.yml creates

```
Namespace: tasksis-todo
  ├── PersistentVolumeClaim: postgres-pvc (1Gi)
  ├── Deployment: postgres
  ├── Service: postgres (ClusterIP :5432)
  ├── Deployment: auth-service
  ├── Service: auth-service (ClusterIP :8081)
  ├── Deployment: todo-service
  ├── Service: todo-service (ClusterIP :8082)
  ├── Deployment: frontend
  └── Service: frontend (NodePort :30080 → pod :8080)
```

### Health checks (liveness + readiness probes)

All three services expose `GET /health` returning:
```json
{"status":"ok","service":"auth-service"}
```

k8s uses this to:
- **readinessProbe** — only send traffic when app is ready
- **livenessProbe** — restart pod if app becomes unresponsive

### Images expected

```
artech0203/auth-service:latest
artech0203/todo-service:latest
artech0203/frontend:latest
postgres:16-alpine              ← official image
```

---

## 11. Environment Variables

### .env (local docker-compose)

```bash
POSTGRES_DB=golive
POSTGRES_USER=golive
POSTGRES_PASSWORD=golive_secret
DATABASE_URL=postgres://golive:golive_secret@postgres:5432/golive?sslmode=disable
APP_ENV=production
APP_PORT=8080
```

### Per-service environment variables

**auth-service + todo-service:**
```
DATABASE_URL  ← full postgres connection string
PORT          ← defaults to 8081 / 8082
```

**frontend:**
```
AUTH_SERVICE_URL  ← e.g. http://auth-service:8081
TODO_SERVICE_URL  ← e.g. http://todo-service:8082
PORT              ← defaults to 8080
```

> In k8s, `AUTH_SERVICE_URL` and `TODO_SERVICE_URL` resolve via k8s internal DNS automatically because the Service names match.

---

## 12. How to Run

### Option 1 — Docker Compose (recommended for local dev)

```bash
# 1. Clone the repo
git clone <your-repo-url>
cd tasksis-todo-golang

# 2. Create .env from template
cp .env.example .env
# Edit .env with your values if needed

# 3. Start everything
docker compose up --build -d

# 4. Open the app
# http://localhost:8080

# 5. Verify services
docker compose ps
curl http://localhost:8081/health   # auth-service
curl http://localhost:8082/health   # todo-service
curl http://localhost:8080/health   # frontend

# 6. Stop
docker compose down
```

### Option 2 — Go directly (requires Go 1.22+)

Run each service in a separate terminal:

```bash
# Terminal 1 — auth-service
cd auth-service
DATABASE_URL="postgres://..." PORT=8081 go run .

# Terminal 2 — todo-service
cd todo-service
DATABASE_URL="postgres://..." PORT=8082 go run .

# Terminal 3 — frontend
cd frontend
AUTH_SERVICE_URL=http://localhost:8081 TODO_SERVICE_URL=http://localhost:8082 PORT=8080 go run .
```

---

## 13. How to Deploy on Kubernetes

```bash
# Step 1 — Build images
docker build -t artech0203/auth-service:latest ./auth-service
docker build -t artech0203/todo-service:latest ./todo-service
docker build -t artech0203/frontend:latest ./frontend

# Step 2 — Push to Docker Hub
docker push artech0203/auth-service:latest
docker push artech0203/todo-service:latest
docker push artech0203/frontend:latest

# Step 3 — (kind only) Load images into cluster
docker save artech0203/auth-service:latest | \
  docker exec -i tasksis-control-plane ctr --namespace=k8s.io images import -
docker save artech0203/todo-service:latest | \
  docker exec -i tasksis-control-plane ctr --namespace=k8s.io images import -
docker save artech0203/frontend:latest | \
  docker exec -i tasksis-control-plane ctr --namespace=k8s.io images import -

# Step 4 — Edit secret.yml with your credentials, then apply
kubectl apply -f k8s/secret.yml

# Step 5 — Deploy everything
kubectl apply -f k8s/microservices.yml

# Step 6 — Check status
kubectl get pods -n tasksis-todo
kubectl get svc -n tasksis-todo

# Step 7 — Access the app
# Get node IP
kubectl get node -o wide
# Open: http://<node-ip>:30080

# Useful commands
kubectl logs -f deployment/frontend -n tasksis-todo
kubectl logs -f deployment/auth-service -n tasksis-todo
kubectl logs -f deployment/todo-service -n tasksis-todo
kubectl describe pod <pod-name> -n tasksis-todo
```

---

## 14. Troubleshooting

### Container is unhealthy

```bash
docker inspect <container-name> --format='{{json .State.Health}}'
docker logs <container-name>
```

Common causes:
- `no such file or directory: /bin/sh` — image is `scratch`, healthcheck uses `CMD-SHELL`. Fix: use `CMD` with explicit binary
- Port already allocated — another container is using the port. Fix: `docker ps` to find it, stop it

### ImagePullBackOff in k8s

```bash
kubectl describe pod <pod-name> -n tasksis-todo
```

Common causes:
- Image name has no Docker Hub username prefix (e.g. `tasksis-todo` instead of `artech0203/tasksis-todo`)
- Image not pushed to Docker Hub
- For kind clusters: image not loaded with `kind load` or `docker exec ctr import`

### App can't connect to database

```bash
kubectl logs deployment/auth-service -n tasksis-todo
# Look for: "waiting for database... attempt X/10"
```

Common causes:
- `DATABASE_URL` not set or wrong
- postgres pod not ready yet (app retries 10 times, 3s apart — usually resolves itself)
- Wrong hostname in `DATABASE_URL` (must match the k8s Service name)

### Frontend shows blank page after login

Check browser console for template errors. Usually means a template function (`sub`, `slice`, etc.) is not registered in the FuncMap.

### Session not working across services

The `golive_session` cookie must be set with `Path=/` so it's sent on all requests. Check:
```go
http.SetCookie(w, &http.Cookie{
    Name:  "golive_session",
    Path:  "/",        // ← must be "/"
    ...
})
```

---

*This document was generated based on the complete development history of the Tickasis TO-DO project.*
