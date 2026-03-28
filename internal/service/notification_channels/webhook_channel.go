package notification_channels

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// WebhookChannel sends notifications via HTTP POST with HMAC-SHA256 signature.
type WebhookChannel struct {
	client *http.Client
}

func NewWebhookChannel() *WebhookChannel {
	return &WebhookChannel{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// WebhookConfig holds the configuration for a webhook channel.
type WebhookConfig struct {
	URL     string            `json:"url"`
	Secret  string            `json:"secret"`
	Headers map[string]string `json:"headers"`
}

// Send delivers a webhook notification with HMAC-SHA256 signature.
func (ch *WebhookChannel) Send(ctx context.Context, config WebhookConfig, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ComplianceForge-Webhook/1.0")
	req.Header.Set("X-CF-Timestamp", time.Now().UTC().Format(time.RFC3339))

	// HMAC-SHA256 signature
	if config.Secret != "" {
		mac := hmac.New(sha256.New, []byte(config.Secret))
		mac.Write(body)
		signature := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-CF-Signature", signature)
	}

	// Custom headers
	for k, v := range config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := ch.client.Do(req)
	if err != nil {
		log.Error().Err(err).Str("url", config.URL).Msg("webhook_channel: request failed")
		return fmt.Errorf("webhook request to %s: %w", config.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		log.Error().Int("status", resp.StatusCode).Str("url", config.URL).Str("body", string(respBody)).Msg("webhook_channel: non-success response")
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	log.Info().Str("url", config.URL).Int("status", resp.StatusCode).Msg("webhook_channel: delivered")
	return nil
}
