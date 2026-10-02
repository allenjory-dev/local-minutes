package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"scriberr/internal/config"
	"scriberr/internal/models"
	"scriberr/internal/repository"
	"scriberr/internal/service"
)

type unavailableJobRepository struct{ repository.JobRepository }

func (unavailableJobRepository) Create(context.Context, *models.TranscriptionJob) error {
	return errors.New("database unavailable")
}

func TestUploadPreservesOriginal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, extension             string
		convert, invalid, dbFailure bool
	}{
		{name: "wav", extension: ".wav"},
		{name: "webm", extension: ".webm", convert: true},
		{name: "uppercase webm", extension: ".WEBM", convert: true},
		{name: "conversion failure", extension: ".webm", convert: true, invalid: true},
		{name: "wav database failure", extension: ".wav", dbFailure: true},
		{name: "webm database failure", extension: ".webm", convert: true, dbFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uploadDir := t.TempDir()
			content := []byte("synthetic uploaded bytes")
			if tc.convert {
				if _, err := exec.LookPath("ffmpeg"); err != nil {
					t.Skip("ffmpeg required for WebM integration tests")
				}
				if !tc.invalid {
					fixture := filepath.Join(t.TempDir(), "sample.webm")
					cmd := exec.Command("ffmpeg", "-nostdin", "-n", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.1", "-c:a", "libopus", fixture)
					if output, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("fixture: %v %s", err, output)
					}
					var err error
					content, err = os.ReadFile(fixture)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			if err := db.AutoMigrate(&models.TranscriptionJob{}); err != nil {
				t.Fatal(err)
			}
			var repo repository.JobRepository = repository.NewJobRepository(db)
			if tc.dbFailure {
				repo = unavailableJobRepository{}
			}
			h := &Handler{config: &config.Config{UploadDir: uploadDir}, fileService: service.NewFileService(), jobRepo: repo}
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			part, err := writer.CreateFormFile("audio", "source"+tc.extension)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write(content); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/upload", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			router := gin.New()
			router.POST("/upload", h.UploadAudio)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			wantStatus := http.StatusOK
			if tc.invalid || tc.dbFailure {
				wantStatus = http.StatusInternalServerError
			}
			if response.Code != wantStatus {
				t.Fatalf("status %d: %s", response.Code, response.Body)
			}
			originals, err := filepath.Glob(filepath.Join(uploadDir, "*"+tc.extension))
			if err != nil || len(originals) != 1 {
				t.Fatalf("original not retained: %v %v", originals, err)
			}
			retained, err := os.ReadFile(originals[0])
			if err != nil || sha256.Sum256(retained) != sha256.Sum256(content) {
				t.Fatal("original bytes changed")
			}
			if wantStatus == http.StatusOK {
				var job models.TranscriptionJob
				if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
					t.Fatal(err)
				}
				var stored models.TranscriptionJob
				if err := db.First(&stored, "id = ?", job.ID).Error; err != nil {
					t.Fatal(err)
				}
				if stored.OriginalAudioPath != originals[0] {
					t.Fatal("original reference not persisted")
				}
				if tc.convert {
					if stored.AudioPath == stored.OriginalAudioPath {
						t.Fatal("derivative overwrote source")
					}
					if info, err := os.Stat(stored.AudioPath); err != nil || info.Size() == 0 {
						t.Fatal("missing playback derivative")
					}
				} else if stored.AudioPath != stored.OriginalAudioPath {
					t.Fatal("non-WebM upload changed path")
				}
			} else {
				files, _ := os.ReadDir(uploadDir)
				if len(files) != 1 {
					t.Fatal("failed upload left a partial derivative")
				}
			}
		})
	}
}
