package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/qqgo/server/internal/config"
	"github.com/qqgo/server/internal/handler"
	"github.com/qqgo/server/internal/middleware"
	"github.com/qqgo/server/internal/service"
	"github.com/qqgo/server/internal/store"
)

func main() {
	cfg := config.Load()

	db, err := store.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("database init error: %v", err)
	}
	log.Printf("database initialized at %s", cfg.DBPath)

	svc := service.NewChatService(db)
	rl := middleware.NewRateLimiter(cfg.MsgRateLimit)
	hub := handler.NewHub(svc, nil, cfg.Server.MaxConnections, rl)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.ServeWS)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		hub.Shutdown()

		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}

		sqlDB, err := db.DB()
		if err == nil {
			sqlDB.Close()
		}

		log.Println("server exited")
	}()

	if cfg.Server.TLSCert != "" && cfg.Server.TLSKey != "" {
		log.Printf("QQGO server starting (wss) on %s", addr)
		if err := srv.ListenAndServeTLS(cfg.Server.TLSCert, cfg.Server.TLSKey); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	} else {
		log.Printf("QQGO server starting (ws) on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}
}
