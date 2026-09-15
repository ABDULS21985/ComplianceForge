package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/pkg/secretbox"
	"github.com/complianceforge/platform/internal/repository"
)

var (
	ErrIdentityInvalid         = errors.New("invalid identity lifecycle request")
	ErrIdentityCredential      = errors.New("invalid or expired identity credential")
	ErrIdentityState           = errors.New("identity state conflict")
	ErrIdentityVersionConflict = errors.New("identity record has changed")
	ErrIdentityRateLimited     = errors.New("identity request rate limited")
	ErrIdentityUnavailable     = errors.New("identity security service unavailable")
	ErrIdentityMFARequired     = errors.New("multi-factor authentication required")
	ErrIdentityEnrollment      = errors.New("multi-factor enrollment required")
	ErrIdentityLastFactor      = errors.New("last required MFA factor is protected")
	ErrIdentityAdminRecovery   = errors.New("another MFA-enabled administrator is required")
)

type IdentityLifecycleStore = repository.IdentityLifecycleRepository

type IdentityRateLimiter interface {
	Allow(context.Context, string, int) (bool, time.Duration, error)
}

type IdentityLifecycleOption func(*IdentityLifecycleService)

type IdentityLifecycleService struct {
	store         IdentityLifecycleStore
	protector     *secretbox.Box
	webauthn      IdentityWebAuthnVerifier
	limiter       IdentityRateLimiter
	logger        zerolog.Logger
	now           func() time.Time
	random        io.Reader
	bcryptCost    int
	invitationTTL time.Duration
	emailTokenTTL time.Duration
	resetTokenTTL time.Duration
	enrollmentTTL time.Duration
	recoveryCount int
}

var _ AuthenticationIdentityLifecycle = (*IdentityLifecycleService)(nil)

func NewIdentityLifecycleService(store IdentityLifecycleStore, protector *secretbox.Box, verifier IdentityWebAuthnVerifier, limiter IdentityRateLimiter, logger zerolog.Logger, options ...IdentityLifecycleOption) (*IdentityLifecycleService, error) {
	if store == nil {
		return nil, errors.New("identity lifecycle store is required")
	}
	if protector == nil {
		return nil, errors.New("identity lifecycle encryption is required")
	}
	if verifier == nil {
		return nil, errors.New("identity lifecycle WebAuthn verifier is required")
	}
	if limiter == nil {
		return nil, errors.New("identity lifecycle rate limiter is required")
	}
	service := &IdentityLifecycleService{
		store: store, protector: protector, webauthn: verifier, limiter: limiter,
		logger: logger.With().Str("service", "identity_lifecycle").Logger(), now: time.Now, random: rand.Reader,
		bcryptCost: bcrypt.DefaultCost, invitationTTL: 72 * time.Hour, emailTokenTTL: 24 * time.Hour,
		resetTokenTTL: time.Hour, enrollmentTTL: 10 * time.Minute, recoveryCount: 10,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	if service.now == nil || service.random == nil || service.bcryptCost < bcrypt.MinCost || service.bcryptCost > bcrypt.MaxCost {
		return nil, errors.New("identity lifecycle options are invalid")
	}
	return service, nil
}

func WithIdentityClock(clock func() time.Time) IdentityLifecycleOption {
	return func(service *IdentityLifecycleService) { service.now = clock }
}

func WithIdentityRandom(reader io.Reader) IdentityLifecycleOption {
	return func(service *IdentityLifecycleService) { service.random = reader }
}

func WithIdentityBcryptCost(cost int) IdentityLifecycleOption {
	return func(service *IdentityLifecycleService) { service.bcryptCost = cost }
}

func (s *IdentityLifecycleService) GetPolicy(ctx context.Context, organizationID string) (*models.IdentityPolicy, error) {
	if !identityUUID(organizationID) {
		return nil, ErrIdentityInvalid
	}
	policy, err := s.store.GetPolicy(ctx, organizationID)
	if errors.Is(err, repository.ErrIdentityNotFound) {
		return defaultIdentityPolicy(organizationID), nil
	}
	if err != nil {
		return nil, mapIdentityStoreError(err)
	}
	return policy, nil
}

func (s *IdentityLifecycleService) UpdatePolicy(ctx context.Context, organizationID, actorID string, patch models.IdentityPolicyPatch) (*models.IdentityPolicy, error) {
	patch.Reason = strings.TrimSpace(patch.Reason)
	if !identityUUID(organizationID) || !identityUUID(actorID) || patch.ExpectedVersion < 0 || !validIdentityReason(patch.Reason) {
		return nil, ErrIdentityInvalid
	}
	current, err := s.GetPolicy(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	if current.Version != patch.ExpectedVersion {
		return nil, ErrIdentityVersionConflict
	}
	if patch.AllowedMethods != nil {
		for _, configured := range patch.AllowedMethods {
			method := strings.ToLower(strings.TrimSpace(configured))
			if method != "totp" && method != "passkey" {
				return nil, ErrIdentityInvalid
			}
		}
		patch.AllowedMethods = normalizeIdentityMethods(patch.AllowedMethods)
		if len(patch.AllowedMethods) == 0 || len(patch.AllowedMethods) > 2 {
			return nil, ErrIdentityInvalid
		}
	}
	if patch.EnrollmentGraceHours != nil && (*patch.EnrollmentGraceHours < 0 || *patch.EnrollmentGraceHours > 720) ||
		patch.AuthenticationChallengeMins != nil && (*patch.AuthenticationChallengeMins < 1 || *patch.AuthenticationChallengeMins > 15) ||
		patch.StepUpTTLMinutes != nil && (*patch.StepUpTTLMinutes < 1 || *patch.StepUpTTLMinutes > 30) {
		return nil, ErrIdentityInvalid
	}
	result, err := s.store.UpdatePolicy(ctx, organizationID, actorID, patch)
	return result, mapIdentityStoreError(err)
}

func (s *IdentityLifecycleService) IssueInvitation(ctx context.Context, organizationID, userID, actorID string, input models.IdentityInvitationIssueInput, metadata models.IdentityRequestMetadata) (*models.IdentityInvitation, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if !identityUUID(organizationID) || !identityUUID(userID) || !identityUUID(actorID) || !validIdentityReason(input.Reason) {
		return nil, ErrIdentityInvalid
	}
	if input.ExpiresInHours == 0 {
		input.ExpiresInHours = int(s.invitationTTL / time.Hour)
	}
	if input.ExpiresInHours < 1 || input.ExpiresInHours > 720 {
		return nil, ErrIdentityInvalid
	}
	if err := s.rateLimit(ctx, "invitation", organizationID+":"+userID+":"+metadata.IPAddress, 10); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	id := uuid.NewString()
	raw, hash, err := s.newOpaqueCredential(organizationID)
	if err != nil {
		return nil, err
	}
	ciphertext, err := s.protector.Seal(organizationID, "invitation:"+id, []byte(raw))
	if err != nil {
		return nil, fmt.Errorf("protect invitation credential: %w", err)
	}
	invitation := models.IdentityInvitation{ID: id, OrganizationID: organizationID, UserID: userID,
		CreatedBy: actorID, CreatedAt: now, ExpiresAt: now.Add(time.Duration(input.ExpiresInHours) * time.Hour),
		PlainToken: raw, TokenHash: hash, DeliveryCipher: ciphertext}
	result, err := s.store.IssueInvitation(ctx, invitation, sanitizeIdentityMetadata(metadata), input.Reason)
	return result, mapIdentityStoreError(err)
}

func (s *IdentityLifecycleService) AcceptInvitation(ctx context.Context, input models.IdentityInvitationAcceptInput, metadata models.IdentityRequestMetadata) (*models.IdentityAcceptanceResult, error) {
	organizationID, tokenHash, err := parseOpaqueIdentityCredential(input.Token)
	if err != nil || !validNewIdentityPassword(input.Password) || utf8.RuneCountInString(strings.TrimSpace(input.FirstName)) > 100 ||
		utf8.RuneCountInString(strings.TrimSpace(input.LastName)) > 100 {
		return nil, ErrIdentityCredential
	}
	if err := s.rateLimit(ctx, "invitation_accept", tokenHash+":"+metadata.IPAddress, 8); err != nil {
		return nil, err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash invited user password: %w", err)
	}
	result, err := s.store.AcceptInvitation(ctx, organizationID, tokenHash, string(passwordHash),
		strings.TrimSpace(input.FirstName), strings.TrimSpace(input.LastName), s.now().UTC(), sanitizeIdentityMetadata(metadata))
	if err != nil {
		return nil, mapIdentityCredentialError(err)
	}
	return result, nil
}

func (s *IdentityLifecycleService) RequestEmailVerification(ctx context.Context, request models.IdentityEmailRequest, metadata models.IdentityRequestMetadata) error {
	return s.requestAccountToken(ctx, request, metadata, true)
}

func (s *IdentityLifecycleService) RequestPasswordReset(ctx context.Context, request models.IdentityEmailRequest, metadata models.IdentityRequestMetadata) error {
	return s.requestAccountToken(ctx, request, metadata, false)
}

func (s *IdentityLifecycleService) requestAccountToken(ctx context.Context, request models.IdentityEmailRequest, metadata models.IdentityRequestMetadata, emailVerification bool) error {
	request.Email = normalizeIdentityServiceEmail(request.Email)
	if !identityUUID(request.OrganizationID) || !validIdentityEmail(request.Email) {
		// Public recovery endpoints deliberately normalize malformed/unknown
		// identities to the same successful outcome.
		return nil
	}
	scope, limit, ttl := "password_reset", 5, s.resetTokenTTL
	if emailVerification {
		scope, limit, ttl = "email_verification", 6, s.emailTokenTTL
	}
	if err := s.rateLimit(ctx, scope, request.OrganizationID+":"+request.Email+":"+metadata.IPAddress, limit); err != nil {
		return err
	}
	now := s.now().UTC()
	id := uuid.NewString()
	raw, hash, err := s.newOpaqueCredential(request.OrganizationID)
	if err != nil {
		return err
	}
	ciphertext, err := s.protector.Seal(request.OrganizationID, scope+":"+id, []byte(raw))
	if err != nil {
		return fmt.Errorf("protect identity delivery credential: %w", err)
	}
	token := models.IdentityCredentialToken{ID: id, OrganizationID: request.OrganizationID, Email: request.Email,
		TokenHash: hash, DeliveryCipher: ciphertext, IssuedAt: now, ExpiresAt: now.Add(ttl), Metadata: sanitizeIdentityMetadata(metadata)}
	if emailVerification {
		_, err = s.store.IssueEmailVerification(ctx, token)
	} else {
		_, err = s.store.IssuePasswordReset(ctx, token)
	}
	if err != nil {
		return mapIdentityStoreError(err)
	}
	return nil
}

func (s *IdentityLifecycleService) VerifyEmail(ctx context.Context, input models.IdentityTokenInput, metadata models.IdentityRequestMetadata) error {
	organizationID, tokenHash, err := parseOpaqueIdentityCredential(input.Token)
	if err != nil {
		return ErrIdentityCredential
	}
	if err := s.rateLimit(ctx, "email_verify", tokenHash+":"+metadata.IPAddress, 8); err != nil {
		return err
	}
	return mapIdentityCredentialError(s.store.VerifyEmail(ctx, organizationID, tokenHash, s.now().UTC(), sanitizeIdentityMetadata(metadata)))
}

func (s *IdentityLifecycleService) ResetPassword(ctx context.Context, input models.IdentityPasswordResetInput, metadata models.IdentityRequestMetadata) error {
	organizationID, tokenHash, err := parseOpaqueIdentityCredential(input.Token)
	if err != nil || !validNewIdentityPassword(input.NewPassword) {
		return ErrIdentityCredential
	}
	if err := s.rateLimit(ctx, "password_reset_complete", tokenHash+":"+metadata.IPAddress, 8); err != nil {
		return err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), s.bcryptCost)
	if err != nil {
		return fmt.Errorf("hash reset password: %w", err)
	}
	return mapIdentityCredentialError(s.store.ResetPassword(ctx, organizationID, tokenHash, string(passwordHash),
		s.now().UTC(), sanitizeIdentityMetadata(metadata)))
}

func (s *IdentityLifecycleService) ListSessions(ctx context.Context, organizationID, userID, accessToken string) ([]models.IdentitySession, error) {
	if !identityUUID(organizationID) || !identityUUID(userID) || accessToken == "" {
		return nil, ErrIdentityInvalid
	}
	items, err := s.store.ListSessions(ctx, organizationID, userID, identityTokenHash(accessToken))
	return items, mapIdentityStoreError(err)
}

func (s *IdentityLifecycleService) RevokeSession(ctx context.Context, organizationID, userID, actorID, sessionID string, accessToken string, input models.IdentitySessionRevokeInput, metadata models.IdentityRequestMetadata) error {
	input.Reason = strings.TrimSpace(input.Reason)
	if !identityUUID(organizationID) || !identityUUID(userID) || actorID != userID || !identityUUID(sessionID) ||
		input.ExpectedVersion < 1 || !validIdentityReason(input.Reason) || accessToken == "" {
		return ErrIdentityInvalid
	}
	return mapIdentityStoreError(s.store.RevokeSession(ctx, organizationID, userID, sessionID, actorID,
		input.ExpectedVersion, input.Reason, sanitizeIdentityMetadata(metadata)))
}

func (s *IdentityLifecycleService) GlobalSignOut(ctx context.Context, organizationID, userID, actorID, accessToken string, input models.IdentityGlobalSignOutInput, metadata models.IdentityRequestMetadata) (int, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if !identityUUID(organizationID) || !identityUUID(userID) || actorID != userID || accessToken == "" || !validIdentityReason(input.Reason) {
		return 0, ErrIdentityInvalid
	}
	count, err := s.store.RevokeAllSessions(ctx, organizationID, userID, actorID, identityTokenHash(accessToken),
		input.ExceptCurrent, input.Reason, sanitizeIdentityMetadata(metadata))
	return count, mapIdentityStoreError(err)
}

func (s *IdentityLifecycleService) ListEvents(ctx context.Context, organizationID, userID string, pagination models.PaginationRequest) ([]models.IdentitySecurityEvent, int, error) {
	if !identityUUID(organizationID) || userID != "" && !identityUUID(userID) {
		return nil, 0, ErrIdentityInvalid
	}
	pagination = normalizeIdentityPagination(pagination)
	items, total, err := s.store.ListEvents(ctx, organizationID, userID, pagination)
	return items, total, mapIdentityStoreError(err)
}

func (s *IdentityLifecycleService) newOpaqueCredential(organizationID string) (string, string, error) {
	secret := make([]byte, 32)
	if _, err := io.ReadFull(s.random, secret); err != nil {
		return "", "", fmt.Errorf("generate identity credential: %w", err)
	}
	raw := "v1." + organizationID + "." + base64.RawURLEncoding.EncodeToString(secret)
	return raw, identityTokenHash(raw), nil
}

func parseOpaqueIdentityCredential(raw string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) != 3 || parts[0] != "v1" || !identityUUID(parts[1]) {
		return "", "", ErrIdentityCredential
	}
	secret, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(secret) != 32 {
		return "", "", ErrIdentityCredential
	}
	return parts[1], identityTokenHash(raw), nil
}

func (s *IdentityLifecycleService) rateLimit(ctx context.Context, scope, material string, limit int) error {
	digest := sha256.Sum256([]byte("identity:" + scope + ":" + material))
	allowed, _, err := s.limiter.Allow(ctx, hex.EncodeToString(digest[:]), limit)
	if err != nil {
		return ErrIdentityUnavailable
	}
	if !allowed {
		return ErrIdentityRateLimited
	}
	return nil
}

func identityTokenHash(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func validNewIdentityPassword(value string) bool {
	return len(value) >= 12 && len(value) <= 72 && utf8.ValidString(value) && strings.TrimSpace(value) == value
}

func validIdentityReason(value string) bool {
	length := utf8.RuneCountInString(value)
	return length >= 3 && length <= 1000 && value == strings.TrimSpace(value)
}

func normalizeIdentityServiceEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validIdentityEmail(value string) bool {
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value && len(value) <= 320
}

func identityUUID(value string) bool { _, err := uuid.Parse(value); return err == nil }

func sanitizeIdentityMetadata(metadata models.IdentityRequestMetadata) models.IdentityRequestMetadata {
	metadata.RequestID = strings.TrimSpace(metadata.RequestID)
	metadata.IPAddress = strings.TrimSpace(metadata.IPAddress)
	if metadata.IPAddress != "" && net.ParseIP(metadata.IPAddress) == nil {
		metadata.IPAddress = ""
	}
	metadata.UserAgent = strings.ToValidUTF8(metadata.UserAgent, "")
	metadata.DeviceName = strings.ToValidUTF8(metadata.DeviceName, "")
	metadata.UserAgent = strings.TrimSpace(metadata.UserAgent)
	if len(metadata.RequestID) > 255 {
		metadata.RequestID = metadata.RequestID[:255]
	}
	metadata.UserAgent = truncateIdentityRunes(metadata.UserAgent, 500)
	metadata.DeviceName = truncateIdentityRunes(metadata.DeviceName, 120)
	return metadata
}

func truncateIdentityRunes(value string, maximum int) string {
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return string(runes[:maximum])
}

func defaultIdentityPolicy(organizationID string) *models.IdentityPolicy {
	return &models.IdentityPolicy{OrganizationID: organizationID, RequireMFAForAdmins: true,
		AllowedMethods: []string{"totp", "passkey"}, EnrollmentGraceHours: 24,
		AuthenticationChallengeMins: 5, StepUpTTLMinutes: 10, Version: 0}
}

func normalizeIdentityMethods(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if (value == "totp" || value == "passkey") && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func normalizeIdentityPagination(value models.PaginationRequest) models.PaginationRequest {
	if value.Page < 1 {
		value.Page = 1
	}
	if value.PageSize < 1 || value.PageSize > 100 {
		value.PageSize = 20
	}
	return value
}

func mapIdentityCredentialError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, repository.ErrIdentityNotFound) || errors.Is(err, repository.ErrIdentityExpired) ||
		errors.Is(err, repository.ErrIdentityConsumed) || errors.Is(err, repository.ErrIdentityAttempts) {
		return ErrIdentityCredential
	}
	return mapIdentityStoreError(err)
}

func mapIdentityStoreError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repository.ErrIdentityNotFound):
		return ErrIdentityCredential
	case errors.Is(err, repository.ErrIdentityVersion):
		return ErrIdentityVersionConflict
	case errors.Is(err, repository.ErrIdentityLastFactor):
		return ErrIdentityLastFactor
	case errors.Is(err, repository.ErrIdentityAdminRecovery):
		return ErrIdentityAdminRecovery
	case errors.Is(err, repository.ErrIdentityConflict):
		return ErrIdentityState
	default:
		return err
	}
}

var identityStepUpPurposePattern = regexp.MustCompile(`^[a-z][a-z0-9_.:-]{2,79}$`)
