package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// face-api servisiga rasmni yuborib, terminal talablariga mosligini
// tekshiradi (ko'zlar orasi ≥60px, sifat ≥50, bitta yuz, burchak).
// Chegaralar terminalning o'z config'idan olingan.

type photoResult struct {
	Valid   bool           `json:"valid"`
	Reasons []string       `json:"reasons"`
	Metrics map[string]any `json:"metrics"`
}

func validatePhoto(ctx context.Context, baseURL string, data []byte, filename string) (photoResult, error) {
	var out photoResult

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return out, err
	}
	if _, err := part.Write(data); err != nil {
		return out, err
	}
	if err := writer.Close(); err != nil {
		return out, err
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(baseURL, "/")+"/validate", &body)
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("face-api HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

func joinReasons(reasons []string) string {
	return strings.Join(reasons, ", ")
}
