package push

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

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

	return &PushManager{
		VapidPublicKey:  publicKey,
		VapidPrivateKey: privateKey,
		ContactEmail:    "mailto:admin@screentime.local",
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
				Subscriber:      p.ContactEmail,
				VAPIDPublicKey:  p.VapidPublicKey,
				VAPIDPrivateKey: p.VapidPrivateKey,
				TTL:             180, // 3 minutes
			})
			if err != nil {
				log.Printf("[Push] Error sending push: %v", err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
				// Expired subscription, clean up
				p.db.Exec("DELETE FROM push_subscriptions WHERE endpoint = ?", sub.Endpoint)
			}
		}(s)
	}
}

