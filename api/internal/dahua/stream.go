package dahua

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// StreamEvent — jonli oqimdan kelgan kirish/chiqish hodisasi.
//
// ⚠️ Log yozuvidan farqi: bu yerda `RecNo` YO'Q. Bog'lash uchun
// `UserID` + `HappenedAt` juftligi ishlatiladi (ikkalasi ham logdagi
// qiymat bilan aynan mos tushadi).
type StreamEvent struct {
	UserID     string
	CardName   string
	HappenedAt time.Time
	Status     int
	ErrorCode  int
	Method     int
	Similarity int // yuz o'xshashligi
	Alive      int // tiriklik bali
}

// AttachEvents — qurilmaning hodisa oqimini ochadi va har bir hodisada
// `onEvent` ni chaqiradi. `ctx` bekor qilinmaguncha bloklaydi.
//
// ⛔ MUHIM: oqim ochiq turganda qurilma port 80'ga YANGI ulanishlarni qabul
// qilmaydi (`connection refused`). Ya'ni shu qurilmaga yozish yoki o'qish
// kerak bo'lsa, avval oqim yopilishi SHART. Oqim yopilgach qurilma darhol
// tiklanadi.
//
// ⚠️ Bir odam bir necha soniya ichida bir necha marta hodisa hosil qiladi
// (o'lchangan: 21 soniyada 4 marta). Takrorlarni chaqiruvchi tomon
// filtrlaydi.
func (c *Client) AttachEvents(ctx context.Context, onEvent func(StreamEvent)) error {
	// ⚠️ `[` va `]` xom holda qolishi kerak — %5B%5D ni qurilma qabul
	// qilmaydi (cgi() dagi bilan bir xil sabab).
	target := c.dev.baseURL() +
		"/cgi-bin/snapManager.cgi?action=attachFileProc&Flags[0]=Event" +
		"&Events=[AccessControl]&heartbeat=5"

	resp, err := c.digestStream(ctx, target)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("oqim ochilmadi: HTTP %d", resp.StatusCode)
	}

	reader := bufio.NewReaderSize(resp.Body, 64*1024)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		part, err := readPart(reader)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		// Heartbeat — ulanish tirikligini bildiradi, hodisa emas.
		if part == "" || strings.TrimSpace(part) == "Heartbeat" {
			continue
		}

		if ev, ok := parseStreamEvent(part); ok {
			onEvent(ev)
		}
	}
}

// readPart — multipart javobdan keyingi MATN qismini o'qiydi.
//
// JPEG qismlari o'tkazib yuboriladi: hozircha suratlar saqlanmaydi, ularni
// o'qish esa xotirani bekorga band qiladi (har hodisada 37–65 KB).
func readPart(r *bufio.Reader) (string, error) {
	var (
		contentType string
		length      = -1
	)

	// Sarlavhalar bo'sh qatorgacha.
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			switch strings.ToLower(strings.TrimSpace(k)) {
			case "content-type":
				contentType = strings.TrimSpace(v)
			case "content-length":
				length, _ = strconv.Atoi(strings.TrimSpace(v))
			}
		}
	}

	if length < 0 {
		return "", nil // ajratgich qatori — keyingisiga o'tamiz
	}

	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return "", err
	}

	if !strings.HasPrefix(contentType, "text/plain") {
		return "", nil // JPEG — tashlab yuboramiz
	}
	return string(body), nil
}

// parseStreamEvent — `Events[0].Xxx=qiymat` qatorlarini tahlil qiladi.
func parseStreamEvent(body string) (StreamEvent, bool) {
	fields := map[string]string{}

	for _, line := range strings.Split(body, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		// `Events[0].UserID` -> `UserID`
		if _, name, found := strings.Cut(k, "."); found {
			fields[name] = strings.TrimSpace(v)
		}
	}

	if fields["EventBaseInfo.Code"] != "" && fields["EventBaseInfo.Code"] != "AccessControl" {
		return StreamEvent{}, false
	}

	sec, err := strconv.ParseInt(fields["CreateTime"], 10, 64)
	if err != nil {
		return StreamEvent{}, false
	}

	atoi := func(s string) int {
		n, _ := strconv.Atoi(s)
		return n
	}

	return StreamEvent{
		UserID: fields["UserID"],
		// ⚠️ Faqat tanilganda to'ladi. Tanilmaganda `Status=0` va `UserID`
		// BO'SH bo'ladi.
		CardName:   fields["CardName"],
		HappenedAt: time.Unix(sec, 0).UTC(),
		Status:     atoi(fields["Status"]),
		ErrorCode:  atoi(fields["ErrorCode"]),
		Method:     atoi(fields["Method"]),
		Similarity: atoi(fields["Similarity"]),
		Alive:      atoi(fields["Alive"]),
	}, true
}
