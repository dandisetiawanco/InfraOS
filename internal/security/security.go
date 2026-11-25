package security

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("argon2id$%d$%d$%d$%s$%s",
		argonTime, argonMemory, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "argon2id" {
		return false
	}
	var t, m uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[1], "%d", &t); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(parts[2], "%d", &m); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(parts[3], "%d", &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func GenerateToken(prefix string, numBytes int) (raw string, hash string, err error) {
	buf := make([]byte, numBytes)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(buf)
	if prefix != "" {
		raw = prefix + "_" + encoded
	} else {
		raw = encoded
	}
	return raw, HashToken(raw), nil
}

func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func EncryptSecret(keyMaterial, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	key := sha256.Sum256([]byte(keyMaterial))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func DecryptSecret(keyMaterial, encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte(keyMaterial))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func MaskSecret(secret string) string {
	s := strings.TrimSpace(secret)
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return "****" + s[len(s)-4:]
}

func VerifyGitHubSignature(secret, signatureHeader string, body []byte) bool {
	if secret == "" || !strings.HasPrefix(signatureHeader, "sha256=") {
		return false
	}
	providedHex := strings.TrimPrefix(signatureHeader, "sha256=")
	provided, err := hex.DecodeString(providedHex)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(provided, expected)
}

var redactionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*(?:bearer|agent|basic)\s+)[^\s"',;]+`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|secret|token|password|passwd|pwd)\s*[:=]\s*["']?)[^\s"',;&]+`),
	regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{20,}\b`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`\binfraos_(?:agt|enr|ses)_[A-Za-z0-9_-]{12,}\b`),
	regexp.MustCompile(`(postgres(?:ql)?://[^:\s]+:)([^@\s]+)(@)`),
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
}

func RedactCredentials(input string) string {
	out := input
	out = redactionPatterns[0].ReplaceAllString(out, "${1}[REDACTED]")
	out = redactionPatterns[1].ReplaceAllString(out, "${1}[REDACTED]")
	out = redactionPatterns[2].ReplaceAllString(out, "[REDACTED_GH_TOKEN]")
	out = redactionPatterns[3].ReplaceAllString(out, "[REDACTED_GH_PAT]")
	out = redactionPatterns[4].ReplaceAllString(out, "[REDACTED_API_KEY]")
	out = redactionPatterns[5].ReplaceAllString(out, "[REDACTED_INFRAOS_TOKEN]")
	out = redactionPatterns[6].ReplaceAllString(out, "${1}[REDACTED]${3}")
	out = redactionPatterns[7].ReplaceAllString(out, "[REDACTED_PRIVATE_KEY]")
	return out
}

// RateLimiter enforces per-key fixed-window request limits.
type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]*rateWindow
}

type rateWindow struct {
	count   int
	resetAt time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = 10
	}
	return &RateLimiter{
		limit:   limit,
		window:  window,
		entries: make(map[string]*rateWindow),
	}
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	if len(rl.entries) > 4096 {
		for k, v := range rl.entries {
			if now.After(v.resetAt) {
				delete(rl.entries, k)
			}
		}
	}
	w, ok := rl.entries[key]
	if !ok || now.After(w.resetAt) {
		rl.entries[key] = &rateWindow{count: 1, resetAt: now.Add(rl.window)}
		return true
	}
	if w.count >= rl.limit {
		return false
	}
	w.count++
	return true
}

// SSRFValidator enforces destination network policies for health checks and webhooks.
type SSRFValidator struct {
	localAllowlist map[string]struct{}
	resolver       *net.Resolver
}

func NewSSRFValidator(localAllowlist []string) *SSRFValidator {
	allowed := make(map[string]struct{}, len(localAllowlist))
	for _, raw := range localAllowlist {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u.Host == "" {
			continue
		}
		allowed[strings.ToLower(u.Scheme+"://"+u.Host)] = struct{}{}
		allowed[strings.ToLower(u.Host)] = struct{}{}
	}
	return &SSRFValidator{
		localAllowlist: allowed,
		resolver:       net.DefaultResolver,
	}
}

func (v *SSRFValidator) isExplicitlyAllowedLocal(scheme, hostPort string, isDemo bool) bool {
	if isDemo {
		return false
	}
	if _, ok := v.localAllowlist[strings.ToLower(scheme+"://"+hostPort)]; ok {
		return true
	}
	if _, ok := v.localAllowlist[strings.ToLower(hostPort)]; ok {
		return true
	}
	return false
}

func (v *SSRFValidator) ValidateHTTPURL(ctx context.Context, rawURL string, isDemo bool) (*url.URL, []net.IP, error) {
	if isDemo {
		return nil, nil, errors.New("external network targets are disabled in demonstration mode")
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, nil, fmt.Errorf("invalid URL: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, nil, fmt.Errorf("scheme %q is not permitted; use http or https", parsed.Scheme)
	}
	if parsed.User != nil {
		return nil, nil, errors.New("embedded credentials in monitoring URLs are not permitted")
	}
	host := parsed.Hostname()
	if host == "" {
		return nil, nil, errors.New("target URL must include a host")
	}
	port := parsed.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	hostPort := net.JoinHostPort(host, port)
	allowLocal := v.isExplicitlyAllowedLocal(scheme, parsed.Host, isDemo) || v.isExplicitlyAllowedLocal(scheme, hostPort, isDemo)

	ips, err := v.ResolveAndValidateHost(ctx, host, allowLocal)
	if err != nil {
		return nil, nil, err
	}
	return parsed, ips, nil
}

func (v *SSRFValidator) ValidateHostPort(ctx context.Context, target string, isDemo bool) (string, []net.IP, error) {
	if isDemo {
		return "", nil, errors.New("network checks are disabled in demonstration mode")
	}
	host, port, err := net.SplitHostPort(strings.TrimSpace(target))
	if err != nil {
		return "", nil, fmt.Errorf("expected host:port target: %w", err)
	}
	if host == "" || port == "" {
		return "", nil, errors.New("host and port are required")
	}
	allowLocal := v.isExplicitlyAllowedLocal("tcp", net.JoinHostPort(host, port), isDemo)
	ips, err := v.ResolveAndValidateHost(ctx, host, allowLocal)
	if err != nil {
		return "", nil, err
	}
	return net.JoinHostPort(host, port), ips, nil
}

func (v *SSRFValidator) ResolveAndValidateHost(ctx context.Context, host string, allowLocal bool) ([]net.IP, error) {
	lowerHost := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if lowerHost == "" {
		return nil, errors.New("empty host")
	}
	if lowerHost == "metadata.google.internal" || lowerHost == "metadata" || strings.HasSuffix(lowerHost, ".internal") && !allowLocal {
		return nil, fmt.Errorf("blocked metadata or internal hostname %q", host)
	}

	if ip := net.ParseIP(lowerHost); ip != nil {
		if err := ValidateIP(ip, allowLocal); err != nil {
			return nil, err
		}
		return []net.IP{ip}, nil
	}

	resolved, err := v.resolver.LookupIPAddr(ctx, lowerHost)
	if err != nil {
		return nil, fmt.Errorf("dns lookup failed for %q: %w", host, err)
	}
	if len(resolved) == 0 {
		return nil, fmt.Errorf("dns lookup returned no addresses for %q", host)
	}
	ips := make([]net.IP, 0, len(resolved))
	for _, item := range resolved {
		if err := ValidateIP(item.IP, allowLocal); err != nil {
			return nil, err
		}
		ips = append(ips, item.IP)
	}
	return ips, nil
}

var (
	cgnatCIDR         = mustCIDR("100.64.0.0/10")
	cloudMetadataIPs  = map[string]struct{}{"169.254.169.254": {}, "169.254.170.2": {}, "100.100.100.200": {}, "fd00:ec2::254": {}}
	carrierBenchmark  = mustCIDR("198.18.0.0/15")
)

func mustCIDR(cidr string) *net.IPNet {
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	return n
}

func ValidateIP(ip net.IP, allowLocal bool) error {
	if ip == nil {
		return errors.New("invalid IP address")
	}
	if _, blocked := cloudMetadataIPs[ip.String()]; blocked {
		return fmt.Errorf("blocked cloud metadata endpoint %s", ip.String())
	}
	if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
		return fmt.Errorf("blocked link-local, multicast, or unspecified address %s", ip.String())
	}
	if ip.IsLoopback() {
		if allowLocal {
			return nil
		}
		return fmt.Errorf("blocked loopback address %s (not in local allowlist)", ip.String())
	}
	if ip.IsPrivate() || cgnatCIDR.Contains(ip) || carrierBenchmark.Contains(ip) {
		return fmt.Errorf("blocked private or non-routable address %s", ip.String())
	}
	return nil
}

func (v *SSRFValidator) NewHTTPClient(timeout time.Duration, isDemo bool) *http.Client {
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 15 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			allowLocal := v.isExplicitlyAllowedLocal("http", addr, isDemo) || v.isExplicitlyAllowedLocal("https", addr, isDemo)
			ips, err := v.ResolveAndValidateHost(ctx, host, allowLocal)
			if err != nil {
				return nil, err
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
		MaxIdleConns:          16,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("stopped after 5 redirects")
			}
			_, _, err := v.ValidateHTTPURL(req.Context(), req.URL.String(), isDemo)
			return err
		},
	}
}

// RunAllowlistedCommand executes only explicitly allowlisted executables without shell interpolation.
func RunAllowlistedCommand(ctx context.Context, allowlist map[string]string, name string, args ...string) ([]byte, error) {
	resolvedPath, ok := allowlist[name]
	if !ok {
		return nil, fmt.Errorf("executable %q is not allowlisted", name)
	}
	cmd := exec.CommandContext(ctx, resolvedPath, args...)
	return cmd.CombinedOutput()
}
