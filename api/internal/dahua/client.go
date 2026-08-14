// Package dahua — Dahua ASI7xxx terminallari bilan ishlash.
//
// Bu paketdagi har bir qaror jonli qurilmada (DHI-ASI7213Y-V3,
// fw 1.000.0000014.0.R) o'lchangan. Tafsilot: device-probe/API-MATRIX.md
//
// Ikki qatlam ishlatiladi:
//
//	RPC2  — foydalanuvchi yozish. Sessiya bilan, 42 ms/yozuv.
//	CGI   — o'qish va yuz yuklash. HTTP Digest bilan, 565 ms/yozuv.
//
// ⚠️ Nega RPC2 13 baravar tez: HTTP Digest HAR SO'ROVDA ikki marta boradi
// (401 challenge + autentifikatsiyalangan so'rov). RPC2 bir marta login
// qilib, sessiya tokeni bilan bitta borish-kelishda ishlaydi. Dahua'ning
// o'z DSS dasturi ham shu sababli tez.
package dahua

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Device — terminalga ulanish ma'lumotlari.
type Device struct {
	IP       string
	Port     int
	Username string
	Password string
}

func (d Device) baseURL() string {
	port := d.Port
	if port == 0 {
		port = 80
	}
	return fmt.Sprintf("http://%s:%d", d.IP, port)
}

// Client — bitta qurilma uchun HTTP klienti.
//
// ⚠️ ULANISH QAYTA ISHLATILADI. Har so'rovga yangi TCP ulanish ochilganda
// qurilma ~90–120 ta ketma-ket yozuv so'rovidan keyin javob bermay qo'yadi —
// soketlari tugaydi. Sur'atni pasaytirish yordam bermaydi (300 ms da ham,
// 700 ms da ham shu chegarada to'xtaydi).
//
// Bitta ulanish qayta ishlatilganda 200 ta ketma-ket insert bitta ham xatosiz
// o'tdi. Shuning uchun Transport bir marta yaratiladi va saqlanadi.
//
// ⚠️ Qurilma bir vaqtda JUDA KAM ulanish qabul qiladi — bir sikl ishlab
// turganda boshqa ulanish "connection refused" oladi. Bitta qurilmaga bir
// vaqtda bitta jarayon ishlasin.
type Client struct {
	dev  Device
	http *http.Client

	// RPC2 sessiyasi
	session    string
	sessionGen int
	requestID  int
}

func New(dev Device, timeout time.Duration) *Client {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		dev: dev,
		http: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				// Ulanishlarni ochiq ushlab turamiz — yuqoridagi izohga qarang.
				MaxIdleConns:        4,
				MaxIdleConnsPerHost: 4,
				MaxConnsPerHost:     2,
				IdleConnTimeout:     90 * time.Second,
				DisableKeepAlives:   false,
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
			},
		},
		requestID: 10000,
	}
}

func (c *Client) Close() {
	c.http.CloseIdleConnections()
}

// md5Hex — Dahua KATTA HARFLI hex kutadi.
func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// Ping — qurilma javob berayotganini tekshiradi.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cgi(ctx, "magicBox.cgi", map[string]string{"action": "getDeviceType"})
	return err
}

// Identity — model, firmware, seriya raqami.
type Identity struct {
	Model    string `json:"model"`
	Firmware string `json:"firmware"`
	Serial   string `json:"serial"`
}

func (c *Client) Identify(ctx context.Context) (Identity, error) {
	var id Identity

	body, err := c.cgi(ctx, "magicBox.cgi", map[string]string{"action": "getDeviceType"})
	if err != nil {
		return id, err
	}
	id.Model = ParseValue(body, "type")

	if body, err = c.cgi(ctx, "magicBox.cgi", map[string]string{"action": "getSoftwareVersion"}); err == nil {
		id.Firmware = ParseValue(body, "version")
	}
	if body, err = c.cgi(ctx, "magicBox.cgi", map[string]string{"action": "getSerialNo"}); err == nil {
		id.Serial = ParseValue(body, "sn")
	}

	return id, nil
}

// DeviceTime — qurilma vaqti.
//
// ⚠️ Bu LOCAL vaqt (qurilma TimeZone sozlamasiga ko'ra), UTC EMAS.
// Loglardagi CreateTime esa aksincha — epoch UTC. Ikkisini aralashtirmang:
// farqi 5 soat (Toshkent).
func (c *Client) DeviceTime(ctx context.Context) (string, error) {
	body, err := c.cgi(ctx, "global.cgi", map[string]string{"action": "getCurrentTime"})
	if err != nil {
		return "", err
	}
	return ParseValue(body, "result"), nil
}

// TimeDrift — qurilma vaqti server vaqtidan necha soniya farq qiladi.
// Musbat = qurilma oldinda. NTP o'chiq bo'lsa farq o'sib boradi.
func (c *Client) TimeDrift(ctx context.Context, loc *time.Location) (int, error) {
	raw, err := c.DeviceTime(ctx)
	if err != nil {
		return 0, err
	}

	t, err := time.ParseInLocation("2006-01-02 15:04:05", raw, loc)
	if err != nil {
		return 0, fmt.Errorf("qurilma vaqtini o'qib bo'lmadi (%q): %w", raw, err)
	}

	return int(t.Unix() - time.Now().Unix()), nil
}
