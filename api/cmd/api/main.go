// Mini-DSS API — Dahua yuz tanish terminallarini boshqarish.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"minidss/api/internal/auth"
	"minidss/api/internal/config"
	"minidss/api/internal/httpapi"
	"minidss/api/internal/store"
	"minidss/api/internal/syncsvc"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("konfiguratsiya xato: %v", err)
	}

	if err := os.MkdirAll(cfg.PhotoDir, 0o755); err != nil {
		log.Fatalf("rasm katalogi yaratilmadi: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	st, err := store.New(ctx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		log.Fatalf("bazaga ulanib bo'lmadi: %v", err)
	}
	defer st.Close()

	authSvc, err := auth.New(cfg.JWTSecret, cfg.JWTTTL, cfg.AdminUser, cfg.AdminPassword, cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("auth xato: %v", err)
	}

	syncSvc := syncsvc.New(st, cfg.PhotoDir, authSvc.Decrypt, cfg.Timezone, cfg.PhotoAllowedHosts)
	api := httpapi.New(cfg, st, authSvc, syncSvc)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		// Rasm yuklash va uzoq sync so'rovlari uchun kengroq.
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		for {
			if n, err := st.ExpirePeople(context.Background()); err != nil {
				log.Printf("muddat tekshiruvi xato: %v", err)
			} else if n > 0 {
				log.Printf("muddati o'tgan %d ta odam faolsizlantirildi", n)
			}
			time.Sleep(time.Hour)
		}
	}()

	// Terminallardan hodisalarni jonli olish. Har bir qurilma uchun alohida
	// oqim ochiladi va u yozish amallari vaqtida vaqtincha yopiladi.
	listenerCtx, stopListeners := context.WithCancel(context.Background())
	defer stopListeners()

	if cfg.LiveEvents {
		go syncSvc.StartListeners(listenerCtx)
		log.Println("jonli hodisa oqimi yoqilgan")
	} else {
		// Oqim o'chirilgan bo'lsa jurnalni davriy yig'amiz — hech
		// bo'lmasa hisobot yangilanib turadi.
		go syncSvc.PollEvents(listenerCtx, time.Minute)
		log.Println("jonli oqim o'chirilgan — jurnal har daqiqada yig'iladi")
	}

	go func() {
		log.Printf("Mini-DSS API :%s da ishga tushdi (zona: %s)", cfg.Port, cfg.Timezone)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server xato: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("to'xtatilmoqda...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown xato: %v", err)
	}
}
