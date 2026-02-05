package stub

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"karting-bot/internal/util"
)

// Stub provider:
// - CreatePayment: генерит ссылку /pay/stub?invoice=...
// - Webhook: POST /webhooks/stub с подписью X-Signature (HMAC SHA-256)

type Provider struct {
	secret  string
	baseURL string
}

func New(secret, baseURL string) *Provider {
	return &Provider{secret: secret, baseURL: strings.TrimRight(baseURL, "/")}
}

func (p *Provider) Name() string { return "stub" }

func (p *Provider) CreatePayment(ctx context.Context, stageID string, tgID int64, amount string, returnURL string) (string, string, error) {
	invoice := fmt.Sprintf("%s:%d:%s", stageID, tgID, util.NowISO())

	url := "/pay/stub?invoice=" + invoice
	if p.baseURL != "" {
		url = p.baseURL + url
	}
	return url, invoice, nil
}

type webhookPayload struct {
	Invoice string `json:"invoice"`
	Status  string `json:"status"` // paid/cancelled
}

func (p *Provider) HandleWebhook(ctx context.Context, body []byte, headers map[string]string) (stageID string, tgID int64, status string, err error) {
	sig := headers["x-signature"]
	expected := util.HMACSHA256Hex(p.secret, string(body))
	if sig == "" || sig != expected {
		return "", 0, "", fmt.Errorf("invalid signature")
	}

	var pl webhookPayload
	if err := json.Unmarshal(body, &pl); err != nil {
		return "", 0, "", err
	}

	parts := strings.Split(pl.Invoice, ":")
	if len(parts) < 2 {
		return "", 0, "", fmt.Errorf("bad invoice")
	}
	stageID = parts[0]

	var id int64
	_, _ = fmt.Sscan(parts[1], &id)
	tgID = id

	status = strings.TrimSpace(pl.Status)
	if status == "" {
		status = "paid"
	}
	return stageID, tgID, status, nil
}
