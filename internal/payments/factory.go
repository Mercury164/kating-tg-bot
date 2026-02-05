package payments

import (
    "fmt"

    "karting-bot/internal/config"
    "karting-bot/internal/payments/stub"
)

func NewProvider(cfg config.Config) (PaymentProvider, error) {
    switch cfg.PaymentProvider {
    case "stub":
        return stub.New(cfg.PaymentWebhookSecret, cfg.BasePublicURL), nil
    default:
        return nil, fmt.Errorf("unknown payment provider: %s", cfg.PaymentProvider)
    }
}
