package store

import "testing"

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
