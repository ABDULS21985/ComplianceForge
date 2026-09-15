package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/database"
)

const (
	defaultNotificationTenantBatch  = 100
	defaultNotificationClaimBatch   = 100
	defaultNotificationLease        = 3 * time.Minute
	defaultNotificationRetryBase    = 30 * time.Second
	defaultNotificationRetryMax     = time.Hour
	defaultNotificationPoll         = 10 * time.Second
	maximumNotificationTenantErrors = 100
)

// NotificationDeliveryConfig controls durable notification claiming. OwnerID
// is a stable worker-instance UUID and is combined with a fresh per-claim token
// to fence stale workers after a lease expires.
type NotificationDeliveryConfig struct {
	OwnerID        string
	TenantBatch    int // Registry page size, not a global per-cycle tenant ceiling.
	ClaimBatch     int
	LeaseDuration  time.Duration
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
	PollInterval   time.Duration
}

// NotificationDeliveryConfigFromEnvironment loads worker-specific tuning
// without coupling notification delivery to the API configuration structure.
func NotificationDeliveryConfigFromEnvironment(ownerID string) (NotificationDeliveryConfig, error) {
	config := NotificationDeliveryConfig{
		OwnerID:        ownerID,
		TenantBatch:    defaultNotificationTenantBatch,
		ClaimBatch:     defaultNotificationClaimBatch,
		LeaseDuration:  defaultNotificationLease,
		RetryBaseDelay: defaultNotificationRetryBase,
		RetryMaxDelay:  defaultNotificationRetryMax,
		PollInterval:   defaultNotificationPoll,
	}
	var err error
	if config.TenantBatch, err = notificationEnvInt("NOTIFICATION_DELIVERY_TENANT_BATCH", config.TenantBatch); err != nil {
		return NotificationDeliveryConfig{}, err
	}
	if config.ClaimBatch, err = notificationEnvInt("NOTIFICATION_DELIVERY_CLAIM_BATCH", config.ClaimBatch); err != nil {
		return NotificationDeliveryConfig{}, err
	}
	if config.LeaseDuration, err = notificationEnvDuration("NOTIFICATION_DELIVERY_LEASE", config.LeaseDuration); err != nil {
		return NotificationDeliveryConfig{}, err
	}
	if config.RetryBaseDelay, err = notificationEnvDuration("NOTIFICATION_RETRY_BASE_DELAY", config.RetryBaseDelay); err != nil {
		return NotificationDeliveryConfig{}, err
	}
	if config.RetryMaxDelay, err = notificationEnvDuration("NOTIFICATION_RETRY_MAX_DELAY", config.RetryMaxDelay); err != nil {
		return NotificationDeliveryConfig{}, err
	}
	if config.PollInterval, err = notificationEnvDuration("NOTIFICATION_DELIVERY_POLL_INTERVAL", config.PollInterval); err != nil {
		return NotificationDeliveryConfig{}, err
	}
	if err := config.Validate(); err != nil {
		return NotificationDeliveryConfig{}, err
	}
	return config, nil
}

// Validate rejects unsafe lease/retry settings before a worker starts.
func (config NotificationDeliveryConfig) Validate() error {
	if _, err := uuid.Parse(config.OwnerID); err != nil {
		return fmt.Errorf("notification delivery owner ID must be a UUID")
	}
	if config.TenantBatch < 1 || config.TenantBatch > 1000 {
		return fmt.Errorf("notification tenant batch must be between 1 and 1000")
	}
	if config.ClaimBatch < 1 || config.ClaimBatch > 500 {
		return fmt.Errorf("notification claim batch must be between 1 and 500")
	}
	if config.LeaseDuration < 3*time.Minute || config.LeaseDuration > 15*time.Minute {
		return fmt.Errorf("notification delivery lease must be between three and 15 minutes")
	}
	if config.RetryBaseDelay < time.Second || config.RetryMaxDelay < config.RetryBaseDelay || config.RetryMaxDelay > 24*time.Hour {
		return fmt.Errorf("notification retry delays must be ordered between one second and 24 hours")
	}
	if config.PollInterval < time.Second || config.PollInterval > time.Minute {
		return fmt.Errorf("notification delivery poll interval must be between one second and one minute")
	}
	return nil
}

func notificationEnvInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	return parsed, nil
}

func notificationEnvDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", name, err)
	}
	return parsed, nil
}

type notificationPreferenceDecision struct {
	Enabled            bool
	DigestFrequency    string
	QuietHoursStart    *string
	QuietHoursEnd      *string
	QuietHoursTimezone *string
}

func defaultNotificationPreference() notificationPreferenceDecision {
	return notificationPreferenceDecision{Enabled: true, DigestFrequency: "immediate"}
}

func notificationScheduledFor(now time.Time, preference notificationPreferenceDecision) time.Time {
	location := time.UTC
	if preference.QuietHoursTimezone != nil && strings.TrimSpace(*preference.QuietHoursTimezone) != "" {
		if configured, err := time.LoadLocation(strings.TrimSpace(*preference.QuietHoursTimezone)); err == nil {
			location = configured
		}
	}
	localNow := now.In(location)
	scheduled := localNow
	switch preference.DigestFrequency {
	case "hourly":
		scheduled = time.Date(localNow.Year(), localNow.Month(), localNow.Day(), localNow.Hour()+1, 0, 0, 0, location)
	case "daily":
		scheduled = time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 8, 0, 0, 0, location)
		if !scheduled.After(localNow) {
			scheduled = scheduled.AddDate(0, 0, 1)
		}
	case "weekly":
		daysUntilMonday := (int(time.Monday) - int(localNow.Weekday()) + 7) % 7
		scheduled = time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 8, 0, 0, 0, location).AddDate(0, 0, daysUntilMonday)
		if !scheduled.After(localNow) {
			scheduled = scheduled.AddDate(0, 0, 7)
		}
	}
	return deferForQuietHours(scheduled, preference, location).UTC()
}

func deferForQuietHours(scheduled time.Time, preference notificationPreferenceDecision, location *time.Location) time.Time {
	if preference.QuietHoursStart == nil || preference.QuietHoursEnd == nil {
		return scheduled
	}
	startMinutes, startOK := notificationClockMinutes(*preference.QuietHoursStart)
	endMinutes, endOK := notificationClockMinutes(*preference.QuietHoursEnd)
	if !startOK || !endOK || startMinutes == endMinutes {
		return scheduled
	}
	local := scheduled.In(location)
	currentMinutes := local.Hour()*60 + local.Minute()
	inside := false
	endTomorrow := false
	if startMinutes < endMinutes {
		inside = currentMinutes >= startMinutes && currentMinutes < endMinutes
	} else {
		inside = currentMinutes >= startMinutes || currentMinutes < endMinutes
		endTomorrow = currentMinutes >= startMinutes
	}
	if !inside {
		return scheduled
	}
	endHour, endMinute := endMinutes/60, endMinutes%60
	deferred := time.Date(local.Year(), local.Month(), local.Day(), endHour, endMinute, 0, 0, location)
	if endTomorrow || !deferred.After(local) {
		deferred = deferred.AddDate(0, 0, 1)
	}
	return deferred
}

func notificationClockMinutes(value string) (int, bool) {
	for _, layout := range []string{"15:04:05", "15:04"} {
		parsed, err := time.Parse(layout, strings.TrimSpace(value))
		if err == nil {
			return parsed.Hour()*60 + parsed.Minute(), true
		}
	}
	return 0, false
}

func notificationEventID(event Event) string {
	encoded, _ := json.Marshal(struct {
		OrgID      string                 `json:"org_id"`
		Type       string                 `json:"type"`
		EntityType string                 `json:"entity_type"`
		EntityID   string                 `json:"entity_id"`
		Timestamp  time.Time              `json:"timestamp"`
		Data       map[string]interface{} `json:"data"`
	}{event.OrgID, event.Type, event.EntityType, event.EntityID, event.Timestamp.UTC(), event.Data})
	return uuid.NewSHA1(uuid.NameSpaceOID, encoded).String()
}

func notificationDeliveryKey(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

// RunDeliveryCycle traverses the active-tenant registry in bounded keyset pages
// and checks due work through each tenant's FORCE-RLS connection. Legacy due
// wrappers depend on a privileged migration owner and are deliberately unused.
// One bounded claim batch per tenant prevents noisy-tenant starvation; page
// size is not a global tenant ceiling. The registry exposes no tenant content.
func (ne *NotificationEngine) RunDeliveryCycle(ctx context.Context, config NotificationDeliveryConfig) error {
	if ctx == nil || ne == nil || ne.pool == nil {
		return fmt.Errorf("notification delivery database is not configured")
	}
	if err := config.Validate(); err != nil {
		return err
	}
	if database.QuerierFromContext(ctx, nil) != nil {
		return errors.New("notification delivery requires an unscoped worker context")
	}
	return runNotificationTenantPages(ctx, config.TenantBatch, ne.notificationTenantPage, func(tenantCtx context.Context, tenantID string) error {
		return database.WithTenantConnection(tenantCtx, ne.pool, tenantID, func(scopedCtx context.Context) error {
			return errors.Join(
				ne.createDueEscalations(scopedCtx, tenantID, config.ClaimBatch),
				ne.deliverDueForTenant(scopedCtx, tenantID, config),
			)
		})
	})
}

func (ne *NotificationEngine) notificationTenantPage(ctx context.Context, after *string, limit int) ([]string, error) {
	rows, err := ne.pool.Query(ctx, `SELECT organization_id FROM evidence_due_tenants($1,$2::uuid)`, limit, after)
	if err != nil {
		return nil, fmt.Errorf("discover active notification tenants: %w", err)
	}
	tenantIDs := make([]string, 0, limit)
	for rows.Next() {
		var tenantID string
		if err := rows.Scan(&tenantID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan active notification tenant: %w", err)
		}
		tenantIDs = append(tenantIDs, tenantID)
	}
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil {
		return nil, fmt.Errorf("iterate active notification tenants: %w", rowErr)
	}
	return tenantIDs, nil
}

// Keeping pagination separate makes continuation, cancellation, ordered-page
// validation and bounded error retention executable without database mocks.
func runNotificationTenantPages(
	ctx context.Context, pageSize int,
	discover func(context.Context, *string, int) ([]string, error),
	run func(context.Context, string) error,
) error {
	if ctx == nil || discover == nil || run == nil || pageSize < 1 || pageSize > 1000 {
		return errors.New("notification tenant pagination is not configured")
	}
	var deliveryErrors []error
	failedTenants := 0
	result := func(cause error) error {
		if omitted := failedTenants - len(deliveryErrors); omitted > 0 {
			deliveryErrors = append(deliveryErrors, fmt.Errorf("notification delivery failed for %d additional tenants", omitted))
		}
		return errors.Join(append(deliveryErrors, cause)...)
	}
	var after *string
	for {
		if err := ctx.Err(); err != nil {
			return result(err)
		}
		tenantIDs, err := discover(ctx, after, pageSize)
		if err != nil {
			return result(err)
		}
		if len(tenantIDs) > pageSize {
			return result(errors.New("notification registry returned an oversized page"))
		}
		previous := ""
		if after != nil {
			previous = *after
		}
		// Validate the entire page before effects; a malformed/cyclic response
		// must not cause repeated deliveries or an endless registry traversal.
		for _, tenantID := range tenantIDs {
			id, err := uuid.Parse(tenantID)
			if err != nil || id == uuid.Nil || id.String() != tenantID || tenantID <= previous {
				return result(errors.New("notification registry returned an invalid ordered page"))
			}
			previous = tenantID
		}
		for _, tenantID := range tenantIDs {
			if err := ctx.Err(); err != nil {
				return result(err)
			}
			if err := run(ctx, tenantID); err != nil {
				failedTenants++
				if len(deliveryErrors) < maximumNotificationTenantErrors {
					deliveryErrors = append(deliveryErrors, fmt.Errorf("tenant %s notification delivery: %w", tenantID, err))
				}
			}
		}
		if len(tenantIDs) < pageSize {
			return result(nil)
		}
		after = &tenantIDs[len(tenantIDs)-1]
	}
}

func (ne *NotificationEngine) deliverDueForTenant(ctx context.Context, tenantID string, config NotificationDeliveryConfig) error {
	now := time.Now().UTC()
	claimed, err := ne.claimDueNotifications(ctx, tenantID, config, now)
	if err != nil {
		return err
	}
	groups := groupClaimedNotifications(claimed)
	var deliveryErrors []error
	for _, group := range groups {
		if err := ne.deliverClaimedGroup(ctx, group, config, now); err != nil {
			deliveryErrors = append(deliveryErrors, err)
		}
	}
	return errors.Join(deliveryErrors...)
}

func (ne *NotificationEngine) claimDueNotifications(
	ctx context.Context,
	tenantID string,
	config NotificationDeliveryConfig,
	now time.Time,
) ([]Notification, error) {
	rows, err := database.QuerierFromContext(ctx, ne.pool).Query(ctx, `
		WITH candidates AS (
			SELECT id
			FROM notifications
			WHERE organization_id=$1
			  AND status IN ('pending','failed')
			  AND dead_at IS NULL
			  AND retry_count < max_retries
			  AND COALESCE(next_retry_at,scheduled_for) <= $2
			  AND (leased_until IS NULL OR leased_until <= $2)
			ORDER BY COALESCE(next_retry_at,scheduled_for),created_at,id
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		UPDATE notifications AS n
		SET lease_owner=$4,
		    lease_token=gen_random_uuid(),
		    leased_until=$2 + make_interval(secs => $5),
		    last_attempt_at=$2,
		    retry_count=n.retry_count+1
		FROM candidates
		WHERE n.id=candidates.id
		RETURNING n.id,n.organization_id,n.event_type,n.recipient_user_id,
		          n.channel_type,COALESCE(n.channel_id::text,''),COALESCE(n.subject,''),
		          COALESCE(n.body,''),COALESCE(n.body_text,''),COALESCE(n.body_html,''),
		          n.status,n.created_at,n.digest_frequency,n.scheduled_for,
		          n.retry_count,n.max_retries,n.lease_owner,n.lease_token`,
		tenantID, now, config.ClaimBatch, config.OwnerID, config.LeaseDuration.Seconds())
	if err != nil {
		return nil, fmt.Errorf("claim due notifications: %w", err)
	}
	defer rows.Close()
	claimed := make([]Notification, 0, config.ClaimBatch)
	for rows.Next() {
		var notification Notification
		if err := rows.Scan(
			&notification.ID, &notification.OrgID, &notification.EventType,
			&notification.RecipientUserID, &notification.ChannelType, &notification.ChannelID,
			&notification.Subject, &notification.Body, &notification.TextBody, &notification.HTMLBody,
			&notification.Status, &notification.CreatedAt, &notification.DigestFrequency,
			&notification.ScheduledFor, &notification.RetryCount, &notification.MaxRetries,
			&notification.LeaseOwner, &notification.LeaseToken,
		); err != nil {
			return nil, fmt.Errorf("scan claimed notification: %w", err)
		}
		claimed = append(claimed, notification)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed notifications: %w", err)
	}
	return claimed, nil
}

type claimedNotificationGroup struct {
	key           string
	notifications []Notification
}

func groupClaimedNotifications(claimed []Notification) []claimedNotificationGroup {
	groupsByKey := make(map[string][]Notification, len(claimed))
	for _, notification := range claimed {
		key := notification.ID
		if notification.DigestFrequency != "immediate" && notification.ChannelType != "in_app" && notification.ChannelType != "webhook" {
			key = strings.Join([]string{
				notification.OrgID, notification.RecipientUserID, notification.ChannelID,
				notification.ChannelType, notification.DigestFrequency,
				notification.ScheduledFor.UTC().Format(time.RFC3339Nano),
			}, "|")
		}
		groupsByKey[key] = append(groupsByKey[key], notification)
	}
	keys := make([]string, 0, len(groupsByKey))
	for key := range groupsByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	groups := make([]claimedNotificationGroup, 0, len(keys))
	for _, key := range keys {
		notifications := groupsByKey[key]
		sort.Slice(notifications, func(i, j int) bool {
			if notifications[i].CreatedAt.Equal(notifications[j].CreatedAt) {
				return notifications[i].ID < notifications[j].ID
			}
			return notifications[i].CreatedAt.Before(notifications[j].CreatedAt)
		})
		if len(notifications) > 0 && notifications[0].DigestFrequency != "immediate" &&
			notifications[0].ChannelType != "in_app" && notifications[0].ChannelType != "webhook" {
			groups = append(groups, splitClaimedDigestGroup(key, notifications)...)
			continue
		}
		groups = append(groups, claimedNotificationGroup{key: key, notifications: notifications})
	}
	return groups
}

func splitClaimedDigestGroup(key string, notifications []Notification) []claimedNotificationGroup {
	if len(notifications) == 0 {
		return nil
	}
	maximumRunes := 128 * 1024
	if notifications[0].ChannelType == "slack" {
		// Slack incoming webhooks cap the rendered message at roughly 3,000
		// characters. Leave room for the digest heading and block syntax.
		maximumRunes = 2400
	}
	var groups []claimedNotificationGroup
	start, used := 0, 0
	for index, notification := range notifications {
		entrySize := utf8.RuneCountInString(notificationDigestEntry(notification)) + 2
		if index > start && used+entrySize > maximumRunes {
			groups = append(groups, claimedNotificationGroup{
				key:           digestChunkKey(key, notifications[start:index]),
				notifications: notifications[start:index],
			})
			start, used = index, 0
		}
		used += entrySize
	}
	groups = append(groups, claimedNotificationGroup{
		key:           digestChunkKey(key, notifications[start:]),
		notifications: notifications[start:],
	})
	return groups
}

func digestChunkKey(groupKey string, notifications []Notification) string {
	identities := make([]string, 0, len(notifications)+1)
	identities = append(identities, groupKey)
	for _, notification := range notifications {
		identities = append(identities, notification.ID)
	}
	return notificationDeliveryKey(identities...)
}

func notificationDigestEntry(notification Notification) string {
	body := notification.TextBody
	if body == "" {
		body = notification.Body
	}
	maximumBodyRunes := 8000
	if notification.ChannelType == "slack" {
		maximumBodyRunes = 1500
	}
	body = truncateNotificationText(body, maximumBodyRunes)
	if strings.TrimSpace(body) == "" {
		return notification.Subject
	}
	return notification.Subject + "\n" + body
}

func (ne *NotificationEngine) deliverClaimedGroup(
	ctx context.Context,
	group claimedNotificationGroup,
	config NotificationDeliveryConfig,
	now time.Time,
) error {
	if len(group.notifications) == 0 {
		return nil
	}
	first := group.notifications[0]
	deliverable := first
	if first.DigestFrequency != "immediate" && first.ChannelType != "in_app" && first.ChannelType != "webhook" {
		deliverable = buildDigestNotification(group)
	}
	channelConfig := make(map[string]any)
	var deliveryErr error
	if first.ChannelType != "in_app" || first.ChannelID != "" {
		_, channelConfig, deliveryErr = ne.getChannelConfig(ctx, first.OrgID, first.ChannelID)
	}
	if deliveryErr == nil {
		deliveryErr = ne.Dispatch(ctx, deliverable, channelConfig)
	}
	failure := classifyNotificationDeliveryError(deliveryErr)

	var stateErrors []error
	for _, notification := range group.notifications {
		if deliveryErr == nil {
			if err := ne.completeNotificationDelivery(ctx, notification, now); err != nil {
				stateErrors = append(stateErrors, err)
			}
			continue
		}
		if err := ne.failNotificationDelivery(ctx, notification, failure, config, now); err != nil {
			stateErrors = append(stateErrors, err)
		}
	}
	if deliveryErr != nil {
		// Provider and configuration errors can contain credentials, URLs or
		// recipient data. Logs and database storage therefore receive only a
		// bounded classification, never the raw error text.
		log.Warn().
			Str("organization_id", first.OrgID).
			Str("channel_type", first.ChannelType).
			Str("failure_code", failure.Code).
			Bool("permanent", failure.Permanent).
			Int("notification_count", len(group.notifications)).
			Msg("notification delivery attempt failed")
	}
	return errors.Join(stateErrors...)
}

func buildDigestNotification(group claimedNotificationGroup) Notification {
	first := group.notifications[0]
	frequency := strings.ToLower(first.DigestFrequency)
	label := frequency
	if label != "" {
		label = strings.ToUpper(label[:1]) + label[1:]
	}
	subject := fmt.Sprintf("[ComplianceForge] %s digest (%d notifications)", label, len(group.notifications))
	var textBody strings.Builder
	for _, notification := range group.notifications {
		textBody.WriteString(notificationDigestEntry(notification))
		textBody.WriteString("\n\n")
	}
	text := strings.TrimSpace(textBody.String())
	htmlBody := "<h2>" + htmltemplate.HTMLEscapeString(subject) + "</h2><pre>" +
		htmltemplate.HTMLEscapeString(text) + "</pre>"
	return Notification{
		ID:              uuid.NewSHA1(uuid.NameSpaceOID, []byte("notification-digest|"+group.key)).String(),
		OrgID:           first.OrgID,
		EventType:       "notification.digest",
		RecipientUserID: first.RecipientUserID,
		ChannelType:     first.ChannelType,
		ChannelID:       first.ChannelID,
		Subject:         subject,
		Body:            text,
		TextBody:        text,
		HTMLBody:        htmlBody,
		CreatedAt:       first.ScheduledFor,
	}
}

func truncateNotificationText(value string, maximum int) string {
	if maximum <= 0 || utf8.RuneCountInString(value) <= maximum {
		return value
	}
	runes := []rune(value)
	return string(runes[:maximum-1]) + "…"
}

func (ne *NotificationEngine) completeNotificationDelivery(ctx context.Context, notification Notification, now time.Time) error {
	status := "sent"
	if notification.ChannelType == "in_app" {
		status = "delivered"
	}
	result, err := database.QuerierFromContext(ctx, ne.pool).Exec(ctx, `
		UPDATE notifications AS n
		SET status=$1::notification_status,
		    sent_at=CASE WHEN $1::notification_status='sent' THEN $2 ELSE sent_at END,
		    delivered_at=CASE WHEN $1::notification_status='delivered' THEN $2 ELSE delivered_at END,
		    acknowledgement_due_at=CASE
		        WHEN n.parent_notification_id IS NULL
		         AND COALESCE((
		             SELECT nr.escalation_after_minutes
		             FROM notification_rules AS nr
		             WHERE nr.id=n.rule_id
		               AND cardinality(COALESCE(nr.escalation_channel_ids,'{}'::uuid[])) > 0
		         ),0) > 0
		        THEN $2 + make_interval(mins => (
		             SELECT nr.escalation_after_minutes FROM notification_rules AS nr WHERE nr.id=n.rule_id
		        ))
		        ELSE n.acknowledgement_due_at
		    END,
		    error_message=NULL,failure_code=NULL,next_retry_at=NULL,dead_at=NULL,
		    lease_owner=NULL,lease_token=NULL,leased_until=NULL
		WHERE n.id=$3 AND n.organization_id=$4 AND n.lease_owner=$5 AND n.lease_token=$6`,
		status, now, notification.ID, notification.OrgID, notification.LeaseOwner, notification.LeaseToken)
	if err != nil {
		return fmt.Errorf("complete notification %s: %w", notification.ID, err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("complete notification %s: delivery lease was lost", notification.ID)
	}
	return nil
}

type notificationDeliveryFailure struct {
	Code      string
	Permanent bool
}

func classifyNotificationDeliveryError(err error) notificationDeliveryFailure {
	if err == nil {
		return notificationDeliveryFailure{}
	}
	var classified *classifiedNotificationError
	if errors.As(err, &classified) {
		return notificationDeliveryFailure{Code: classified.code, Permanent: classified.permanent}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return notificationDeliveryFailure{Code: "transport_timeout"}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return notificationDeliveryFailure{Code: "recipient_unavailable", Permanent: true}
	}
	return notificationDeliveryFailure{Code: "transport_failure"}
}

type classifiedNotificationError struct {
	code      string
	permanent bool
	err       error
}

func (err *classifiedNotificationError) Error() string { return err.err.Error() }
func (err *classifiedNotificationError) Unwrap() error { return err.err }

func notificationDeliveryError(code string, permanent bool, err error) error {
	if err == nil {
		err = errors.New("notification delivery failed")
	}
	return &classifiedNotificationError{code: code, permanent: permanent, err: err}
}

func (ne *NotificationEngine) failNotificationDelivery(
	ctx context.Context,
	notification Notification,
	failure notificationDeliveryFailure,
	config NotificationDeliveryConfig,
	now time.Time,
) error {
	if failure.Code == "" {
		failure.Code = "transport_failure"
	}
	terminal := failure.Permanent || notification.RetryCount >= notification.MaxRetries
	var nextRetry *time.Time
	if !terminal {
		next := now.Add(notificationRetryDelay(notification.ID, notification.RetryCount, config))
		nextRetry = &next
	}
	storedMessage := "notification delivery failed (" + failure.Code + ")"
	if !terminal {
		storedMessage += "; retry scheduled"
	}
	result, err := database.QuerierFromContext(ctx, ne.pool).Exec(ctx, `
		UPDATE notifications
		SET status='failed',error_message=$1::text,failure_code=$2::varchar,next_retry_at=$3::timestamptz,
		    dead_at=CASE WHEN $4::boolean THEN $5::timestamptz ELSE NULL::timestamptz END,
		    lease_owner=NULL,lease_token=NULL,leased_until=NULL
		WHERE id=$6 AND organization_id=$7 AND lease_owner=$8 AND lease_token=$9`,
		storedMessage, failure.Code, nextRetry, terminal, now,
		notification.ID, notification.OrgID, notification.LeaseOwner, notification.LeaseToken)
	if err != nil {
		return fmt.Errorf("record notification %s failure: %w", notification.ID, err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("record notification %s failure: delivery lease was lost", notification.ID)
	}
	return nil
}

func notificationRetryDelay(notificationID string, attempt int, config NotificationDeliveryConfig) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := config.RetryBaseDelay
	for exponent := 1; exponent < attempt && delay < config.RetryMaxDelay; exponent++ {
		if delay > config.RetryMaxDelay/2 {
			delay = config.RetryMaxDelay
			break
		}
		delay *= 2
	}
	if delay > config.RetryMaxDelay {
		delay = config.RetryMaxDelay
	}
	// Stable 0-10% jitter avoids synchronized retry storms without making a
	// notification's retry time nondeterministic across worker restarts.
	digest := sha256.Sum256([]byte(notificationID))
	jitter := time.Duration(int64(delay) * int64(digest[0]%11) / 100)
	if delay+jitter > config.RetryMaxDelay {
		return config.RetryMaxDelay
	}
	return delay + jitter
}

type escalationCandidate struct {
	ID                   string
	OrgID                string
	RuleID               *string
	EventID              *string
	EventType            string
	EventPayload         []byte
	RecipientUserID      string
	Subject              string
	Body                 string
	TextBody             string
	HTMLBody             string
	MaxRetries           int
	EscalationChannelIDs []string
}

type notificationTransactionBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func (ne *NotificationEngine) createDueEscalations(ctx context.Context, tenantID string, batch int) error {
	querier := database.QuerierFromContext(ctx, ne.pool)
	beginner, ok := querier.(notificationTransactionBeginner)
	if !ok {
		return fmt.Errorf("notification database executor cannot start a transaction")
	}
	tx, err := beginner.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin notification escalation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	txCtx := database.WithQuerier(ctx, tx)
	rows, err := tx.Query(txCtx, `
		SELECT n.id,n.organization_id,n.rule_id,n.event_id,n.event_type,n.event_payload,
		       n.recipient_user_id,COALESCE(n.subject,''),COALESCE(n.body,''),
		       COALESCE(n.body_text,''),COALESCE(n.body_html,''),n.max_retries,
		       COALESCE(r.escalation_channel_ids,'{}'::uuid[])
		FROM notifications AS n
		JOIN notification_rules AS r ON r.id=n.rule_id AND r.organization_id=n.organization_id
		WHERE n.organization_id=$1
		  AND n.parent_notification_id IS NULL
		  AND n.status IN ('sent','delivered')
		  AND n.acknowledgement_due_at IS NOT NULL
		  AND n.acknowledgement_due_at <= NOW()
		  AND n.acknowledged_at IS NULL
		  AND n.escalated_at IS NULL
		ORDER BY n.acknowledgement_due_at,n.id
		LIMIT $2
		FOR UPDATE OF n SKIP LOCKED`, tenantID, batch)
	if err != nil {
		return fmt.Errorf("claim notification escalations: %w", err)
	}
	var candidates []escalationCandidate
	for rows.Next() {
		var candidate escalationCandidate
		if err := rows.Scan(
			&candidate.ID, &candidate.OrgID, &candidate.RuleID, &candidate.EventID,
			&candidate.EventType, &candidate.EventPayload, &candidate.RecipientUserID,
			&candidate.Subject, &candidate.Body, &candidate.TextBody, &candidate.HTMLBody,
			&candidate.MaxRetries, &candidate.EscalationChannelIDs,
		); err != nil {
			rows.Close()
			return fmt.Errorf("scan due notification escalation: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil {
		return fmt.Errorf("iterate due notification escalations: %w", rowErr)
	}

	for _, candidate := range candidates {
		for _, channelID := range candidate.EscalationChannelIDs {
			var channelType string
			if err := tx.QueryRow(txCtx, `
				SELECT channel_type FROM notification_channels
				WHERE id=$1 AND organization_id=$2 AND is_active=true AND deleted_at IS NULL`,
				channelID, candidate.OrgID).Scan(&channelType); err != nil {
				return fmt.Errorf("load escalation channel: %w", err)
			}
			escalationEventID := candidate.ID
			if candidate.EventID != nil {
				escalationEventID = *candidate.EventID
			}
			deliveryKey := notificationDeliveryKey("escalation", candidate.ID, channelID)
			subject := "Escalation: " + candidate.Subject
			textBody := "Acknowledgement is overdue.\n\n" + candidate.TextBody
			htmlBody := "<p><strong>Acknowledgement is overdue.</strong></p>" + candidate.HTMLBody
			body := textBody
			if channelType == "email" && htmlBody != "" {
				body = htmlBody
			}
			if _, err := tx.Exec(txCtx, `
				INSERT INTO notifications
				    (organization_id,rule_id,event_id,event_type,event_payload,recipient_user_id,
				     channel_type,channel_id,subject,body,body_text,body_html,status,max_retries,
				     delivery_key,digest_frequency,scheduled_for,parent_notification_id,metadata,created_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'pending',$13,$14,'immediate',NOW(),$15::uuid,
				        jsonb_build_object('escalation',true,'parent_notification_id',($15::uuid)::text),NOW())
				ON CONFLICT (organization_id,delivery_key) DO NOTHING`,
				candidate.OrgID, candidate.RuleID, escalationEventID, candidate.EventType,
				candidate.EventPayload, candidate.RecipientUserID, channelType, channelID,
				subject, body, textBody, htmlBody, candidate.MaxRetries, deliveryKey, candidate.ID); err != nil {
				return fmt.Errorf("create notification escalation: %w", err)
			}
		}
		if _, err := tx.Exec(txCtx, `
			UPDATE notifications SET escalated_at=NOW()
			WHERE id=$1 AND organization_id=$2 AND acknowledged_at IS NULL AND escalated_at IS NULL`,
			candidate.ID, candidate.OrgID); err != nil {
			return fmt.Errorf("complete notification escalation: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit notification escalations: %w", err)
	}
	return nil
}

// AcknowledgeNotification records acknowledgement only for the authenticated
// recipient in the active tenant. It is idempotent and returns pgx.ErrNoRows
// when the notification is absent, belongs to another user/tenant, or has not
// reached a delivered state.
func (ne *NotificationEngine) AcknowledgeNotification(ctx context.Context, orgID, userID, notificationID string) (time.Time, error) {
	if ne == nil || ne.pool == nil {
		return time.Time{}, fmt.Errorf("notification database is not configured")
	}
	if _, err := uuid.Parse(orgID); err != nil {
		return time.Time{}, fmt.Errorf("organization ID is invalid")
	}
	if _, err := uuid.Parse(userID); err != nil {
		return time.Time{}, fmt.Errorf("user ID is invalid")
	}
	if _, err := uuid.Parse(notificationID); err != nil {
		return time.Time{}, fmt.Errorf("notification ID is invalid")
	}
	if ne.state == nil {
		return time.Time{}, fmt.Errorf("notification state repository is not configured")
	}
	var acknowledgedAt time.Time
	err := database.WithTenantConnection(ctx, ne.pool, orgID, func(tenantCtx context.Context) error {
		var acknowledgeErr error
		acknowledgedAt, acknowledgeErr = ne.state.Acknowledge(tenantCtx, orgID, userID, notificationID)
		return acknowledgeErr
	})
	if err != nil {
		return time.Time{}, err
	}
	return acknowledgedAt.UTC(), nil
}

// RecordNotificationBounce is the provider-adapter boundary for asynchronous
// bounce callbacks. Provider details are intentionally not persisted here.
func (ne *NotificationEngine) RecordNotificationBounce(ctx context.Context, orgID, notificationID, providerCode string) error {
	if ne == nil || ne.pool == nil {
		return fmt.Errorf("notification database is not configured")
	}
	if _, err := uuid.Parse(orgID); err != nil {
		return fmt.Errorf("organization ID is invalid")
	}
	if _, err := uuid.Parse(notificationID); err != nil {
		return fmt.Errorf("notification ID is invalid")
	}
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	if providerCode == "" || len(providerCode) > 40 || !isSafeEventToken(providerCode) {
		providerCode = "provider_rejected"
	}
	if ne.state == nil {
		return fmt.Errorf("notification state repository is not configured")
	}
	return database.WithTenantConnection(ctx, ne.pool, orgID, func(tenantCtx context.Context) error {
		return ne.state.RecordBounce(tenantCtx, orgID, notificationID, providerCode)
	})
}
