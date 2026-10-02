package database

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// A failed write is logged (SQL text and error) without the values it carried.
func TestSQLLoggerOmitsBoundValues(t *testing.T) {
	const marker = "TRANSCRIPT-TEXT-MARKER-55e1"
	var buf bytes.Buffer
	dir := t.TempDir()
	failIfFilesOpen(t, dir)
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "log.db")), &gorm.Config{Logger: NewSQLLogger(&buf)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	// Close before the temporary folder is removed: Windows cannot delete an
	// open database file, so leaving it open failed this test there.
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
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
	dir := t.TempDir()
	failIfFilesOpen(t, dir)
	t.Cleanup(func() { _ = Close() }) // also on early failure, before dir is removed
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	prev := os.Stdout
	os.Stdout = w // the SQL logger binds stdout when the database is opened
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	initErr := Initialize(filepath.Join(dir, "wired.db"))
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

// failIfFilesOpen fails the test if any file under dir is still open when the
// test ends. Windows refuses to delete open files, so a database left open
// fails t.TempDir's cleanup there; this catches the same leak on Linux. Call it
// after t.TempDir and before registering the cleanup that closes the files.
func failIfFilesOpen(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		return
	}
	t.Cleanup(func() {
		fds, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			return
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join("/proc/self/fd", fd.Name()))
			if err == nil && strings.HasPrefix(target, dir+string(filepath.Separator)) {
				t.Errorf("file still open when the test ended: %s", filepath.Base(target))
			}
		}
	})
}
