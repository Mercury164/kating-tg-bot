package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"karting-bot/internal/config"
	"karting-bot/internal/payments"
	"karting-bot/internal/sheets"
	"karting-bot/internal/tgbot"
	"karting-bot/internal/util"
)

func New(cfg config.Config, sh *sheets.Client, pay payments.PaymentProvider, bot *tgbot.App) *http.Server {
	mux := http.NewServeMux()

	// Stub payment page (for testing)
	mux.HandleFunc("/pay/stub", func(w http.ResponseWriter, r *http.Request) {
		invoice := r.URL.Query().Get("invoice")
		if invoice == "" {
			http.Error(w, "invoice required", http.StatusBadRequest)
			return
		}
		// Show simple HTML with a "Pay" button that triggers webhook.
		// In production, real provider would host checkout.
		html := `<!doctype html><html><head><meta charset="utf-8"><title>Stub Pay</title></head><body>
<h2>Оплата (тестовый провайдер)</h2>
<p>Invoice: ` + invoice + `</p>
<button onclick="pay()">Оплатить (paid)</button>
<button onclick="cancelPay()">Отменить (cancelled)</button>
<pre id="out"></pre>
<script>
async function send(status){
  const body = JSON.stringify({invoice: "` + invoice + `", status});
  const sig = await hmac(body);
  const res = await fetch("/webhooks/stub", {method:"POST", headers: {"Content-Type":"application/json","X-Signature": sig}, body});
  document.getElementById("out").textContent = await res.text();
}
function pay(){ send("paid"); }
function cancelPay(){ send("cancelled"); }

// Minimal HMAC SHA-256 in browser is non-trivial without libs.
// For simplicity, signature is not generated here; server accepts missing signature only if running locally.
// Use real provider in prod.
async function hmac(body){ return ""; }
</script>
</body></html>`
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	})

	// Payment webhooks
	mux.HandleFunc("/webhooks/stub", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		headers := map[string]string{}
		for k, v := range r.Header {
			if len(v) > 0 {
				headers[strings.ToLower(k)] = v[0]
			}
		}

		// DEV: если подпись не пришла (stub-страница), досчитаем её на сервере
		if headers["x-signature"] == "" && (cfg.BasePublicURL == "" || strings.Contains(cfg.BasePublicURL, "localhost")) {
			headers["x-signature"] = util.HMACSHA256Hex(cfg.PaymentWebhookSecret, string(body))
		}

		stageID, tgID, status, err := pay.HandleWebhook(r.Context(), body, headers)

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Map to pay_status values
		payStatus := "paid"
		if status == "cancelled" {
			payStatus = "cancelled"
		}

		if err := sh.UpdatePayStatus(stageID, tgID, payStatus); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Notify user in Telegram
		go func() {
			msg := "✅ Оплата подтверждена. Участие в этапе закреплено."
			if payStatus == "cancelled" {
				msg = "❌ Оплата отменена."
			}
			if err := bot.SendText(tgID, msg); err != nil {
				log.Printf("notify user: %v", err)
			}
		}()

		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         true,
			"stage_id":   stageID,
			"tg_id":      tgID,
			"pay_status": payStatus,
			"ts":         util.NowISO(),
		})
	})

	// CSV export (admin-only link with token = HMAC)
	mux.HandleFunc("/export/stage.csv", func(w http.ResponseWriter, r *http.Request) {
		stageID := r.URL.Query().Get("stage_id")
		token := r.URL.Query().Get("token")
		if stageID == "" || token == "" {
			http.Error(w, "stage_id and token required", http.StatusBadRequest)
			return
		}
		expected := util.HMACSHA256Hex(cfg.PaymentWebhookSecret, "export:"+stageID)
		if token != expected {
			http.Error(w, "invalid token", http.StatusForbidden)
			return
		}
		csv, err := bot.BuildStageCSV(r.Context(), stageID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="stage_`+stageID+`.csv"`)
		_, _ = w.Write([]byte(csv))
	})

	return &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: mux,
	}
}
