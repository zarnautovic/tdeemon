package store

import (
	"slices"
	"testing"
)

func TestTransactionControl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		script   string
		wantStmt string // "" means the script is allowed
		wantLine int
	}{
		{name: "plain DDL", script: "CREATE TABLE a (x INTEGER) STRICT;\nCREATE INDEX a_x ON a (x);"},
		{name: "commit", script: "CREATE TABLE a (x INTEGER);\nCOMMIT;", wantStmt: "COMMIT", wantLine: 2},
		{name: "lower case", script: "commit", wantStmt: "COMMIT", wantLine: 1},
		{name: "end transaction", script: "CREATE TABLE a (x INTEGER);\n\nEND TRANSACTION;", wantStmt: "END", wantLine: 3},
		{name: "rollback", script: "ROLLBACK;", wantStmt: "ROLLBACK", wantLine: 1},
		{name: "rollback transaction", script: "ROLLBACK TRANSACTION;", wantStmt: "ROLLBACK", wantLine: 1},
		{name: "begin", script: "BEGIN IMMEDIATE;\nCREATE TABLE a (x INTEGER);", wantStmt: "BEGIN", wantLine: 1},
		{name: "savepoints", script: "SAVEPOINT s; ROLLBACK TO s; ROLLBACK TRANSACTION TO SAVEPOINT s; RELEASE s;"},
		{
			name: "trigger body and its END",
			script: `CREATE TRIGGER t AFTER INSERT ON a BEGIN
	UPDATE a SET x = CASE WHEN new.x > 0 THEN 1 ELSE 0 END WHERE rowid = new.rowid;
	SELECT 1;
END;`,
		},
		{
			name:   "temporary trigger",
			script: "CREATE TEMPORARY TRIGGER IF NOT EXISTS t AFTER INSERT ON a BEGIN SELECT 1; END;",
		},
		{
			name:     "commit after a trigger",
			script:   "CREATE TRIGGER t AFTER INSERT ON a BEGIN SELECT 1; END;\nCOMMIT;",
			wantStmt: "COMMIT", wantLine: 2,
		},
		{name: "keywords in strings", script: "INSERT INTO a (s) VALUES ('COMMIT; END'), ('it''s; ROLLBACK');"},
		{name: "keywords in identifiers", script: `SELECT "commit;", [end;], ` + "`rollback;`" + ` FROM a;`},
		{name: "keywords in comments", script: "-- COMMIT;\n/* END;\nROLLBACK; */ SELECT 1;"},
		{
			name:     "line numbers count comments and strings",
			script:   "/* one\ntwo */ INSERT INTO a VALUES ('x\ny');\n-- four\nCOMMIT;",
			wantStmt: "COMMIT", wantLine: 5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stmt, line, found := transactionControl(tt.script)
			if tt.wantStmt == "" {
				if found {
					t.Fatalf("found %s on line %d, want the script allowed", stmt, line)
				}
				return
			}
			if !found || stmt != tt.wantStmt || line != tt.wantLine {
				t.Errorf("got %q on line %d (found %v), want %q on line %d", stmt, line, found, tt.wantStmt, tt.wantLine)
			}
		})
	}
}

func TestSplitStatements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		script string
		want   []statement
	}{
		{
			name:   "statements and their lines",
			script: "SELECT 1;\n\nSELECT 2",
			want:   []statement{{"SELECT 1", 1}, {"  SELECT 2", 3}},
		},
		{
			name:   "doubled quote inside a string",
			script: "SELECT 'it''s; x' ; SELECT 2",
			want:   []statement{{"SELECT   ", 1}, {" SELECT 2", 1}},
		},
		{
			name:   "quoted identifiers",
			script: "SELECT [a;b], `c;d`, \"e\"\";f\"; x",
			want:   []statement{{"SELECT  ,  ,  ", 1}, {" x", 1}},
		},
		{
			// SQLite ends [ at the first ]; ]] is no escape there.
			name:   "a bracket identifier ends at the first ]",
			script: "SELECT [a]]; x",
			want:   []statement{{"SELECT  ]", 1}, {" x", 1}},
		},
		{
			name:   "a statement that starts with a quote",
			script: "'a\n' AS y;",
			want:   []statement{{"  AS y", 1}},
		},
		{
			name:   "a comment separates words",
			script: "END/**/TRANSACTION;",
			want:   []statement{{"END TRANSACTION", 1}},
		},
		{
			name:   "comments are dropped",
			script: "SELECT /* c */ 1 -- d\n;",
			want:   []statement{{"SELECT   1  ", 1}},
		},
		{
			name:   "unterminated string",
			script: "SELECT 'x; y",
			want:   []statement{{"SELECT  ", 1}},
		},
		{name: "unterminated comment", script: "/* x; y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := splitStatements(tt.script); !slices.Equal(got, tt.want) {
				t.Errorf("splitStatements(%q)\n got %#v\nwant %#v", tt.script, got, tt.want)
			}
		})
	}
}
