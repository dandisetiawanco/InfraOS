package security

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"strings"
	"testing"
	"time"
)

func TestPasswordHashingAndVerification(t *testing.T) {
	hash, err := HashPassword("CorrectHorseBattery99!")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if !strings.HasPrefix(hash, "argon2id$") {
		t.Fatalf("unexpected hash prefix: %s", hash)
	}
	if !VerifyPassword("CorrectHorseBattery99!", hash) {
		t.Fatal("expected valid password to verify")
	}
	if VerifyPassword("WrongPassword99!", hash) {
		t.Fatal("expected wrong password to fail verification")
	}
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("expected short password to be rejected")
	}
}

func TestSecretEncryptionAndMasking(t *testing.T) {
	enc, err := EncryptSecret("test-master-key", "ghp_secretTokenValue1234567890")
	if err != nil {
		t.Fatalf("EncryptSecret failed: %v", err)
	}
	if strings.Contains(enc, "ghp_secret") {
		t.Fatal("ciphertext must not contain plaintext")
	}
	dec, err := DecryptSecret("test-master-key", enc)
	if err != nil {
		t.Fatalf("DecryptSecret failed: %v", err)
	}
	if dec != "ghp_secretTokenValue1234567890" {
		t.Fatalf("decrypted mismatch: %q", dec)
	}
	if MaskSecret("ghp_12345678abcd") != "****abcd" {
		t.Fatalf("unexpected mask: %s", MaskSecret("ghp_12345678abcd"))
	}
}

func TestSSRFValidatorBlocksMetadataPrivateAndDemoLocal(t *testing.T) {
	v := NewSSRFValidator([]string{"http://127.0.0.1:8080", "http://localhost:8090"})
	ctx := context.Background()

	blocked := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://100.100.100.200/latest/meta-data/",
		"http://10.0.0.5:8080/health",
		"http://192.168.1.1/admin",
		"http://172.16.0.12/",
		"http://127.0.0.1:9999/not-allowlisted",
		"file:///etc/passwd",
		"http://user:pass@example.com/",
		"http://metadata.google.internal/computeMetadata/v1/",
	}
	for _, raw := range blocked {
		if _, _, err := v.ValidateHTTPURL(ctx, raw, false); err == nil {
			t.Errorf("expected SSRF validator to block %q", raw)
		}
	}

	// Explicitly allowlisted local target allowed for real workspace, blocked for demo workspace.
	if _, _, err := v.ValidateHTTPURL(ctx, "http://127.0.0.1:8080/healthz", false); err != nil {
		t.Errorf("expected allowlisted local target to pass in real workspace: %v", err)
	}
	if _, _, err := v.ValidateHTTPURL(ctx, "http://127.0.0.1:8080/healthz", true); err == nil {
		t.Error("expected demo workspace to be blocked from local allowlisted target")
	}

	// Direct IP policy checks
	if err := ValidateIP(net.ParseIP("169.254.169.254"), true); err == nil {
		t.Error("cloud metadata IP must always be blocked even when allowLocal=true")
	}
	if err := ValidateIP(net.ParseIP("100.64.0.1"), false); err == nil {
		t.Error("CGNAT address must be blocked")
	}
}

func TestGitHubWebhookVerification(t *testing.T) {
	secret := "whsec_test_secret"
	body := []byte(`{"action":"completed"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !VerifyGitHubSignature(secret, validSig, body) {
		t.Fatal("expected valid HMAC signature to verify")
	}
	if VerifyGitHubSignature(secret, validSig, []byte(`{"action":"tampered"}`)) {
		t.Fatal("expected tampered body to fail verification")
	}
	if VerifyGitHubSignature(secret, "sha256=deadbeef", body) {
		t.Fatal("expected invalid signature to fail")
	}
}

func TestCredentialRedaction(t *testing.T) {
	input := "Authorization: Bearer secret_abc123 token=mysecret123 postgres://admin:hunter2@db.local/app ghp_abcdefghijklmnopqrstuvwxyz123456"
	redacted := RedactCredentials(input)
	for _, leak := range []string{"secret_abc123", "mysecret123", "hunter2", "ghp_abcdefghijklmnopqrstuvwxyz123456"} {
		if strings.Contains(redacted, leak) {
			t.Errorf("redacted output still contains %q: %s", leak, redacted)
		}
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !rl.Allow("192.0.2.1") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if rl.Allow("192.0.2.1") {
		t.Fatal("4th attempt should be rate limited")
	}
	if !rl.Allow("192.0.2.2") {
		t.Fatal("different IP should still be allowed")
	}
}
