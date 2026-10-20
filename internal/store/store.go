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
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"

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
// exist. It checks that the file is in WAL mode and that foreign keys are
// enforced, on the first connection of each pool; every later connection
// is opened with the same settings. It does not apply migrations; call
// Migrate next.
//
// The directory must exist and should be on a local disk: SQLite's WAL
// mode relies on file locks and shared memory that network file systems
// such as NFS do not provide reliably.
func Open(ctx context.Context, path string) (*DB, error) {
	// The driver splits its connection string at the first "?".
	if strings.ContainsRune(path, '?') {
		return nil, fmt.Errorf("open %s: the database path must not contain \"?\"", path)
	}
	// SQLite would create the file readable by everyone (0644 under the
	// usual umask); it holds health data, so only its owner may read it.
	// SQLite gives the -wal and -shm files the database file's mode. An
	// existing file keeps the mode its operator chose.
	// The *fs.PathError already names the operation and the path.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	writeParams := url.Values{}
	writeParams.Set("_busy_timeout", strconv.Itoa(busyTimeoutMS))
	writeParams.Set("_foreign_keys", "1")
	writeParams.Set("_journal_mode", "WAL")
	// FULL syncs the WAL on every commit. Writes are rare and the database
	// is the only copy of what users logged, so durability wins over the
	// throughput NORMAL would buy.
	writeParams.Set("_synchronous", "FULL")
	// Every write transaction takes the write lock up front, so a
	// transaction that starts by reading can never fail when it later
	// upgrades to a write.
	writeParams.Set("_txlock", "immediate")
	// Temporary tables, indexes and sort spills stay in memory. With a
	// file temp store, SQLite needs a writable temp directory for large
	// sorts, index builds and table rebuilds, and a read-only container
	// (Docker --read-only, Kubernetes readOnlyRootFilesystem) has none:
	// such statements fail with "disk I/O error" once data outgrows the
	// page cache. Everything tdeemon writes must go to the data directory.
	writeParams.Add("_pragma", "temp_store(memory)")

	write, err := sql.Open("sqlite", path+"?"+writeParams.Encode())
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	write.SetMaxOpenConns(1)
	write.SetMaxIdleConns(1)

	// Readers get only what applies to them. The journal mode belongs to
	// the file (the write pool sets it), and a write lock would make
	// BEGIN fail on a query-only connection.
	readParams := url.Values{}
	readParams.Set("_busy_timeout", strconv.Itoa(busyTimeoutMS))
	readParams.Set("_foreign_keys", "1")
	readParams.Set("_query_only", "1")
	readParams.Add("_pragma", "temp_store(memory)")
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
// any statement that would change the database fails. A transaction on
// it (BeginTx) reads one consistent snapshot, even while writes commit.
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

// Close closes both pools. It does not wait for connections that are
// still in use, so a shutdown that gave up on a slow request cannot hang
// here. A transaction that is still running keeps its connection and can
// still commit; that connection closes when the transaction ends.
func (db *DB) Close() error { return db.close() }

func (db *DB) close() error {
	return errors.Join(db.read.Close(), db.write.Close())
}
