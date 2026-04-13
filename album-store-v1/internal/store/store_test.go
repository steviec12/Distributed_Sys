package store

import (
	"testing"
	"time"
)

type fakePool struct {
	maxOpenConns    int
	maxIdleConns    int
	connMaxLifetime time.Duration
	connMaxIdleTime time.Duration
}

func (f *fakePool) SetMaxOpenConns(n int) {
	f.maxOpenConns = n
}

func (f *fakePool) SetMaxIdleConns(n int) {
	f.maxIdleConns = n
}

func (f *fakePool) SetConnMaxLifetime(d time.Duration) {
	f.connMaxLifetime = d
}

func (f *fakePool) SetConnMaxIdleTime(d time.Duration) {
	f.connMaxIdleTime = d
}

func TestConfigurePoolSetsBoundedConnectionDefaults(t *testing.T) {
	pool := &fakePool{}

	configurePool(pool)

	if pool.maxOpenConns != defaultMaxOpenConns {
		t.Fatalf("max open conns = %d, want %d", pool.maxOpenConns, defaultMaxOpenConns)
	}
	if pool.maxIdleConns != defaultMaxIdleConns {
		t.Fatalf("max idle conns = %d, want %d", pool.maxIdleConns, defaultMaxIdleConns)
	}
	if pool.connMaxLifetime != defaultConnMaxLifetime {
		t.Fatalf("conn max lifetime = %s, want %s", pool.connMaxLifetime, defaultConnMaxLifetime)
	}
	if pool.connMaxIdleTime != defaultConnMaxIdleTime {
		t.Fatalf("conn max idle time = %s, want %s", pool.connMaxIdleTime, defaultConnMaxIdleTime)
	}
}
