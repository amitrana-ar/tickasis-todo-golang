package db

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool is the shared database connection pool
var Pool *pgxpool.Pool

// Connect opens a connection pool to PostgreSQL and creates the schema.
// Retries for up to 30 seconds to handle slow Postgres startup.
func Connect() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}

	var pool *pgxpool.Pool
	var err error

	for i := 0; i < 10; i++ {
		pool, err = pgxpool.New(context.Background(), dsn)
		if err == nil {
			if pingErr := pool.Ping(context.Background()); pingErr == nil {
				break
			}
		}
		log.Printf("Waiting for database... attempt %d/10", i+1)
		time.Sleep(3 * time.Second)
	}

	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	Pool = pool
	log.Println("Connected to PostgreSQL")

	createSchema()
}

// createSchema creates all required tables if they don't already exist.
func createSchema() {
	queries := []string{
		// Users table — stores credentials
		`CREATE TABLE IF NOT EXISTS users (
			id            SERIAL PRIMARY KEY,
			name          TEXT        NOT NULL,
			email         TEXT        NOT NULL UNIQUE,
			password_hash TEXT        NOT NULL,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,

		// Sessions table — maps cookie token → user
		`CREATE TABLE IF NOT EXISTS sessions (
			token      TEXT        PRIMARY KEY,
			user_id    INT         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days'
		);`,

		// Todos table — scoped per user, with subject + description
		`CREATE TABLE IF NOT EXISTS todos (
			id          SERIAL      PRIMARY KEY,
			user_id     INT         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			subject     TEXT        NOT NULL,
			description TEXT        NOT NULL DEFAULT '',
			done        BOOLEAN     NOT NULL DEFAULT FALSE,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,

		// Index for fast per-user todo queries
		`CREATE INDEX IF NOT EXISTS idx_todos_user_id ON todos(user_id);`,

		// Index for fast session lookups
		`CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);`,
	}

	for _, q := range queries {
		if _, err := Pool.Exec(context.Background(), q); err != nil {
			log.Fatalf("Failed to create schema: %v\nQuery: %s", err, q)
		}
	}

	log.Println("Database schema ready")
}
