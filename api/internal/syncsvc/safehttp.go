package syncsvc

import (
	"context"
	"fmt"
	"net"
	"net/http"
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
func safeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}

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

				for _, ip := range ips {
					if isBlockedIP(ip.IP) {
						return nil, fmt.Errorf("manzil bloklangan (ichki/lokal IP): %s", ip.IP)
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
