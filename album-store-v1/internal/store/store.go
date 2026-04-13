package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// ErrNotFound is returned when a requested row should be treated as absent.
var ErrNotFound = errors.New("not found")

const (
	defaultMaxOpenConns    = 50
	defaultMaxIdleConns    = 25
	defaultConnMaxLifetime = 5 * time.Minute
	defaultConnMaxIdleTime = 1 * time.Minute
)

type poolConfigurer interface {
	SetMaxOpenConns(int)
	SetMaxIdleConns(int)
	SetConnMaxLifetime(time.Duration)
	SetConnMaxIdleTime(time.Duration)
}

// Store wraps the shared Postgres handle used by the repositories below.
type Store struct {
	db *sql.DB
}

// Open creates a Postgres-backed store and verifies the connection eagerly.
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	configurePool(db)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return New(db), nil
}

// New wraps an existing SQL handle for tests or external setup code.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func configurePool(db poolConfigurer) {
	db.SetMaxOpenConns(defaultMaxOpenConns)
	db.SetMaxIdleConns(defaultMaxIdleConns)
	db.SetConnMaxLifetime(defaultConnMaxLifetime)
	db.SetConnMaxIdleTime(defaultConnMaxIdleTime)
}

// Close releases the underlying Postgres connection pool.
func (s *Store) Close() error {
	return s.db.Close()
}
