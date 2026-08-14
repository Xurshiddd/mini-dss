package dahua

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
)

// HTTP Digest autentifikatsiyasi.
//
// Go standart kutubxonasida Digest yo'q — challenge-response qo'lda bajariladi.
//
// ⚠️ Digest HAR SO'ROVDA IKKI MARTA boradi: birinchi so'rov 401 va challenge
// oladi, ikkinchisi autentifikatsiya bilan ketadi. Aynan shu sababli CGI
// yozish 565 ms, RPC2 esa 42 ms (u sessiya tokeni bilan bir marta boradi).
//
// Challenge'ni keshlaymiz: nonce yaroqli ekan qayta ishlatiladi va so'rovlar
// soni ikki barobar kamayadi.

type digestChallenge struct {
	realm     string
	nonce     string
	qop       string
	opaque    string
	algorithm string
	nc        int
}

var (
	digestCache   = map[string]*digestChallenge{}
	digestCacheMu sync.Mutex
	digestParamRe = regexp.MustCompile(`(\w+)="?([^",]+)"?`)
)

func parseChallenge(header string) *digestChallenge {
	ch := &digestChallenge{algorithm: "MD5"}
	for _, m := range digestParamRe.FindAllStringSubmatch(header, -1) {
		switch strings.ToLower(m[1]) {
		case "realm":
			ch.realm = m[2]
		case "nonce":
			ch.nonce = m[2]
		case "qop":
			ch.qop = m[2]
		case "opaque":
			ch.opaque = m[2]
		case "algorithm":
			ch.algorithm = m[2]
		}
	}
	if ch.realm == "" || ch.nonce == "" {
		return nil
	}
	return ch
}

func cnonce() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (ch *digestChallenge) authorization(user, pass, method, uri string) string {
	ha1 := strings.ToLower(hex.EncodeToString(md5Sum(user + ":" + ch.realm + ":" + pass)))
	ha2 := strings.ToLower(hex.EncodeToString(md5Sum(method + ":" + uri)))

	if !strings.Contains(ch.qop, "auth") {
		resp := strings.ToLower(hex.EncodeToString(md5Sum(ha1 + ":" + ch.nonce + ":" + ha2)))
		return fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
			user, ch.realm, ch.nonce, uri, resp)
	}

	ch.nc++
	nc := fmt.Sprintf("%08x", ch.nc)
	cn := cnonce()
	resp := strings.ToLower(hex.EncodeToString(
		md5Sum(ha1 + ":" + ch.nonce + ":" + nc + ":" + cn + ":auth:" + ha2)))

	auth := fmt.Sprintf(
		`Digest username="%s", realm="%s", nonce="%s", uri="%s", qop=auth, nc=%s, cnonce="%s", response="%s"`,
		user, ch.realm, ch.nonce, uri, nc, cn, resp)

	if ch.opaque != "" {
		auth += fmt.Sprintf(`, opaque="%s"`, ch.opaque)
	}
	return auth
}

func (c *Client) digestGet(ctx context.Context, target string) (string, int, error) {
	return c.digestDo(ctx, http.MethodGet, target, "", nil)
}

func (c *Client) digestPost(ctx context.Context, target, contentType string, body []byte) (string, int, error) {
	return c.digestDo(ctx, http.MethodPost, target, contentType, body)
}

// digestStream — javob TANASINI OCHIQ qoldiradi.
//
// Uzoq yashovchi multipart oqim uchun (`AttachEvents`): `digestDo` tanani
// oxirigacha o'qib yopadi, oqimda esa u hech qachon tugamaydi.
// Chaqiruvchi `resp.Body` ni o'zi yopishi shart.
func (c *Client) digestStream(ctx context.Context, target string) (*http.Response, error) {
	uri := target
	if i := strings.Index(target, "://"); i >= 0 {
		if j := strings.Index(target[i+3:], "/"); j >= 0 {
			uri = target[i+3+j:]
		}
	}

	newRequest := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Connection", "keep-alive")
		return req, nil
	}

	// Challenge olamiz.
	req, err := newRequest()
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil // qurilma auth so'ramadi
	}

	header := resp.Header.Get("WWW-Authenticate")
	_, _ = drain(resp)

	ch := parseChallenge(header)
	if ch == nil {
		return nil, fmt.Errorf("digest challenge o'qilmadi: %q", header)
	}

	digestCacheMu.Lock()
	digestCache[c.dev.IP+"|"+c.dev.Username] = ch
	digestCacheMu.Unlock()

	req2, err := newRequest()
	if err != nil {
		return nil, err
	}
	req2.Header.Set("Authorization",
		ch.authorization(c.dev.Username, c.dev.Password, http.MethodGet, uri))

	return c.http.Do(req2)
}

func (c *Client) digestDo(ctx context.Context, method, target, contentType string, body []byte) (string, int, error) {
	uri := target
	if i := strings.Index(target, "://"); i >= 0 {
		if j := strings.Index(target[i+3:], "/"); j >= 0 {
			uri = target[i+3+j:]
		}
	}

	cacheKey := c.dev.IP + "|" + c.dev.Username

	newRequest := func() (*http.Request, error) {
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, target, rdr)
		if err != nil {
			return nil, err
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		req.Header.Set("Connection", "keep-alive")
		return req, nil
	}

	// 1-urinish: keshdagi challenge bilan (agar bo'lsa) — bir borish-kelish tejaladi.
	digestCacheMu.Lock()
	ch := digestCache[cacheKey]
	digestCacheMu.Unlock()

	if ch != nil {
		req, err := newRequest()
		if err != nil {
			return "", 0, err
		}
		req.Header.Set("Authorization", ch.authorization(c.dev.Username, c.dev.Password, method, uri))

		resp, err := c.http.Do(req)
		if err != nil {
			return "", 0, err
		}
		if resp.StatusCode != http.StatusUnauthorized {
			out, err := drain(resp)
			return out, resp.StatusCode, err
		}
		// Nonce eskirdi — quyida yangisini olamiz.
		_, _ = drain(resp)
	}

	// 2-urinish: challenge olamiz.
	req, err := newRequest()
	if err != nil {
		return "", 0, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, err
	}

	if resp.StatusCode != http.StatusUnauthorized {
		out, err := drain(resp)
		return out, resp.StatusCode, err
	}

	header := resp.Header.Get("WWW-Authenticate")
	// ⚠️ Tanani oxirigacha o'qish SHART — aks holda ulanish qayta
	// ishlatilmaydi va qurilmada soket tugashi muammosi qaytadi.
	_, _ = drain(resp)

	ch = parseChallenge(header)
	if ch == nil {
		return "", resp.StatusCode, fmt.Errorf("digest challenge o'qilmadi: %q", header)
	}

	digestCacheMu.Lock()
	digestCache[cacheKey] = ch
	digestCacheMu.Unlock()

	req2, err := newRequest()
	if err != nil {
		return "", 0, err
	}
	req2.Header.Set("Authorization", ch.authorization(c.dev.Username, c.dev.Password, method, uri))

	resp2, err := c.http.Do(req2)
	if err != nil {
		return "", 0, err
	}
	out, err := drain(resp2)
	return out, resp2.StatusCode, err
}
