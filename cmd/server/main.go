package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/qqgo/server/internal/config"
	"github.com/qqgo/server/internal/handler"
	"github.com/qqgo/server/internal/middleware"
	"github.com/qqgo/server/internal/service"
	"github.com/qqgo/server/internal/store"
)

func main() {
	cfg := config.Load()
	service.InitJWT(cfg.JWT)

	instanceID := fmt.Sprintf("server-%s", generateShortID())

	var rdb *redis.Client
	var onlineTracker *middleware.OnlineTracker
	var pubsubRouter *middleware.PubSubRouter

	if cfg.RedisEnabled {
		rdb = redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})
		if err := rdb.Ping(context.Background()).Err(); err != nil {
			log.Printf("[redis] connection failed: %v, running in single-instance mode", err)
			rdb = nil
		} else {
			log.Printf("[redis] connected to %s, instance: %s", cfg.Redis.Addr, instanceID)
			onlineTracker = middleware.NewOnlineTracker(rdb, instanceID)
		}
	}

	db, err := store.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("database init error: %v", err)
	}
	log.Printf("database initialized at %s", cfg.DBPath)

	svc := service.NewChatService(db)
	rl := middleware.NewRateLimiter(cfg.MsgRateLimit)
	hub := handler.NewHub(svc, nil, cfg.Server.MaxConnections, rl, onlineTracker, pubsubRouter, instanceID)

	if rdb != nil {
		pubsubRouter = middleware.NewPubSubRouter(rdb, instanceID, hub.HandlePubSubMessage)
		pubsubRouter.Start()
		hub.SetPubSubRouter(pubsubRouter)
	}

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

		if pubsubRouter != nil {
			pubsubRouter.Stop()
		}

		hub.Shutdown()

		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}

		if rdb != nil {
			rdb.Close()
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

func generateShortID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}
