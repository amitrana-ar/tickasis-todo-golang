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
		log.Printf("[todo-service] waiting for database... attempt %d/10", i+1)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatalf("[todo-service] failed to connect: %v", err)
	}

	Pool = pool
	log.Println("[todo-service] connected to PostgreSQL")
	createSchema()
}

func createSchema() {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS todos (
			id          SERIAL      PRIMARY KEY,
			user_id     INT         NOT NULL,
			subject     TEXT        NOT NULL,
			description TEXT        NOT NULL DEFAULT '',
			done        BOOLEAN     NOT NULL DEFAULT FALSE,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE INDEX IF NOT EXISTS idx_todos_user_id ON todos(user_id);`,
	}
	for _, q := range queries {
		if _, err := Pool.Exec(context.Background(), q); err != nil {
			log.Fatalf("[todo-service] schema error: %v", err)
		}
	}
	log.Println("[todo-service] schema ready")
}
