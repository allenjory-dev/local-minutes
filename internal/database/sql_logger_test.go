package database

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// A failed write is logged (SQL text and error) without the values it carried.
func TestSQLLoggerOmitsBoundValues(t *testing.T) {
	const marker = "TRANSCRIPT-TEXT-MARKER-55e1"
	var buf bytes.Buffer
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "log.db")), &gorm.Config{Logger: NewSQLLogger(&buf)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT NOT NULL UNIQUE)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO notes (body) VALUES (?)", marker).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO notes (body) VALUES (?)", marker).Error; err == nil {
		t.Fatal("expected a unique-constraint failure")
	}
	out := buf.String()
	if !strings.Contains(out, "INSERT INTO notes") {
		t.Fatalf("the failing statement should still be logged, got %q", out)
	}
	if strings.Contains(out, marker) {
		t.Fatalf("bound values reached the SQL log: %q", out)
	}
}

// Initialize wires the value-free logger: a failed write through the global DB
// prints the statement but not the transcript text it carried.
func TestInitializeUsesValueFreeSQLLogger(t *testing.T) {
	const marker = "TRANSCRIPT-TEXT-MARKER-77b0"
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prev := os.Stdout
	os.Stdout = w // the SQL logger binds stdout when the database is opened
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	initErr := Initialize(filepath.Join(t.TempDir(), "wired.db"))
	os.Stdout = prev
	if initErr != nil {
		t.Fatal(initErr)
	}
	// transcription_jobs.audio_path is NOT NULL; the failing insert carries the marker.
	execErr := DB.Exec("INSERT INTO transcription_jobs (id, title, status, audio_path) VALUES (?, ?, ?, NULL)", "job-1", marker, "pending").Error
	_ = Close()
	_ = w.Close()
	out := <-done
	if execErr == nil {
		t.Fatal("expected the insert to fail")
	}
	if !strings.Contains(out, "INSERT INTO transcription_jobs") {
		t.Fatalf("expected the failing statement in the SQL log, got %q", out)
	}
	if strings.Contains(out, marker) {
		t.Fatalf("bound values reached the SQL log: %q", out)
	}
}
