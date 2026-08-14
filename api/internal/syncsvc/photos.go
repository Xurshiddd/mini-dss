package syncsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Rasmlarni HEMIS'dan yuklab olish va terminal talablariga tekshirish.
//
// ⚠️ Alohida bosqich: sync ichida ketma-ket qilinsa 8000+ rasm uchun
// soatlab vaqt ketadi. Bu yerda bir necha ishchi parallel yuklaydi.
//
// ⚠️ face-api javob bermasa rasm "yaroqsiz" deb belgilanMAYDI — bu bizning
// nosozligimiz, rasmning aybi emas. `pending` da qoladi va qayta tekshiriladi.

const photoWorkers = 4

type photoJob struct {
	personID int64
	userID   string
	url      string
}

// StartPhotoFetch — rasmi yo'q yoki manbadagi URL o'zgargan odamlarning
// rasmini yuklaydi.
func (s *Service) StartPhotoFetch(faceAPI string) error {
	s.mu.Lock()
	if s.photos != nil && s.photos.Running {
		s.mu.Unlock()
		return fmt.Errorf("rasm yuklash allaqachon ketyapti")
	}
	s.photos = &HemisProgress{Running: true, StartedAt: time.Now(), Stage: "photos"}
	s.mu.Unlock()

	go s.fetchPhotos(faceAPI)
	return nil
}

func (s *Service) PhotoProgress() *HemisProgress {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.photos == nil {
		return nil
	}
	copied := *s.photos
	return &copied
}

func (s *Service) updatePhotos(fn func(*HemisProgress)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.photos != nil {
		fn(s.photos)
	}
}

func (s *Service) fetchPhotos(faceAPI string) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()

	runID, _ := s.store.StartRun(ctx, "photo_fetch", nil)

	var (
		mu                             sync.Mutex
		total, valid, rejected, failed int
	)

	finish := func(err error) {
		now := time.Now()
		status := "done"
		var errMsg *string
		if err != nil {
			status = "failed"
			msg := err.Error()
			errMsg = &msg
		}
		if err := s.store.FinishRun(ctx, runID, status, total, valid, 0, rejected, failed, errMsg); err != nil {
			log.Printf("sync log yozilmadi: %v", err)
		}

		s.updatePhotos(func(p *HemisProgress) {
			p.Running = false
			p.EndedAt = &now
			if err != nil {
				p.Error = err.Error()
			}
		})
	}

	pending, err := s.store.PeopleNeedingPhoto(ctx, 100000)
	if err != nil {
		finish(err)
		return
	}

	s.updatePhotos(func(p *HemisProgress) { p.Total = len(pending) })

	jobs := make(chan photoJob)
	var wg sync.WaitGroup

	for range photoWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// ⚠️ SSRF himoyasi: ichki/lokal IP'larga ulanishni rad etadi.
			client := safeHTTPClient(60 * time.Second)

			for job := range jobs {
				status, reason, metrics := s.fetchOne(ctx, client, faceAPI, job)

				_ = s.store.UpdatePersonPhotoFromSource(ctx, job.personID,
					"photos/"+job.userID+".jpg", status, reason, metrics, job.url)

				mu.Lock()
				total++
				switch status {
				case "valid":
					valid++
				case "rejected":
					rejected++
				default:
					failed++
				}
				t, v, r, f := total, valid, rejected, failed
				mu.Unlock()

				s.updatePhotos(func(p *HemisProgress) {
					p.Created, p.Skipped, p.Failed = v, r, f
					p.Updated = t
				})
			}
		}()
	}

	for _, p := range pending {
		select {
		case jobs <- photoJob{personID: p.ID, userID: p.UserID, url: p.PhotoURL}:
		case <-ctx.Done():
		}
	}
	close(jobs)
	wg.Wait()

	finish(nil)
}

// fetchOne — bitta rasmni yuklab, tekshiradi va diskka yozadi.
func (s *Service) fetchOne(ctx context.Context, client *http.Client,
	faceAPI string, job photoJob) (status, reason string, metrics []byte) {

	// ⚠️ Faqat http/https — `file://`, `gopher://` va shu kabilarni rad
	// etamiz. IP darajasidagi himoya safeHTTPClient'da (SSRF).
	if u, err := url.Parse(job.url); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "rejected", "URL sxemasi yaroqsiz (faqat http/https)", nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, job.url, nil)
	if err != nil {
		return "rejected", "URL yaroqsiz", nil
	}

	resp, err := client.Do(req)
	if err != nil {
		// ⚠️ Manbadagi URL ochilmasligi mumkin (masalan dev placeholder).
		// Buni "rasm yomon" deb emas, "olib bo'lmadi" deb belgilaymiz.
		return "rejected", "manba URL ochilmadi", nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "rejected", fmt.Sprintf("rasm yuklanmadi: HTTP %d", resp.StatusCode), nil
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 12<<20))
	if err != nil || len(data) == 0 {
		return "rejected", "rasm o'qilmadi", nil
	}

	name := job.userID + ".jpg"
	if err := os.WriteFile(filepath.Join(s.photoDir, name), data, 0o644); err != nil {
		return "pending", "", nil
	}

	valid, reasons, m := validateWithFaceAPI(ctx, faceAPI, data, name)
	metrics, _ = json.Marshal(m)

	switch {
	case reasons == "face_api_unavailable":
		// Servis ishlamayapti — rasmni ayblamaymiz.
		return "pending", "", metrics
	case valid:
		return "valid", "", metrics
	default:
		return "rejected", reasons, metrics
	}
}

func validateWithFaceAPI(ctx context.Context, baseURL string, data []byte, filename string) (bool, string, map[string]any) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return false, "face_api_unavailable", nil
	}
	_, _ = part.Write(data)
	_ = w.Close()

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(baseURL, "/")+"/validate", &body)
	if err != nil {
		return false, "face_api_unavailable", nil
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, "face_api_unavailable", nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, "face_api_unavailable", nil
	}

	var out struct {
		Valid   bool           `json:"valid"`
		Reasons []string       `json:"reasons"`
		Metrics map[string]any `json:"metrics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, "face_api_unavailable", nil
	}

	return out.Valid, strings.Join(out.Reasons, ", "), out.Metrics
}
