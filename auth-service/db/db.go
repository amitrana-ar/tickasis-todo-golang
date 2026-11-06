package db

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var Pool *pgxpool.Pool

func Connect() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is not set")
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
		log.Printf("[auth-service] waiting for database... attempt %d/10", i+1)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatalf("[auth-service] failed to connect: %v", err)
	}

	Pool = pool
	log.Println("[auth-service] connected to PostgreSQL")
	createSchema()
}

func createSchema() {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id            SERIAL      PRIMARY KEY,
			name          TEXT        NOT NULL,
			email         TEXT        NOT NULL UNIQUE,
			password_hash TEXT        NOT NULL,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token      TEXT        PRIMARY KEY,
			user_id    INT         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days'
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);`,
	}
	for _, q := range queries {
		if _, err := Pool.Exec(context.Background(), q); err != nil {
			log.Fatalf("[auth-service] schema error: %v", err)
		}
	}
	log.Println("[auth-service] schema ready")
}
