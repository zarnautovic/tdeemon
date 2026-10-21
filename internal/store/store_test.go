package store_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

// queryRower is satisfied by *sql.DB, *sql.Conn and *sql.Tx.
type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func pragma(ctx context.Context, t *testing.T, q queryRower, name string) string {
	t.Helper()

	var v string
	if err := q.QueryRowContext(ctx, "PRAGMA "+name).Scan(&v); err != nil {
		t.Fatalf("PRAGMA %s: %v", name, err)
	}
	return v
}

func exec(t *testing.T, db *store.DB, query string, args ...any) {
	t.Helper()

	err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), query, args...)
		return err
	})
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// rowsInT counts the rows of table t, the one most tests create. ctx
// lets a caller bound a read that must not wait.
func rowsInT(ctx context.Context, t *testing.T, db *store.DB) int {
	t.Helper()

	var n int
	if err := db.Read().QueryRowContext(ctx, "SELECT count(*) FROM t").Scan(&n); err != nil {
		t.Fatalf("count the rows of t: %v", err)
	}
	return n
}

func TestOpenConfiguresWriteConnection(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	want := map[string]string{
		"journal_mode": "wal",
		"foreign_keys": "1",
		"busy_timeout": "5000",
		"synchronous":  "2", // FULL
		"query_only":   "0",
		"temp_store":   "2", // MEMORY
	}
	err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
		for name, w := range want {
			if got := pragma(t.Context(), t, tx, name); got != w {
				t.Errorf("write connection: %s = %s, want %s", name, got, w)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WriteTx: %v", err)
	}
}

func TestOpenConfiguresEveryReadConnection(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	want := map[string]string{
		"journal_mode": "wal",
		"foreign_keys": "1",
		"busy_timeout": "5000",
		"query_only":   "1",
		"temp_store":   "2", // MEMORY
	}
	// Hold several connections at once so the pool has to open new ones.
	for i := range 3 {
		conn, err := db.Read().Conn(t.Context())
		if err != nil {
			t.Fatalf("read connection %d: %v", i, err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		for name, w := range want {
			if got := pragma(t.Context(), t, conn, name); got != w {
				t.Errorf("read connection %d: %s = %s, want %s", i, name, got, w)
			}
		}
	}
}

func TestReadRejectsWrites(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	if _, err := db.Read().ExecContext(t.Context(), "CREATE TABLE t (n INTEGER) STRICT"); err == nil {
		t.Fatal("CREATE TABLE through Read succeeded, want an error")
	}
}

func TestReadsDoNotWaitForWriter(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	exec(t, db, "CREATE TABLE t (n INTEGER NOT NULL) STRICT")

	inTx, release := make(chan struct{}), make(chan struct{})
	// If the test fails before releasing the writer, cleanup must still
	// release it, or the test would hang instead of failing.
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)
	done := make(chan error, 1)
	go func() {
		done <- db.WriteTx(t.Context(), func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(t.Context(), "INSERT INTO t (n) VALUES (1)"); err != nil {
				return err
			}
			close(inTx)
			<-release
			return nil
		})
	}()
	<-inTx

	// The writer holds its transaction open; a read must still answer
	// at once, from the last committed state.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if n := rowsInT(ctx, t, db); n != 0 {
		t.Errorf("read during the write saw %d rows, want 0 (uncommitted)", n)
	}

	releaseOnce()
	if err := <-done; err != nil {
		t.Fatalf("WriteTx: %v", err)
	}
	if n := rowsInT(t.Context(), t, db); n != 1 {
		t.Errorf("read after commit saw %d rows, want 1", n)
	}
}

func TestReadTxSeesOneSnapshot(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	exec(t, db, "CREATE TABLE t (n INTEGER NOT NULL) STRICT")

	tx, err := db.Read().BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("BeginTx on the read pool: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	n := func() int {
		t.Helper()
		var n int
		if err := tx.QueryRowContext(t.Context(), "SELECT count(*) FROM t").Scan(&n); err != nil {
			t.Fatalf("count in the read transaction: %v", err)
		}
		return n
	}
	// The snapshot starts with the first read.
	if got := n(); got != 0 {
		t.Fatalf("first read saw %d rows, want 0", got)
	}
	exec(t, db, "INSERT INTO t (n) VALUES (1)")
	if got := n(); got != 0 {
		t.Errorf("second read in the same transaction saw %d rows, want 0 (the snapshot)", got)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("end read transaction: %v", err)
	}
	if got := rowsInT(t.Context(), t, db); got != 1 {
		t.Errorf("read after the transaction saw %d rows, want 1", got)
	}
}

func TestCloseDoesNotWaitForBusyWriter(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	exec(t, db, "CREATE TABLE t (n INTEGER NOT NULL) STRICT")
	inTx, release := make(chan struct{}), make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)
	written := make(chan error, 1)
	go func() {
		written <- db.WriteTx(context.Background(), func(tx *sql.Tx) error {
			close(inTx)
			<-release
			// Runs after Close: the transaction still has its connection.
			_, err := tx.ExecContext(context.Background(), "INSERT INTO t (n) VALUES (1)")
			return err
		})
	}()
	<-inTx

	closed := make(chan error, 1)
	go func() { closed <- db.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Errorf("Close returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close blocked while a write transaction held the connection")
	}

	// As documented on Close, the running transaction can still finish.
	releaseOnce()
	select {
	case err := <-written:
		if err != nil {
			t.Errorf("the transaction that outlived Close failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the transaction that outlived Close did not finish")
	}
}

func TestWriteTxSerialisesWriters(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	exec(t, db, "CREATE TABLE counter (n INTEGER NOT NULL) STRICT")
	exec(t, db, "INSERT INTO counter (n) VALUES (0)")

	const writers, increments = 8, 25
	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			for range increments {
				// Read, then write in the same transaction: without
				// serialisation two writers would lose an update or
				// fail with SQLITE_BUSY.
				err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
					var n int
					if err := tx.QueryRowContext(t.Context(), "SELECT n FROM counter").Scan(&n); err != nil {
						return err
					}
					_, err := tx.ExecContext(t.Context(), "UPDATE counter SET n = ?", n+1)
					return err
				})
				if err != nil {
					t.Errorf("WriteTx: %v", err)
					return
				}
			}
		})
	}
	wg.Wait()

	var n int
	if err := db.Read().QueryRowContext(t.Context(), "SELECT n FROM counter").Scan(&n); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	if n != writers*increments {
		t.Errorf("counter = %d, want %d", n, writers*increments)
	}
}

func TestWriteTxRollsBackOnError(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	exec(t, db, "CREATE TABLE t (n INTEGER NOT NULL) STRICT")

	errBoom := errors.New("boom")
	err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(t.Context(), "INSERT INTO t (n) VALUES (1)"); err != nil {
			return err
		}
		return errBoom
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("WriteTx returned %v, want %v", err, errBoom)
	}
	if n := rowsInT(t.Context(), t, db); n != 0 {
		t.Errorf("table has %d rows after a failed transaction, want 0", n)
	}
}

func TestWriteTxRollsBackOnPanic(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	exec(t, db, "CREATE TABLE t (n INTEGER NOT NULL) STRICT")

	func() {
		defer func() {
			if recover() == nil {
				t.Error("WriteTx swallowed the panic")
			}
		}()
		_ = db.WriteTx(t.Context(), func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(t.Context(), "INSERT INTO t (n) VALUES (1)"); err != nil {
				return err
			}
			panic("boom")
		})
	}()

	if n := rowsInT(t.Context(), t, db); n != 0 {
		t.Errorf("table has %d rows after a panic, want 0", n)
	}
	// The only write connection must be free again.
	exec(t, db, "INSERT INTO t (n) VALUES (2)")
}

func TestOpenCreatesPrivateFiles(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), store.FileName)
	db, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// A write makes sure the -wal and -shm files exist.
	exec(t, db, "CREATE TABLE t (n INTEGER) STRICT")

	for _, f := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(f)
		if err != nil {
			t.Errorf("stat %s: %v", filepath.Base(f), err)
			continue
		}
		if mode := info.Mode().Perm(); mode&0o077 != 0 {
			t.Errorf("%s has mode %v, want no access for group or others", filepath.Base(f), mode)
		}
	}
}

func TestOpenRejectsQuestionMarkInPath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "what?", store.FileName)
	db, err := store.Open(t.Context(), path)
	if err == nil {
		_ = db.Close()
		t.Fatal("Open accepted a path with \"?\", want an error")
	}
	if !strings.Contains(err.Error(), `must not contain "?"`) {
		t.Errorf("error %q does not explain the problem", err)
	}
}

func TestOpenFailsWithoutDirectory(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing", store.FileName)
	db, err := store.Open(t.Context(), path)
	if err == nil {
		_ = db.Close()
		t.Fatal("Open succeeded in a missing directory, want an error")
	}
	if n := strings.Count(err.Error(), path); n != 1 {
		t.Errorf("error %q names the path %d times, want once", err, n)
	}
}
