package service

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

func TestNotificationScheduledForDigestAndQuietHours(t *testing.T) {
	lagos := "Africa/Lagos"
	start, end := "22:00", "07:00"
	tests := []struct {
		name       string
		now        string
		preference notificationPreferenceDecision
		want       string
	}{
		{
			name: "immediate outside quiet hours",
			now:  "2026-09-14T09:15:00+01:00",
			preference: notificationPreferenceDecision{
				Enabled: true, DigestFrequency: "immediate", QuietHoursTimezone: &lagos,
			},
			want: "2026-09-14T08:15:00Z",
		},
		{
			name: "overnight quiet hours defer until local end",
			now:  "2026-09-14T23:15:00+01:00",
			preference: notificationPreferenceDecision{
				Enabled: true, DigestFrequency: "immediate", QuietHoursStart: &start,
				QuietHoursEnd: &end, QuietHoursTimezone: &lagos,
			},
			want: "2026-09-15T06:00:00Z",
		},
		{
			name: "hourly digest uses next local hour then quiet end",
			now:  "2026-09-14T21:45:00+01:00",
			preference: notificationPreferenceDecision{
				Enabled: true, DigestFrequency: "hourly", QuietHoursStart: &start,
				QuietHoursEnd: &end, QuietHoursTimezone: &lagos,
			},
			want: "2026-09-15T06:00:00Z",
		},
		{
			name: "daily digest uses eight local",
			now:  "2026-09-14T09:00:00+01:00",
			preference: notificationPreferenceDecision{
				Enabled: true, DigestFrequency: "daily", QuietHoursTimezone: &lagos,
			},
			want: "2026-09-15T07:00:00Z",
		},
		{
			name: "weekly digest uses next Monday",
			now:  "2026-09-14T09:00:00+01:00",
			preference: notificationPreferenceDecision{
				Enabled: true, DigestFrequency: "weekly", QuietHoursTimezone: &lagos,
			},
			want: "2026-09-21T07:00:00Z",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, test.now)
			if err != nil {
				t.Fatal(err)
			}
			if got := notificationScheduledFor(now, test.preference).Format(time.RFC3339); got != test.want {
				t.Fatalf("notificationScheduledFor() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestNotificationRetryDelayIsExponentialBoundedAndStable(t *testing.T) {
	config := NotificationDeliveryConfig{RetryBaseDelay: 10 * time.Second, RetryMaxDelay: time.Minute}
	id := uuid.NewString()
	previous := time.Duration(0)
	for attempt := 1; attempt <= 20; attempt++ {
		delay := notificationRetryDelay(id, attempt, config)
		if delay < previous || delay > time.Minute {
			t.Fatalf("attempt %d delay=%s previous=%s", attempt, delay, previous)
		}
		if repeated := notificationRetryDelay(id, attempt, config); repeated != delay {
			t.Fatalf("attempt %d delay is not stable: %s != %s", attempt, delay, repeated)
		}
		previous = delay
	}
}

func TestNotificationDeliveryIdentityAndDigestGrouping(t *testing.T) {
	event := Event{
		OrgID: uuid.NewString(), Type: "risk.changed", EntityType: "risk", EntityID: uuid.NewString(),
		Timestamp: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC), Data: map[string]interface{}{"score": 20},
	}
	if notificationEventID(event) != notificationEventID(event) {
		t.Fatal("derived notification event ID is not stable")
	}
	key := notificationDeliveryKey(notificationEventID(event), "rule", "recipient", "channel")
	if len(key) != 64 || strings.Trim(key, "0123456789abcdef") != "" {
		t.Fatalf("delivery key = %q", key)
	}

	scheduled := time.Date(2026, 9, 14, 13, 0, 0, 0, time.UTC)
	base := Notification{
		OrgID: event.OrgID, RecipientUserID: uuid.NewString(), ChannelID: uuid.NewString(),
		ChannelType: "email", DigestFrequency: "hourly", ScheduledFor: scheduled,
		CreatedAt: event.Timestamp, Subject: "First", TextBody: "First body",
	}
	first, second := base, base
	first.ID, second.ID = uuid.NewString(), uuid.NewString()
	second.Subject, second.TextBody = "Second", "Second body"
	groups := groupClaimedNotifications([]Notification{second, first})
	if len(groups) != 1 || len(groups[0].notifications) != 2 {
		t.Fatalf("digest groups = %#v", groups)
	}
	digest := buildDigestNotification(groups[0])
	if !strings.Contains(digest.Subject, "2 notifications") ||
		!strings.Contains(digest.TextBody, "First body") || !strings.Contains(digest.TextBody, "Second body") {
		t.Fatalf("digest = %#v", digest)
	}
	if repeated := buildDigestNotification(groups[0]); repeated.ID != digest.ID {
		t.Fatalf("digest idempotency IDs differ: %s != %s", digest.ID, repeated.ID)
	}
}

func TestDigestChunkingRetainsEveryNotification(t *testing.T) {
	base := Notification{
		OrgID: uuid.NewString(), RecipientUserID: uuid.NewString(), ChannelID: uuid.NewString(),
		ChannelType: "slack", DigestFrequency: "hourly",
		ScheduledFor: time.Date(2026, 9, 14, 13, 0, 0, 0, time.UTC),
		CreatedAt:    time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
	}
	claimed := make([]Notification, 0, 5)
	for index := 0; index < 5; index++ {
		notification := base
		notification.ID = uuid.NewString()
		notification.Subject = "Digest event"
		notification.TextBody = strings.Repeat("x", 1200)
		claimed = append(claimed, notification)
	}
	groups := groupClaimedNotifications(claimed)
	if len(groups) < 2 {
		t.Fatalf("oversized Slack digest was not chunked: groups=%d", len(groups))
	}
	total := 0
	seen := make(map[string]struct{}, len(claimed))
	for _, group := range groups {
		digest := buildDigestNotification(group)
		if utf8.RuneCountInString(digest.Body) > 2400 {
			t.Fatalf("digest chunk body contains %d runes", utf8.RuneCountInString(digest.Body))
		}
		for _, notification := range group.notifications {
			if _, duplicate := seen[notification.ID]; duplicate {
				t.Fatalf("notification %s appeared in multiple digest chunks", notification.ID)
			}
			seen[notification.ID] = struct{}{}
			total++
		}
	}
	if total != len(claimed) {
		t.Fatalf("digest retained %d of %d notifications", total, len(claimed))
	}
}

func TestNotificationDeliveryFailureClassification(t *testing.T) {
	classified := classifyNotificationDeliveryError(notificationDeliveryError(
		"configuration_invalid", true, errors.New("https://user:secret@example.test/token"),
	))
	if classified.Code != "configuration_invalid" || !classified.Permanent {
		t.Fatalf("classification = %#v", classified)
	}
	if strings.Contains(classified.Code, "secret") || strings.Contains(classified.Code, "example.test") {
		t.Fatal("classification leaked provider error detail")
	}
}

func TestNotificationDeliveryConfigFromEnvironment(t *testing.T) {
	ownerID := uuid.NewString()
	t.Setenv("NOTIFICATION_DELIVERY_CLAIM_BATCH", "25")
	t.Setenv("NOTIFICATION_DELIVERY_LEASE", "4m")
	t.Setenv("NOTIFICATION_RETRY_BASE_DELAY", "2s")
	t.Setenv("NOTIFICATION_RETRY_MAX_DELAY", "10m")
	t.Setenv("NOTIFICATION_DELIVERY_POLL_INTERVAL", "5s")
	config, err := NotificationDeliveryConfigFromEnvironment(ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if config.OwnerID != ownerID || config.ClaimBatch != 25 || config.LeaseDuration != 4*time.Minute || config.PollInterval != 5*time.Second {
		t.Fatalf("config = %#v", config)
	}
	t.Setenv("NOTIFICATION_DELIVERY_LEASE", "5s")
	if _, err := NotificationDeliveryConfigFromEnvironment(ownerID); err == nil {
		t.Fatal("expected unsafe lease configuration to fail")
	}
}
