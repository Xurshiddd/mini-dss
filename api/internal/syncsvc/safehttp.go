package syncsvc

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// safeHTTPClient — SSRF'ga qarshi himoyalangan HTTP klient.
//
// ⚠️ Rasmlar tashqi manbadan (HEMIS) foydalanuvchi bergan URL bo'yicha
// olinadi. Tekshiruvsiz bu — server orqali ichki tarmoqni skanerlash yoki
// bulut metama'lumot xizmatiga (169.254.169.254) yetish yo'li.
//
// Himoya DIAL darajasida: DNS nomi resolve qilingandan KEYINGI haqiqiy IP
// tekshiriladi. Bu DNS rebinding'ni ham to'xtatadi — nom ochiq IP'ga, keyin
// lokal IP'ga o'zgarsa ham, ulanish paytidagi IP bloklanadi.
//
// ⚠️ `allowedHosts` — ishonchli manba hostlari (masalan HEMIS rasm serveri).
// Ular ICHKI tarmoqda turishi mumkin: hemis.ttyesi.uz -> 172.16.0.253. Bu
// ro'yxatsiz xususiy IP bloki ularni ham kesadi va HAMMA rasm rad etiladi.
// Ro'yxat audit tavsiyasiga mos (TASK-02 2b): ixtiyoriy `photo_url` baribir
// bloklanadi, faqat aniq ko'rsatilgan host'gagina ruxsat beriladi.
func safeHTTPClient(timeout time.Duration, allowedHosts []string) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	allowed := make(map[string]bool, len(allowedHosts))
	for _, h := range allowedHosts {
		allowed[strings.ToLower(h)] = true
	}

	return &http.Client{
		Timeout: timeout,
		// ⚠️ Redirect'ni ham tekshiramiz: ochiq URL lokal manzilga
		// yo'naltirishi mumkin.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("juda ko'p redirect")
			}
			return nil
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}

				ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
				if err != nil {
					return nil, err
				}

				// Ishonchli host — ichki IP tekshiruvidan o'tkazamiz.
				// Redirect'da host o'zgarsa, bu dial qaytadan chaqiriladi
				// va yangi host ro'yxatda bo'lmasa baribir bloklanadi.
				if !allowed[strings.ToLower(host)] {
					for _, ip := range ips {
						if isBlockedIP(ip.IP) {
							return nil, fmt.Errorf("manzil bloklangan (ichki/lokal IP): %s", ip.IP)
						}
					}
				}

				// Resolve qilingan (tekshirilgan) IP'ga bevosita ulanamiz —
				// dial vaqtida qayta resolve qilib rebinding bo'lmasligi uchun.
				return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
			},
		},
	}
}

// isBlockedIP — xususiy, lokal yoki maxsus IP oralig'imi.
func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}

	// Bulut metama'lumot xizmati (169.254.169.254) IsLinkLocalUnicast
	// bilan qamraladi, lekin aniqlik uchun tekshiramiz.
	if ip.Equal(net.IPv4(169, 254, 169, 254)) {
		return true
	}

	return false
}
