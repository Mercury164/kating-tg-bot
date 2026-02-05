package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/joho/godotenv"

    "karting-bot/internal/config"
    "karting-bot/internal/payments"
    "karting-bot/internal/server"
    "karting-bot/internal/sheets"
    "karting-bot/internal/tgbot"
)

func main() {
    _ = godotenv.Load()

    cfg, err := config.FromEnv()
    if err != nil {
        log.Fatalf("config: %v", err)
    }

    sheetsClient, err := sheets.New(cfg.GoogleServiceAccountJSON, cfg.SpreadsheetID)
    if err != nil {
        log.Fatalf("sheets: %v", err)
    }

    payProvider, err := payments.NewProvider(cfg)
    if err != nil {
        log.Fatalf("payments: %v", err)
    }

    botApp, err := tgbot.New(cfg, sheetsClient, payProvider)
    if err != nil {
        log.Fatalf("telegram: %v", err)
    }

    httpSrv := server.New(cfg, sheetsClient, payProvider, botApp)

    // Start HTTP server
    go func() {
        log.Printf("HTTP listening on %s", cfg.HTTPAddr)
        if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("http server: %v", err)
        }
    }()

    // Start Telegram
    ctx, cancel := context.WithCancel(context.Background())
    go func() {
        if err := botApp.Run(ctx); err != nil {
            log.Printf("bot stopped: %v", err)
            cancel()
        }
    }()

    // Graceful shutdown
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
    <-sig
    log.Println("shutting down...")

    cancel()
    ctxTimeout, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel2()
    _ = httpSrv.Shutdown(ctxTimeout)

    log.Println("bye")
}
