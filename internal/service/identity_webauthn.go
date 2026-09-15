package service

import (
	"encoding/json"
	"fmt"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/complianceforge/platform/internal/config"
)

// IdentityWebAuthnVerifier keeps ceremony verification replaceable in tests
// while production uses the FIDO2-conformance-tested go-webauthn library.
type IdentityWebAuthnVerifier interface {
	BeginRegistration(IdentityWebAuthnUser) (options json.RawMessage, session json.RawMessage, err error)
	FinishRegistration(IdentityWebAuthnUser, json.RawMessage, json.RawMessage) (*webauthn.Credential, error)
	BeginAuthentication(IdentityWebAuthnUser) (options json.RawMessage, session json.RawMessage, err error)
	BeginDummyAuthentication() (options json.RawMessage, err error)
	FinishAuthentication(IdentityWebAuthnUser, json.RawMessage, json.RawMessage) (*webauthn.Credential, error)
}

type IdentityWebAuthnUser struct {
	ID          []byte
	Name        string
	DisplayName string
	Credentials []webauthn.Credential
}

func (u IdentityWebAuthnUser) WebAuthnID() []byte                         { return u.ID }
func (u IdentityWebAuthnUser) WebAuthnName() string                       { return u.Name }
func (u IdentityWebAuthnUser) WebAuthnDisplayName() string                { return u.DisplayName }
func (u IdentityWebAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

type webAuthnIdentityVerifier struct{ relyingParty *webauthn.WebAuthn }

func NewIdentityWebAuthnVerifier(cfg config.IdentityConfig) (IdentityWebAuthnVerifier, error) {
	rp, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.RPID,
		RPDisplayName: cfg.RPDisplayName,
		RPOrigins:     append([]string(nil), cfg.RPOrigins...),
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationRequired,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("configure WebAuthn verifier: %w", err)
	}
	return &webAuthnIdentityVerifier{relyingParty: rp}, nil
}

func (v *webAuthnIdentityVerifier) BeginRegistration(user IdentityWebAuthnUser) (json.RawMessage, json.RawMessage, error) {
	options, session, err := v.relyingParty.BeginRegistration(user)
	return marshalWebAuthnCeremony(options, session, err)
}

func (v *webAuthnIdentityVerifier) FinishRegistration(user IdentityWebAuthnUser, rawSession, response json.RawMessage) (*webauthn.Credential, error) {
	var session webauthn.SessionData
	if err := json.Unmarshal(rawSession, &session); err != nil {
		return nil, fmt.Errorf("decode WebAuthn registration state: %w", err)
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
	if err != nil {
		return nil, fmt.Errorf("parse WebAuthn registration response: %w", err)
	}
	credential, err := v.relyingParty.CreateCredential(user, session, parsed)
	if err != nil {
		return nil, fmt.Errorf("verify WebAuthn registration response: %w", err)
	}
	return credential, nil
}

func (v *webAuthnIdentityVerifier) BeginAuthentication(user IdentityWebAuthnUser) (json.RawMessage, json.RawMessage, error) {
	options, session, err := v.relyingParty.BeginLogin(user)
	return marshalWebAuthnCeremony(options, session, err)
}

func (v *webAuthnIdentityVerifier) BeginDummyAuthentication() (json.RawMessage, error) {
	options, _, err := v.relyingParty.BeginDiscoverableLogin()
	if err != nil {
		return nil, fmt.Errorf("begin opaque WebAuthn authentication: %w", err)
	}
	encoded, err := json.Marshal(options)
	if err != nil {
		return nil, fmt.Errorf("encode opaque WebAuthn options: %w", err)
	}
	return encoded, nil
}

func (v *webAuthnIdentityVerifier) FinishAuthentication(user IdentityWebAuthnUser, rawSession, response json.RawMessage) (*webauthn.Credential, error) {
	var session webauthn.SessionData
	if err := json.Unmarshal(rawSession, &session); err != nil {
		return nil, fmt.Errorf("decode WebAuthn authentication state: %w", err)
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return nil, fmt.Errorf("parse WebAuthn authentication response: %w", err)
	}
	credential, err := v.relyingParty.ValidateLogin(user, session, parsed)
	if err != nil {
		return nil, fmt.Errorf("verify WebAuthn authentication response: %w", err)
	}
	return credential, nil
}

func marshalWebAuthnCeremony(options, session any, ceremonyErr error) (json.RawMessage, json.RawMessage, error) {
	if ceremonyErr != nil {
		return nil, nil, ceremonyErr
	}
	encodedOptions, err := json.Marshal(options)
	if err != nil {
		return nil, nil, fmt.Errorf("encode WebAuthn options: %w", err)
	}
	encodedSession, err := json.Marshal(session)
	if err != nil {
		return nil, nil, fmt.Errorf("encode WebAuthn state: %w", err)
	}
	return encodedOptions, encodedSession, nil
}
