package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	authdomain "github.com/complianceforge/platform/internal/auth"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
	protocol "github.com/complianceforge/platform/internal/scim"
)

var (
	ErrSCIMInvalid       = errors.New("SCIM request is invalid")
	ErrSCIMNotFound      = errors.New("SCIM resource not found")
	ErrSCIMConflict      = errors.New("SCIM resource conflicts with existing data")
	ErrSCIMVersion       = errors.New("SCIM resource version conflict")
	ErrSCIMLastAdmin     = errors.New("SCIM operation would remove the last administrator")
	ErrSCIMDynamicGroup  = errors.New("SCIM cannot mutate rule-managed group memberships")
	ErrSCIMTokenInvalid  = errors.New("SCIM token request is invalid")
	ErrSCIMTokenNotFound = errors.New("SCIM token not found")
	ErrSCIMTokenConflict = errors.New("SCIM token state conflict")
)

type SCIMStore interface {
	CreateSCIMToken(context.Context, string, string, *models.SCIMToken, string) (*models.SCIMToken, error)
	RotateSCIMToken(context.Context, string, string, string, int64, string, string, string, *time.Time) (*models.SCIMToken, error)
	RevokeSCIMToken(context.Context, string, string, string, int64, string) error
	ListSCIMTokens(context.Context, string, models.PaginationRequest) ([]models.SCIMToken, int, error)
	ResolveSCIMTokenTenant(context.Context, string) (string, string, error)
	GetSCIMTokenForAuthentication(context.Context, string, string, string) (*models.SCIMToken, error)
	RecordSCIMTokenUse(context.Context, string, string, string, int64, string, time.Time) error

	CreateSCIMUser(context.Context, string, string, string, *models.SCIMUser) (*models.SCIMUser, error)
	GetSCIMUser(context.Context, string, string) (*models.SCIMUser, error)
	ListSCIMUsers(context.Context, string, models.SCIMListRequest, protocol.Filter) ([]models.SCIMUser, int, error)
	ReplaceSCIMUser(context.Context, string, string, string, string, int64, string, *models.SCIMUser) (*models.SCIMUser, error)
	DeleteSCIMUser(context.Context, string, string, string, string, int64) error
	CreateSCIMGroup(context.Context, string, string, string, *models.SCIMGroup) (*models.SCIMGroup, error)
	GetSCIMGroup(context.Context, string, string) (*models.SCIMGroup, error)
	ListSCIMGroups(context.Context, string, models.SCIMListRequest, protocol.Filter) ([]models.SCIMGroup, int, error)
	ReplaceSCIMGroup(context.Context, string, string, string, string, int64, string, *models.SCIMGroup) (*models.SCIMGroup, error)
	DeleteSCIMGroup(context.Context, string, string, string, string, int64) error
}

type SCIMService struct {
	store  SCIMStore
	logger zerolog.Logger
	now    func() time.Time
	random io.Reader
}

type SCIMServiceOption func(*SCIMService)

var (
	_ SCIMStore                         = repository.SCIMRepository(nil)
	_ authdomain.SCIMTokenAuthenticator = (*SCIMService)(nil)
)

func NewSCIMService(store SCIMStore, logger zerolog.Logger, options ...SCIMServiceOption) (*SCIMService, error) {
	if store == nil {
		return nil, errors.New("SCIM store is required")
	}
	service := &SCIMService{store: store, logger: logger.With().Str("service", "scim").Logger(), now: func() time.Time { return time.Now().UTC() }, random: rand.Reader}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	if service.now == nil || service.random == nil {
		return nil, errors.New("SCIM service options are invalid")
	}
	return service, nil
}

func WithSCIMClock(clock func() time.Time) SCIMServiceOption {
	return func(service *SCIMService) { service.now = clock }
}

func WithSCIMRandom(reader io.Reader) SCIMServiceOption {
	return func(service *SCIMService) { service.random = reader }
}

func (s *SCIMService) CreateToken(ctx context.Context, organizationID, actorID string, input models.SCIMTokenCreateInput) (*models.SCIMTokenIssueResult, error) {
	if !validRequestedSCIMScopes(input.Scopes) {
		return nil, ErrSCIMTokenInvalid
	}
	normalizeSCIMTokenCreate(&input)
	if !scimUUID(organizationID) || !scimUUID(actorID) || !validSCIMTokenCreate(input, s.now()) {
		return nil, ErrSCIMTokenInvalid
	}
	plain, prefix, digest, err := s.generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate SCIM token: %w", err)
	}
	token := &models.SCIMToken{ID: uuid.NewString(), OrganizationID: organizationID, Name: input.Name,
		Prefix: prefix, TokenHash: digest, Scopes: input.Scopes, RateLimitPerMinute: input.RateLimitPerMinute,
		ExpiresAt: input.ExpiresAt, Active: true, CreatedBy: actorID, Version: 1}
	created, err := s.store.CreateSCIMToken(ctx, organizationID, actorID, token, input.Reason)
	if err != nil {
		return nil, mapSCIMTokenStoreError(err)
	}
	return &models.SCIMTokenIssueResult{Token: *created, Credential: plain}, nil
}

func (s *SCIMService) RotateToken(ctx context.Context, organizationID, tokenID, actorID string, input models.SCIMTokenRotateInput) (*models.SCIMTokenIssueResult, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if !scimUUID(organizationID) || !scimUUID(tokenID) || !scimUUID(actorID) || input.ExpectedVersion < 1 ||
		!validSCIMReason(input.Reason) || input.ExpiresAt != nil && !input.ExpiresAt.After(s.now().Add(time.Minute)) {
		return nil, ErrSCIMTokenInvalid
	}
	plain, prefix, digest, err := s.generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate rotated SCIM token: %w", err)
	}
	rotated, err := s.store.RotateSCIMToken(ctx, organizationID, tokenID, actorID, input.ExpectedVersion,
		prefix, digest, input.Reason, input.ExpiresAt)
	if err != nil {
		return nil, mapSCIMTokenStoreError(err)
	}
	return &models.SCIMTokenIssueResult{Token: *rotated, Credential: plain}, nil
}

func (s *SCIMService) RevokeToken(ctx context.Context, organizationID, tokenID, actorID string, input models.SCIMTokenRevokeInput) error {
	input.Reason = strings.TrimSpace(input.Reason)
	if !scimUUID(organizationID) || !scimUUID(tokenID) || !scimUUID(actorID) || input.ExpectedVersion < 1 || !validSCIMReason(input.Reason) {
		return ErrSCIMTokenInvalid
	}
	return mapSCIMTokenStoreError(s.store.RevokeSCIMToken(ctx, organizationID, tokenID, actorID, input.ExpectedVersion, input.Reason))
}

func (s *SCIMService) ListTokens(ctx context.Context, organizationID string, pagination models.PaginationRequest) ([]models.SCIMToken, int, error) {
	if !scimUUID(organizationID) {
		return nil, 0, ErrSCIMTokenInvalid
	}
	pagination = normalizeSCIMPagination(pagination)
	items, total, err := s.store.ListSCIMTokens(ctx, organizationID, pagination)
	if err != nil {
		return nil, 0, mapSCIMTokenStoreError(err)
	}
	return items, total, nil
}

func (s *SCIMService) AuthenticateSCIMToken(ctx context.Context, rawToken, clientIP string) (*authdomain.SCIMPrincipal, error) {
	if len(rawToken) < 20 || len(rawToken) > 256 || !strings.HasPrefix(rawToken, "cfs_") {
		return nil, authdomain.ErrInvalidSCIMToken
	}
	prefix := rawToken
	if len(prefix) > 16 {
		prefix = prefix[:16]
	}
	tokenID, organizationID, err := s.store.ResolveSCIMTokenTenant(ctx, prefix)
	if err != nil {
		if errors.Is(err, repository.ErrSCIMNotFound) {
			return nil, authdomain.ErrInvalidSCIMToken
		}
		return nil, err
	}
	token, err := s.store.GetSCIMTokenForAuthentication(ctx, organizationID, tokenID, prefix)
	if err != nil {
		if errors.Is(err, repository.ErrSCIMNotFound) {
			return nil, authdomain.ErrInvalidSCIMToken
		}
		return nil, err
	}
	digest := sha256.Sum256([]byte(rawToken))
	expected, decodeErr := hex.DecodeString(token.TokenHash)
	if decodeErr != nil || len(expected) != sha256.Size || subtle.ConstantTimeCompare(digest[:], expected) != 1 ||
		!token.Active || token.RevokedAt != nil || token.ExpiresAt != nil && !token.ExpiresAt.After(s.now()) {
		return nil, authdomain.ErrInvalidSCIMToken
	}
	if net.ParseIP(strings.TrimSpace(clientIP)) == nil {
		clientIP = ""
	}
	// Bind the usage write to the exact digest and version just verified. A
	// simultaneous rotate/revoke therefore makes the old credential fail rather
	// than allowing one last request through a time-of-check/time-of-use race.
	if err := s.store.RecordSCIMTokenUse(ctx, organizationID, token.ID, token.TokenHash,
		token.Version, clientIP, s.now()); err != nil {
		if errors.Is(err, repository.ErrSCIMNotFound) {
			return nil, authdomain.ErrInvalidSCIMToken
		}
		return nil, err
	}
	return &authdomain.SCIMPrincipal{TokenID: token.ID, OrganizationID: organizationID,
		CreatedByUserID: token.CreatedBy, Scopes: append([]string(nil), token.Scopes...), RateLimitPerMinute: token.RateLimitPerMinute}, nil
}

func (s *SCIMService) ListUsers(ctx context.Context, organizationID string, request models.SCIMListRequest) ([]models.SCIMUser, int, error) {
	if !scimUUID(organizationID) {
		return nil, 0, ErrSCIMInvalid
	}
	filter, err := protocol.ParseFilter(request.Filter, protocol.ResourceUsers)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrSCIMInvalid, err)
	}
	items, total, err := s.store.ListSCIMUsers(ctx, organizationID, request, filter)
	if err != nil {
		return nil, 0, mapSCIMStoreError(err)
	}
	for index := range items {
		finalizeSCIMUser(&items[index])
	}
	return items, total, nil
}

func (s *SCIMService) GetUser(ctx context.Context, organizationID, userID string) (*models.SCIMUser, error) {
	if !scimUUID(organizationID) || !scimUUID(userID) {
		return nil, ErrSCIMInvalid
	}
	item, err := s.store.GetSCIMUser(ctx, organizationID, userID)
	if err != nil {
		return nil, mapSCIMStoreError(err)
	}
	finalizeSCIMUser(item)
	return item, nil
}

func (s *SCIMService) CreateUser(ctx context.Context, organizationID, tokenID, actorID string, user *models.SCIMUser) (*models.SCIMUser, error) {
	if !validSCIMActor(organizationID, tokenID, actorID) {
		return nil, ErrSCIMInvalid
	}
	if err := protocol.NormalizeUser(user); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSCIMInvalid, err)
	}
	item, err := s.store.CreateSCIMUser(ctx, organizationID, tokenID, actorID, user)
	if err != nil {
		return nil, mapSCIMStoreError(err)
	}
	finalizeSCIMUser(item)
	return item, nil
}

func (s *SCIMService) ReplaceUser(ctx context.Context, organizationID, userID, tokenID, actorID string, expectedVersion int64, user *models.SCIMUser) (*models.SCIMUser, error) {
	if !validSCIMActor(organizationID, tokenID, actorID) || !scimUUID(userID) || expectedVersion < 1 {
		return nil, ErrSCIMInvalid
	}
	if err := protocol.NormalizeUser(user); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSCIMInvalid, err)
	}
	item, err := s.store.ReplaceSCIMUser(ctx, organizationID, userID, tokenID, actorID, expectedVersion, "replaced", user)
	if err != nil {
		return nil, mapSCIMStoreError(err)
	}
	finalizeSCIMUser(item)
	return item, nil
}

func (s *SCIMService) PatchUser(ctx context.Context, organizationID, userID, tokenID, actorID string, expectedVersion int64, patch models.SCIMPatchRequest) (*models.SCIMUser, error) {
	if err := protocol.ValidatePatchRequest(patch); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSCIMInvalid, err)
	}
	current, err := s.GetUser(ctx, organizationID, userID)
	if err != nil {
		return nil, err
	}
	if err := applySCIMUserPatch(current, patch); err != nil {
		return nil, err
	}
	if err := protocol.NormalizeUser(current); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSCIMInvalid, err)
	}
	item, err := s.store.ReplaceSCIMUser(ctx, organizationID, userID, tokenID, actorID, expectedVersion, "patched", current)
	if err != nil {
		return nil, mapSCIMStoreError(err)
	}
	finalizeSCIMUser(item)
	return item, nil
}

func (s *SCIMService) DeleteUser(ctx context.Context, organizationID, userID, tokenID, actorID string, expectedVersion int64) error {
	if !validSCIMActor(organizationID, tokenID, actorID) || !scimUUID(userID) || expectedVersion < 1 {
		return ErrSCIMInvalid
	}
	return mapSCIMStoreError(s.store.DeleteSCIMUser(ctx, organizationID, userID, tokenID, actorID, expectedVersion))
}

func (s *SCIMService) ListGroups(ctx context.Context, organizationID string, request models.SCIMListRequest) ([]models.SCIMGroup, int, error) {
	if !scimUUID(organizationID) {
		return nil, 0, ErrSCIMInvalid
	}
	filter, err := protocol.ParseFilter(request.Filter, protocol.ResourceGroups)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrSCIMInvalid, err)
	}
	items, total, err := s.store.ListSCIMGroups(ctx, organizationID, request, filter)
	if err != nil {
		return nil, 0, mapSCIMStoreError(err)
	}
	for index := range items {
		finalizeSCIMGroup(&items[index])
	}
	return items, total, nil
}

func (s *SCIMService) GetGroup(ctx context.Context, organizationID, groupID string) (*models.SCIMGroup, error) {
	if !scimUUID(organizationID) || !scimUUID(groupID) {
		return nil, ErrSCIMInvalid
	}
	item, err := s.store.GetSCIMGroup(ctx, organizationID, groupID)
	if err != nil {
		return nil, mapSCIMStoreError(err)
	}
	finalizeSCIMGroup(item)
	return item, nil
}

func (s *SCIMService) CreateGroup(ctx context.Context, organizationID, tokenID, actorID string, group *models.SCIMGroup) (*models.SCIMGroup, error) {
	if !validSCIMActor(organizationID, tokenID, actorID) {
		return nil, ErrSCIMInvalid
	}
	if err := protocol.NormalizeGroup(group); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSCIMInvalid, err)
	}
	item, err := s.store.CreateSCIMGroup(ctx, organizationID, tokenID, actorID, group)
	if err != nil {
		return nil, mapSCIMStoreError(err)
	}
	finalizeSCIMGroup(item)
	return item, nil
}

func (s *SCIMService) ReplaceGroup(ctx context.Context, organizationID, groupID, tokenID, actorID string, expectedVersion int64, group *models.SCIMGroup) (*models.SCIMGroup, error) {
	if !validSCIMActor(organizationID, tokenID, actorID) || !scimUUID(groupID) || expectedVersion < 1 {
		return nil, ErrSCIMInvalid
	}
	if err := protocol.NormalizeGroup(group); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSCIMInvalid, err)
	}
	item, err := s.store.ReplaceSCIMGroup(ctx, organizationID, groupID, tokenID, actorID, expectedVersion, "replaced", group)
	if err != nil {
		return nil, mapSCIMStoreError(err)
	}
	finalizeSCIMGroup(item)
	return item, nil
}

func (s *SCIMService) PatchGroup(ctx context.Context, organizationID, groupID, tokenID, actorID string, expectedVersion int64, patch models.SCIMPatchRequest) (*models.SCIMGroup, error) {
	if err := protocol.ValidatePatchRequest(patch); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSCIMInvalid, err)
	}
	current, err := s.GetGroup(ctx, organizationID, groupID)
	if err != nil {
		return nil, err
	}
	if err := applySCIMGroupPatch(current, patch); err != nil {
		return nil, err
	}
	if err := protocol.NormalizeGroup(current); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSCIMInvalid, err)
	}
	item, err := s.store.ReplaceSCIMGroup(ctx, organizationID, groupID, tokenID, actorID, expectedVersion, "patched", current)
	if err != nil {
		return nil, mapSCIMStoreError(err)
	}
	finalizeSCIMGroup(item)
	return item, nil
}

func (s *SCIMService) DeleteGroup(ctx context.Context, organizationID, groupID, tokenID, actorID string, expectedVersion int64) error {
	if !validSCIMActor(organizationID, tokenID, actorID) || !scimUUID(groupID) || expectedVersion < 1 {
		return ErrSCIMInvalid
	}
	return mapSCIMStoreError(s.store.DeleteSCIMGroup(ctx, organizationID, groupID, tokenID, actorID, expectedVersion))
}

func (s *SCIMService) generateToken() (string, string, string, error) {
	secret := make([]byte, 32)
	if _, err := io.ReadFull(s.random, secret); err != nil {
		return "", "", "", err
	}
	plain := "cfs_" + base64.RawURLEncoding.EncodeToString(secret)
	prefix := plain[:16]
	digest := sha256.Sum256([]byte(plain))
	return plain, prefix, hex.EncodeToString(digest[:]), nil
}

func normalizeSCIMTokenCreate(input *models.SCIMTokenCreateInput) {
	input.Name = strings.TrimSpace(input.Name)
	input.Reason = strings.TrimSpace(input.Reason)
	if input.RateLimitPerMinute == 0 {
		input.RateLimitPerMinute = 120
	}
	seen := make(map[string]struct{}, len(input.Scopes))
	result := make([]string, 0, len(input.Scopes))
	for _, scope := range input.Scopes {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if models.SCIMAllowedTokenScopes[scope] {
			if _, duplicate := seen[scope]; !duplicate {
				seen[scope] = struct{}{}
				result = append(result, scope)
			}
		}
	}
	sort.Strings(result)
	input.Scopes = result
}

func validRequestedSCIMScopes(scopes []string) bool {
	if len(scopes) == 0 || len(scopes) > len(models.SCIMAllowedTokenScopes) {
		return false
	}
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if !models.SCIMAllowedTokenScopes[scope] {
			return false
		}
		if _, duplicate := seen[scope]; duplicate {
			return false
		}
		seen[scope] = struct{}{}
	}
	return true
}

func validSCIMTokenCreate(input models.SCIMTokenCreateInput, now time.Time) bool {
	nameLength := utf8.RuneCountInString(input.Name)
	return nameLength >= 2 && nameLength <= 120 && len(input.Scopes) > 0 && input.RateLimitPerMinute >= 1 &&
		input.RateLimitPerMinute <= 10_000 && validSCIMReason(input.Reason) &&
		(input.ExpiresAt == nil || input.ExpiresAt.After(now.Add(time.Minute)))
}

func validSCIMActor(organizationID, tokenID, actorID string) bool {
	return scimUUID(organizationID) && scimUUID(tokenID) && scimUUID(actorID)
}

func validSCIMReason(value string) bool {
	length := utf8.RuneCountInString(value)
	return value == strings.TrimSpace(value) && utf8.ValidString(value) && length >= 3 && length <= 1000
}

func normalizeSCIMPagination(pagination models.PaginationRequest) models.PaginationRequest {
	if pagination.Page < 1 {
		pagination.Page = 1
	}
	if pagination.PageSize < 1 {
		pagination.PageSize = 20
	}
	if pagination.PageSize > 100 {
		pagination.PageSize = 100
	}
	return pagination
}

func finalizeSCIMUser(item *models.SCIMUser) {
	if item == nil {
		return
	}
	item.Schemas = []string{models.SCIMUserSchema}
	if item.Enterprise != nil {
		item.Schemas = append(item.Schemas, models.SCIMEnterpriseUserSchema)
	}
	item.Meta = models.SCIMMeta{ResourceType: "User", Created: item.Meta.Created, LastModified: item.Meta.LastModified,
		Location: "/api/scim/v2/Users/" + item.ID, Version: protocol.VersionETag(item.Version)}
	if item.Groups == nil {
		item.Groups = []models.SCIMGroupReference{}
	}
}

func finalizeSCIMGroup(item *models.SCIMGroup) {
	if item == nil {
		return
	}
	item.Schemas = []string{models.SCIMGroupSchema}
	item.Meta = models.SCIMMeta{ResourceType: "Group", Created: item.Meta.Created, LastModified: item.Meta.LastModified,
		Location: "/api/scim/v2/Groups/" + item.ID, Version: protocol.VersionETag(item.Version)}
	if item.Members == nil {
		item.Members = []models.SCIMMember{}
	}
}

func mapSCIMStoreError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrSCIMNotFound):
		return ErrSCIMNotFound
	case errors.Is(err, repository.ErrSCIMConflict):
		return ErrSCIMConflict
	case errors.Is(err, repository.ErrSCIMVersion):
		return ErrSCIMVersion
	case errors.Is(err, repository.ErrSCIMLastAdmin):
		return ErrSCIMLastAdmin
	case errors.Is(err, repository.ErrSCIMDynamicGroup):
		return ErrSCIMDynamicGroup
	case errors.Is(err, repository.ErrEntitlementLimitExceeded):
		return fmt.Errorf("%w: user capacity is exhausted", ErrSubscriptionLimitExceeded)
	default:
		return err
	}
}

func mapSCIMTokenStoreError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrSCIMNotFound):
		return ErrSCIMTokenNotFound
	case errors.Is(err, repository.ErrSCIMConflict), errors.Is(err, repository.ErrSCIMVersion):
		return ErrSCIMTokenConflict
	default:
		return err
	}
}

func scimUUID(value string) bool { _, err := uuid.Parse(value); return err == nil }
