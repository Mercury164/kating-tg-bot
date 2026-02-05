package config

import (
    "fmt"
    "os"
    "strconv"
    "strings"
)

type Config struct {
    TelegramToken string

    SpreadsheetID            string
    GoogleServiceAccountJSON string

    AdminTGIDs map[int64]bool

    PaymentProvider     string
    PaymentWebhookSecret string

    HTTPAddr      string
    BasePublicURL string
}

func FromEnv() (Config, error) {
    var c Config
    c.TelegramToken = strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
    c.SpreadsheetID = strings.TrimSpace(os.Getenv("GOOGLE_SHEETS_SPREADSHEET_ID"))
    c.GoogleServiceAccountJSON = strings.TrimSpace(os.Getenv("GOOGLE_SERVICE_ACCOUNT_JSON"))

    c.PaymentProvider = strings.TrimSpace(os.Getenv("PAYMENT_PROVIDER"))
    if c.PaymentProvider == "" {
        c.PaymentProvider = "stub"
    }
    c.PaymentWebhookSecret = strings.TrimSpace(os.Getenv("PAYMENT_WEBHOOK_SECRET"))
    if c.PaymentWebhookSecret == "" {
        c.PaymentWebhookSecret = "change-me"
    }

    c.HTTPAddr = strings.TrimSpace(os.Getenv("HTTP_ADDR"))
    if c.HTTPAddr == "" {
        c.HTTPAddr = ":8080"
    }

    c.BasePublicURL = strings.TrimRight(strings.TrimSpace(os.Getenv("BASE_PUBLIC_URL")), "/")

    if c.TelegramToken == "" {
        return c, fmt.Errorf("TELEGRAM_BOT_TOKEN is empty")
    }
    if c.SpreadsheetID == "" {
        return c, fmt.Errorf("GOOGLE_SHEETS_SPREADSHEET_ID is empty")
    }
    if c.GoogleServiceAccountJSON == "" {
        return c, fmt.Errorf("GOOGLE_SERVICE_ACCOUNT_JSON is empty")
    }

    c.AdminTGIDs = parseAdminIDs(os.Getenv("ADMIN_TG_IDS"))

    return c, nil
}

func parseAdminIDs(raw string) map[int64]bool {
    m := map[int64]bool{}
    raw = strings.TrimSpace(raw)
    if raw == "" {
        return m
    }
    parts := strings.Split(raw, ",")
    for _, p := range parts {
        p = strings.TrimSpace(p)
        if p == "" {
            continue
        }
        v, err := strconv.ParseInt(p, 10, 64)
        if err != nil {
            continue
        }
        m[v] = true
    }
    return m
}
