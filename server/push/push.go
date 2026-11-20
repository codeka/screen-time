package push

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type PushManager struct {
	VapidPublicKey  string
	VapidPrivateKey string
	ContactEmail    string
	db              *sql.DB
}

func NewPushManager(db *sql.DB) *PushManager {
	var publicKey, privateKey string
	_ = db.QueryRow("SELECT value FROM server_settings WHERE key = 'vapid_public_key'").Scan(&publicKey)
	_ = db.QueryRow("SELECT value FROM server_settings WHERE key = 'vapid_private_key'").Scan(&privateKey)

	if publicKey == "" || privateKey == "" {
		var err error
		privateKey, publicKey, err = webpush.GenerateVAPIDKeys()
		if err != nil {
			log.Printf("Warning: failed to generate VAPID keys: %v", err)
		} else {
			_, _ = db.Exec("INSERT OR REPLACE INTO server_settings (key, value) VALUES ('vapid_public_key', ?), ('vapid_private_key', ?)", publicKey, privateKey)
		}
	}

	contact := os.Getenv("VAPID_CONTACT")
	if contact == "" {
		contact = "admin@codeka.com"
	}

	return &PushManager{
		VapidPublicKey:  publicKey,
		VapidPrivateKey: privateKey,
		ContactEmail:    contact,
		db:              db,
	}
}

type PushPayload struct {
	Title string                 `json:"title"`
	Body  string                 `json:"body"`
	Icon  string                 `json:"icon,omitempty"`
	Tag   string                 `json:"tag,omitempty"`
	Data  map[string]interface{} `json:"data,omitempty"`
}

func (p *PushManager) SendToFamily(familyID string, payload PushPayload) {
	rows, err := p.db.Query(`
		SELECT ps.endpoint, ps.p256dh, ps.auth
		FROM push_subscriptions ps
		JOIN family_users fu ON ps.user_id = fu.id
		WHERE fu.family_id = ?
	`, familyID)
	if err != nil {
		log.Printf("[Push] Failed to query subscriptions: %v", err)
		return
	}
	defer rows.Close()

	payloadBytes, _ := json.Marshal(payload)

	subscriber := p.ContactEmail
	if !strings.HasPrefix(subscriber, "https:") {
		// webpush-go prepends "mailto:" automatically for non-https subscribers.
		subscriber = strings.TrimPrefix(subscriber, "mailto:")
	}

	for rows.Next() {
		var endpoint, p256dh, auth string
		if err := rows.Scan(&endpoint, &p256dh, &auth); err != nil {
			continue
		}

		s := &webpush.Subscription{
			Endpoint: endpoint,
			Keys: webpush.Keys{
				P256dh: p256dh,
				Auth:   auth,
			},
		}

		go func(sub *webpush.Subscription) {
			resp, err := webpush.SendNotification(payloadBytes, sub, &webpush.Options{
				Subscriber:      subscriber,
				VAPIDPublicKey:  p.VapidPublicKey,
				VAPIDPrivateKey: p.VapidPrivateKey,
				TTL:             1800,                // 30 minutes so alerts survive transient connectivity gaps
				Urgency:         webpush.UrgencyHigh, // Wake Android devices in Doze immediately
			})
			if err != nil {
				log.Printf("[Push] Error sending push: %v", err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusGone ||
				resp.StatusCode == http.StatusNotFound ||
				resp.StatusCode == http.StatusUnauthorized ||
				resp.StatusCode == http.StatusForbidden {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
				log.Printf("[Push] Removing invalid/expired subscription (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
				_, _ = p.db.Exec("DELETE FROM push_subscriptions WHERE endpoint = ?", sub.Endpoint)
			} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
				log.Printf("[Push] Push delivery returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			}
		}(s)
	}
}
