# Tickasis TO-DO

A multi-user todo web application built with **Go** (no frontend frameworks). Each user sees only their own tasks. Built as three independent microservices backed by PostgreSQL.

## Tech Stack

| | |
|---|---|
| Language | Go 1.22 |
| Database | PostgreSQL 16 |
| Auth | bcrypt passwords · UUID session tokens · HttpOnly cookies |
| Containers | Docker (multi-stage, alpine-based images) |
| Orchestration | Kubernetes · Helm |
| Secrets | AWS Secrets Manager · External Secrets Operator |
| Ingress | NGINX Ingress Controller |

---

## Architecture

Only the frontend is exposed publicly. Auth and todo services are internal.

```
Browser
  └── frontend :8080  (public)
        ├── auth-service :8081  (internal)
        └── todo-service :8082  (internal)
              └── PostgreSQL :5432  (internal)
```

**Session flow:** Browser sends a `golive_session` cookie → frontend validates it with `auth-service /validate` → frontend forwards `X-User-ID` / `X-User-Name` headers to `todo-service` → todo-service scopes all queries to that user.

---

## Services

### frontend (port 8080)

Serves the HTML UI and proxies form submissions to the internal services.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET/POST | `/signup` | — | Sign up page / submit |
| GET/POST | `/signin` | — | Sign in page / submit |
| POST | `/signout` | — | Sign out |
| GET | `/` | ✅ | Todo dashboard |
| POST | `/todo/add` | ✅ | Add task |
| POST | `/todo/toggle` | ✅ | Toggle task done |
| POST | `/todo/delete` | ✅ | Delete task |
| GET | `/health` | — | Health check |

---

### auth-service (port 8081)

Manages user accounts and sessions. All responses are JSON.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/signup` | — | Create account · returns session cookie |
| POST | `/signin` | — | Sign in · returns session cookie |
| POST | `/signout` | — | Delete session · clears cookie |
| GET | `/me` | ✅ cookie | Current user info |
| GET | `/validate?token=` | — | Internal — validate session token |
| GET | `/health` | — | Health check |

```bash
# Signup
curl -X POST http://localhost:8081/signup \
  -H "Content-Type: application/json" \
  -d '{"name":"Amit Rana","email":"amit@example.com","password":"pass1234","confirm":"pass1234"}'
# → {"message":"account created","user_id":1,"user_name":"Amit Rana"}

# Signin
curl -X POST http://localhost:8081/signin \
  -H "Content-Type: application/json" \
  -d '{"email":"amit@example.com","password":"pass1234"}' -c cookies.txt
# → {"message":"signed in","user_id":1,"user_name":"Amit Rana"}
```

---

### todo-service (port 8082)

Manages todos. All endpoints require `X-User-ID` and `X-User-Name` headers (set by frontend after session validation). All responses are JSON.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/todos` | List todos + stats |
| POST | `/todos` | Create todo |
| PATCH | `/todos/{id}/toggle` | Toggle done/undone |
| DELETE | `/todos/{id}` | Delete todo |
| GET | `/health` | Health check |

```bash
# List todos
curl http://localhost:8082/todos \
  -H "X-User-ID: 1" -H "X-User-Name: Amit Rana"
# → {"todos":[...],"total":2,"done":1,"pending":1}

# Add todo
curl -X POST http://localhost:8082/todos \
  -H "Content-Type: application/json" \
  -H "X-User-ID: 1" -H "X-User-Name: Amit Rana" \
  -d '{"subject":"Deploy to k8s","description":"Push images and apply manifests"}'
# → {"id":1,"subject":"Deploy to k8s","description":"...","done":false,"created_at":"..."}

# Toggle done
curl -X PATCH http://localhost:8082/todos/1/toggle \
  -H "X-User-ID: 1" -H "X-User-Name: Amit Rana"

# Delete
curl -X DELETE http://localhost:8082/todos/1 \
  -H "X-User-ID: 1" -H "X-User-Name: Amit Rana"
```

Calls without required headers return `401 Unauthorized`.

---

## Project Structure

```
tickasis-todo-golang/
├── frontend/                  # Web UI service (port 8080)
│   ├── main.go
│   ├── Dockerfile
│   ├── go.mod
│   └── templates/             # signin.html, signup.html, index.html
├── auth-service/              # Auth service (port 8081)
│   ├── main.go
│   ├── db/db.go               # Schema: users, sessions
│   ├── Dockerfile
│   └── go.mod
├── todo-service/              # Todo service (port 8082)
│   ├── main.go
│   ├── db/db.go               # Schema: todos
│   ├── Dockerfile
│   └── go.mod
├── helm/
│   └── tickasis/              # Helm chart
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/         # Deployments, Services, Ingress, PVC, ExternalSecret
├── k8s/
│   ├── cluster-secret-store.yaml  # ClusterSecretStore for ESO
│   └── microservices.yml          # Legacy raw manifests (reference only)
├── kind-cluster.yaml          # Kind cluster config (3-node)
├── docker-compose.yml
├── .env                       # Local credentials (git-ignored)
└── .env.example               # Credentials template
```

---

## Database Schema

```sql
-- auth-service owns these tables
CREATE TABLE users (
    id            SERIAL PRIMARY KEY,
    name          TEXT NOT NULL,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ DEFAULT NOW()
);
CREATE TABLE sessions (
    token      TEXT PRIMARY KEY,
    user_id    INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ DEFAULT NOW() + INTERVAL '7 days'
);

-- todo-service owns this table
CREATE TABLE todos (
    id          SERIAL PRIMARY KEY,
    user_id     INT NOT NULL,   -- scoped per user, no cross-service FK
    subject     TEXT NOT NULL,
    description TEXT DEFAULT '',
    done        BOOLEAN DEFAULT FALSE,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);
```

All tables are created automatically on first startup (`CREATE TABLE IF NOT EXISTS`).

---

## Environment Variables

Copy `.env.example` to `.env` and fill in your values. Never commit `.env`.

```bash
cp .env.example .env
```

| Variable | Used by | Description |
|----------|---------|-------------|
| `POSTGRES_DB` | postgres, auth, todo | Database name |
| `POSTGRES_USER` | postgres, auth, todo | Database user |
| `POSTGRES_PASSWORD` | postgres, auth, todo | Database password |
| `DATABASE_URL` | auth-service, todo-service | Full connection string |
| `AUTH_SERVICE_URL` | frontend | e.g. `http://auth-service:8081` |
| `TODO_SERVICE_URL` | frontend | e.g. `http://todo-service:8082` |
| `PORT` | all services | Listening port (has sensible defaults) |

In Kubernetes, `DATABASE_URL` and database credentials are managed through AWS Secrets Manager and synced into the cluster via External Secrets Operator. `AUTH_SERVICE_URL` and `TODO_SERVICE_URL` are set in `helm/tickasis/values.yaml`.

---

## Running Locally (Docker Compose)

```bash
git clone <repo-url>
cd tickasis-todo-golang
cp .env.example .env          # edit credentials if needed

docker compose up --build -d  # build and start all 4 containers
```

Open **http://localhost:8080**

```bash
docker compose ps             # check status
docker compose logs -f        # stream logs
docker compose down           # stop everything
```

Startup order is enforced: postgres → auth-service + todo-service → frontend.

---

## Deploying on Kubernetes (local Kind + Helm)

> For full setup details see `REVIEW-DEPLOY.md`.

### Prerequisites

- Kind, kubectl, Helm, AWS CLI installed
- Docker images pushed to Amazon ECR
- AWS Secrets Manager secret `tickasis-app` created with `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `DATABASE_URL`

### 1. Create the Kind cluster

```bash
kind create cluster --config kind-cluster.yaml --name tickasis-todo
kubectl create namespace tickasis
```

### 2. Push images to ECR and load into cluster

```bash
# Authenticate to ECR
aws ecr get-login-password --region us-east-1 \
  | docker login --username AWS --password-stdin \
    <AWS_ACCOUNT_ID>.dkr.ecr.us-east-1.amazonaws.com

# Build and push
docker build -t <AWS_ACCOUNT_ID>.dkr.ecr.us-east-1.amazonaws.com/tickasis-todo-frontend:latest ./frontend
docker build -t <AWS_ACCOUNT_ID>.dkr.ecr.us-east-1.amazonaws.com/tickasis-todo-auth:latest     ./auth-service
docker build -t <AWS_ACCOUNT_ID>.dkr.ecr.us-east-1.amazonaws.com/tickasis-todo-app:latest      ./todo-service
docker push <AWS_ACCOUNT_ID>.dkr.ecr.us-east-1.amazonaws.com/tickasis-todo-frontend:latest
docker push <AWS_ACCOUNT_ID>.dkr.ecr.us-east-1.amazonaws.com/tickasis-todo-auth:latest
docker push <AWS_ACCOUNT_ID>.dkr.ecr.us-east-1.amazonaws.com/tickasis-todo-app:latest
```

### 3. Create ECR image pull secret

```bash
kubectl create secret docker-registry ecr-registry-secret \
  --namespace tickasis \
  --docker-server=<AWS_ACCOUNT_ID>.dkr.ecr.us-east-1.amazonaws.com \
  --docker-username=AWS \
  --docker-password="$(aws ecr get-login-password --region us-east-1)" \
  --dry-run=client -o yaml | kubectl apply -f -
```

### 4. Install External Secrets Operator and configure AWS access

```bash
helm repo add external-secrets https://charts.external-secrets.io
helm install external-secrets external-secrets/external-secrets \
  -n external-secrets --create-namespace

# Create AWS credentials secret for ESO (local Kind only)
kubectl create secret generic aws-secret-manager-credentials \
  --namespace external-secrets \
  --from-literal=access-key=<AWS_ACCESS_KEY_ID> \
  --from-literal=secret-access-key=<AWS_SECRET_ACCESS_KEY>

# Apply ClusterSecretStore
kubectl apply -f k8s/cluster-secret-store.yaml
```

### 5. Install NGINX Ingress Controller

```bash
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm install ingress-nginx ingress-nginx/ingress-nginx -n ingress-nginx --create-namespace
```

### 6. Deploy the application with Helm

```bash
helm lint ./helm/tickasis
helm upgrade --install tickasis ./helm/tickasis -n tickasis
```

### 7. Verify

```bash
kubectl get pods -n tickasis
kubectl get services -n tickasis
kubectl get ingress -n tickasis
kubectl get externalsecret -n tickasis
kubectl get pvc -n tickasis
```

### 8. Access the app

The Ingress routes `tickasis.local` to the frontend. Since Kind does not assign an external IP, use port-forward:

```bash
kubectl port-forward -n ingress-nginx \
  svc/ingress-nginx-controller 8088:80 --address 0.0.0.0
```

Add to your hosts file (Windows: `C:\Windows\System32\drivers\etc\hosts`, Linux: `/etc/hosts`):

```
127.0.0.1  tickasis.local
```

Open **http://tickasis.local:8088**

### What gets deployed

| Resource | Type | Exposed |
|----------|------|---------|
| `tickasis-postgres` | ClusterIP :5432 | Internal only |
| `tickasis-auth` | ClusterIP :8081 | Internal only |
| `tickasis-app` | ClusterIP :8082 | Internal only |
| `tickasis-frontend` | ClusterIP :8080 | Via Ingress only |
| `tickasis-postgres-pvc` | PVC 10Gi | Storage |
| `tickasis-ingress` | Ingress | `tickasis.local` → frontend |
| `tickasis-app-secrets` | Secret | Synced from AWS Secrets Manager |

All services have liveness and readiness probes on `/health`.

> For production EKS deployment considerations see `REVIEW-DEPLOY.md`.
