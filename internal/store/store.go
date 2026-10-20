// Package store owns the SQLite database: it opens the file with the
// settings the rest of the code relies on, applies the embedded
// migrations and hands out connections for reads and writes.
//
// Writes go through a single connection, so concurrent writers queue in
// Go instead of failing with SQLITE_BUSY. Reads use a separate pool of
// query-only connections and, thanks to WAL, never wait for a writer.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"runtime"
	"strconv"

	_ "modernc.org/sqlite" // Registers the pure-Go "sqlite" driver.
)

// FileName is the name of the database file inside the data directory.
const FileName = "tdeemon.db"

// busyTimeoutMS is how long a connection waits for a lock held by
// another connection before giving up. Writers never contend with each
// other (there is one write connection), so this mostly covers a
// checkpoint or a backup running next to a write.
const busyTimeoutMS = 5000

// DB is the application database.
type DB struct {
	path  string
	write *sql.DB
	read  *sql.DB
}

// Open opens the database at path, creating the file if it does not
// exist, and verifies that every connection runs with WAL, foreign keys
// and a busy timeout. It does not apply migrations; call Migrate next.
//
// The directory must exist and should be on a local disk: SQLite's WAL
// mode relies on file locks and shared memory that network file systems
// such as NFS do not provide reliably.
func Open(ctx context.Context, path string) (*DB, error) {
	common := url.Values{}
	common.Set("_busy_timeout", strconv.Itoa(busyTimeoutMS))
	common.Set("_foreign_keys", "1")
	common.Set("_journal_mode", "WAL")
	// FULL syncs the WAL on every commit. Writes are rare and the database
	// is the only copy of what users logged, so durability wins over the
	// throughput NORMAL would buy.
	common.Set("_synchronous", "FULL")
	// Every write transaction takes the write lock up front, so a
	// transaction that starts by reading can never fail when it later
	// upgrades to a write.
	common.Set("_txlock", "immediate")

	write, err := sql.Open("sqlite", path+"?"+common.Encode())
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	write.SetMaxOpenConns(1)
	write.SetMaxIdleConns(1)

	readParams := maps.Clone(common)
	readParams.Set("_query_only", "1")
	read, err := sql.Open("sqlite", path+"?"+readParams.Encode())
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open %s: %w", path, err), write.Close())
	}
	readConns := max(4, runtime.GOMAXPROCS(0))
	read.SetMaxOpenConns(readConns)
	read.SetMaxIdleConns(readConns)

	db := &DB{path: path, write: write, read: read}
	// The write pool goes first: its first connection creates the file
	// and switches it to WAL before any reader opens it.
	if err := db.verify(ctx); err != nil {
		return nil, errors.Join(err, db.close())
	}
	return db, nil
}

// verify checks that the settings requested in the DSN actually took
// effect. SQLite silently keeps another journal mode when the file
// system cannot support WAL, and the app must not run like that.
func (db *DB) verify(ctx context.Context) error {
	var mode string
	if err := db.write.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("open %s: %w", db.path, err)
	}
	if mode != "wal" {
		return fmt.Errorf("open %s: journal mode is %q, want wal; is the data directory on a local disk?", db.path, mode)
	}

	for _, p := range []struct {
		name string
		pool *sql.DB
	}{{"write", db.write}, {"read", db.read}} {
		var fk int
		if err := p.pool.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
			return fmt.Errorf("open %s (%s connection): %w", db.path, p.name, err)
		}
		if fk != 1 {
			return fmt.Errorf("open %s (%s connection): foreign keys are off", db.path, p.name)
		}
	}
	return nil
}

// Read returns the pool for queries. Its connections are query-only:
// any statement that would change the database fails.
func (db *DB) Read() *sql.DB { return db.read }

// WriteTx runs fn inside a write transaction and commits it if fn
// returns nil; otherwise, or if fn panics, the transaction is rolled
// back. Calls are serialised: while fn runs, other writers wait.
//
// fn must use tx for every statement. Calling WriteTx again from inside
// fn deadlocks, because the only write connection is already taken.
func (db *DB) WriteTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := db.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin write: %w", err)
	}
	// After a successful Commit this is a no-op that returns ErrTxDone.
	defer func() { _ = tx.Rollback() }()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Close runs PRAGMA optimize, as SQLite recommends before closing a
// long-lived connection, and closes both pools.
func (db *DB) Close() error {
	_, optErr := db.write.ExecContext(context.Background(), "PRAGMA optimize")
	if optErr != nil {
		optErr = fmt.Errorf("optimize: %w", optErr)
	}
	return errors.Join(optErr, db.close())
}

func (db *DB) close() error {
	return errors.Join(db.read.Close(), db.write.Close())
}
