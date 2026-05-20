package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/qqgo/server/internal/config"
	"github.com/qqgo/server/internal/handler"
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
	hub := handler.NewHub(svc, nil)

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
		srv.Close()
	}()

	log.Printf("QQGO server starting on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
