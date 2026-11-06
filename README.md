# GoLive — Todo App

A classic, attractive Todo web application built with pure **Go** (no external dependencies). Dark-themed UI, runs in Docker.

## Features

- Add, complete, and delete todos
- Live stats (total / done / pending)
- Concurrency-safe in-memory store
- Zero external Go dependencies
- Tiny Docker image (scratch-based, ~7 MB)

## Project Structure

```
golive/
├── main.go              # Entry point, routes
├── handler/
│   └── todo.go          # Handlers + in-memory store
├── templates/
│   └── index.html       # HTML + CSS (dark gradient UI)
├── static/              # (reserved for future assets)
├── Dockerfile           # Multi-stage build
├── docker-compose.yml
└── go.mod
```

## Run Locally

```bash
go run .
# open http://localhost:8080
```

## Run with Docker

```bash
docker compose up --build
# open http://localhost:8080
```

## Deploy on Another Machine

1. Copy the repo (or `git clone`)
2. Make sure Docker + Docker Compose are installed
3. Run:

```bash
docker compose up -d --build
```

The app will be available on port **8080**.

To stop:

```bash
docker compose down
```

## Tech Stack

| Layer     | Technology            |
|-----------|-----------------------|
| Language  | Go 1.22               |
| Templates | `html/template`       |
| Server    | `net/http`            |
| Container | Docker (scratch image)|
