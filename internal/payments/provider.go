package payments

import "context"

type PaymentProvider interface {
	Name() string

	// Возвращает ссылку на оплату и invoice
	CreatePayment(ctx context.Context, stageID string, tgID int64, amount string, returnURL string) (payURL string, invoice string, err error)

	// Валидирует вебхук и возвращает (stageID, tgID, status=paid/cancelled)
	HandleWebhook(ctx context.Context, body []byte, headers map[string]string) (stageID string, tgID int64, status string, err error)
}
