package service

import (
	"math"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/models"
)

func TestIdentityTOTPCodeRejectsNegativeMovingFactors(t *testing.T) {
	secret := []byte("12345678901234567890") // RFC 4226's public interoperability test key.
	for _, step := range []int64{math.MinInt64, -2, -1} {
		if got := identityTOTPCode(secret, step); got != "" {
			t.Fatalf("negative moving factor %d produced an OTP", step)
		}
	}
	if got := identityTOTPCode(secret, math.MaxInt64); len(got) != 6 {
		t.Fatal("largest nonnegative signed moving factor was rejected")
	}
}

func TestIdentityTOTPCodePreservesRFC4226HMACSHA1Vectors(t *testing.T) {
	secret := []byte("12345678901234567890") // RFC 4226 Appendix D; not an application credential.
	want := []string{"755224", "287082", "359152", "969429", "338314", "254676", "287922", "162583", "399871", "520489"}
	for step, code := range want {
		if got := identityTOTPCode(secret, int64(step)); got != code {
			t.Fatalf("moving factor %d: got %s, want %s", step, got, code)
		}
	}
}

func TestIdentityTOTPValidationRejectsPreEpochClockAndUsedSteps(t *testing.T) {
	secret := []byte("12345678901234567890")
	store := &identityServiceStoreStub{}
	service := newIdentityServiceForTest(t, store, time.Unix(0, 0).UTC())
	ciphertext, err := service.protector.Seal(identityTestOrg, "totp:"+identityTestUser, []byte("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"))
	if err != nil {
		t.Fatal(err)
	}
	factor := &models.IdentityMFAFactor{OrganizationID: identityTestOrg, UserID: identityTestUser, SecretCipher: []byte(ciphertext)}
	code := identityTOTPCode(secret, 0)
	for _, seconds := range []int64{-31, -30, -1} {
		service.now = func() time.Time { return time.Unix(seconds, 0).UTC() }
		if _, valid := service.validateTOTP(factor, code); valid {
			t.Fatalf("pre-epoch clock %d accepted a nonnegative OTP", seconds)
		}
	}
	service.now = func() time.Time { return time.Unix(0, 0).UTC() }
	if step, valid := service.validateTOTP(factor, code); !valid || step != 0 {
		t.Fatalf("epoch OTP: step=%d valid=%v", step, valid)
	}
	used := int64(0)
	factor.LastTOTPStep = &used
	if _, valid := service.validateTOTP(factor, code); valid {
		t.Fatal("already used epoch OTP was accepted")
	}
}
