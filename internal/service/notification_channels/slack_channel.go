package notification_channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// SlackChannel sends notifications via Slack webhook API.
type SlackChannel struct {
	client *http.Client
}

func NewSlackChannel() *SlackChannel {
	return &SlackChannel{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// SlackConfig holds Slack channel configuration.
type SlackConfig struct {
	WebhookURL string `json:"webhook_url"`
	Channel    string `json:"channel"`
}

// SlackMessage represents a Slack message payload.
type SlackMessage struct {
	Channel string       `json:"channel,omitempty"`
	Text    string       `json:"text"`
	Blocks  []SlackBlock `json:"blocks,omitempty"`
}

// SlackBlock represents a Slack Block Kit block.
type SlackBlock struct {
	Type string     `json:"type"`
	Text *SlackText `json:"text,omitempty"`
}

type SlackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Send delivers a notification to Slack via webhook.
func (ch *SlackChannel) Send(ctx context.Context, config SlackConfig, subject, body string) error {
	msg := SlackMessage{
		Channel: config.Channel,
		Text:    subject,
		Blocks: []SlackBlock{
			{
				Type: "header",
				Text: &SlackText{Type: "plain_text", Text: subject},
			},
			{
				Type: "section",
				Text: &SlackText{Type: "mrkdwn", Text: body},
			},
		},
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshalling slack message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ch.client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("slack_channel: request failed")
		return fmt.Errorf("slack webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		log.Error().Int("status", resp.StatusCode).Str("body", string(respBody)).Msg("slack_channel: non-OK response")
		return fmt.Errorf("slack returned status %d: %s", resp.StatusCode, string(respBody))
	}

	log.Info().Str("channel", config.Channel).Msg("slack_channel: delivered")
	return nil
}
