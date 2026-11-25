package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/dandisetiawanco/InfraOS/internal/audit"
	"github.com/dandisetiawanco/InfraOS/internal/database"
	"github.com/dandisetiawanco/InfraOS/internal/security"
	"github.com/google/uuid"
)

const (
	SessionCookieName = "infraos_session"
	CSRFCookieName    = "infraos_csrf"
)

type Role string

const (
	RoleOwner    Role = "owner"
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

type Permission string

const (
	PermRead               Permission = "read"
	PermOperateServices    Permission = "operate_services"
	PermOperateDeployments Permission = "operate_deployments"
	PermOperateIncidents   Permission = "operate_incidents"
	PermOperateLogs        Permission = "operate_logs"
	PermManageAlerts       Permission = "manage_alerts"
	PermManageHosts        Permission = "manage_hosts"
	PermManageContainers   Permission = "manage_containers"
	PermManageIntegrations Permission = "manage_integrations"
	PermManageTeam         Permission = "manage_team"
	PermManageOrg          Permission = "manage_org"
	PermManageBackups      Permission = "manage_backups"
)

func HasPermission(role Role, perm Permission) bool {
	switch role {
	case RoleOwner:
		return true
	case RoleAdmin:
		return perm != PermManageOrg
	case RoleOperator:
		switch perm {
		case PermRead, PermOperateServices, PermOperateDeployments, PermOperateIncidents, PermOperateLogs, PermManageAlerts:
			return true
		default:
			return false
		}
	case RoleViewer:
		return perm == PermRead
	default:
		return false
	}
}

type User struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type Organization struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	IsDemo    bool   `json:"is_demo"`
	Role      Role   `json:"role"`
	CreatedAt string `json:"created_at"`
}

type SessionContext struct {
	SessionID string
	User      User
	Org       Organization
	CSRFToken string
	ExpiresAt time.Time
}

type TeamMember struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      Role   `json:"role"`
	CreatedAt string `json:"created_at"`
}

type Service struct {
	db         *database.DB
	audit      *audit.Recorder
	sessionTTL time.Duration
	limiter    *security.RateLimiter
}

func NewService(db *database.DB, auditRec *audit.Recorder, sessionTTL time.Duration, loginLimitPerMin int) *Service {
	if sessionTTL <= 0 {
		sessionTTL = 24 * time.Hour
	}
	return &Service{
		db:         db,
		audit:      auditRec,
		sessionTTL: sessionTTL,
		limiter:    security.NewRateLimiter(loginLimitPerMin, time.Minute),
	}
}

var slugSanitizer = regexp.MustCompile(`[^a-z0-9-]+`)

func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	s = slugSanitizer.ReplaceAllString(s, "")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "workspace"
	}
	return s
}

func (s *Service) AllowLoginAttempt(ip string) bool {
	return s.limiter.Allow(ip)
}

type RegisterInput struct {
	Name             string
	Email            string
	Password         string
	OrganizationName string
	IPAddress        string
	UserAgent        string
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (*SessionContext, string, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	name := strings.TrimSpace(in.Name)
	orgName := strings.TrimSpace(in.OrganizationName)
	if name == "" || email == "" || !strings.Contains(email, "@") {
		return nil, "", errors.New("valid name and email are required")
	}
	if orgName == "" {
		orgName = name + " Infrastructure"
	}
	pwHash, err := security.HashPassword(in.Password)
	if err != nil {
		return nil, "", err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	userID := uuid.NewString()
	orgID := uuid.NewString()
	slug := fmt.Sprintf("%s-%s", slugify(orgName), userID[:6])
	projectID := uuid.NewString()

	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`), userID, email, name, pwHash, now, now); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return errors.New("an account with this email already exists")
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO organizations (id, name, slug, is_demo, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`), orgID, orgName, slug, database.BoolToDB(s.db.Driver, false), now, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO organization_memberships (id, organization_id, user_id, role, created_at)
			VALUES (?, ?, ?, ?, ?)`), uuid.NewString(), orgID, userID, string(RoleOwner), now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO projects (id, organization_id, name, slug, description, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`), projectID, orgID, "Core Infrastructure", "core-infrastructure", "Default infrastructure workspace project.", now, now); err != nil {
			return err
		}
		envs := []struct{ name, slug, tier, desc string }{
			{"Development", "development", "development", "Local development and workstation hosts."},
			{"Staging", "staging", "staging", "Pre-production verification environment."},
			{"Production", "production", "production", "Authorized production services and hosts."},
		}
		for _, e := range envs {
			if _, err := tx.ExecContext(ctx, s.db.Rebind(`
				INSERT INTO environments (id, organization_id, project_id, name, slug, tier, description, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`), uuid.NewString(), orgID, projectID, e.name, e.slug, e.tier, e.desc, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}

	sess, rawToken, err := s.createSession(ctx, userID, orgID, in.IPAddress, in.UserAgent)
	if err != nil {
		return nil, "", err
	}
	_ = s.audit.Record(ctx, audit.RecordInput{
		OrgID: orgID, ActorUserID: userID, Action: "auth.register", ResourceType: "user", ResourceID: userID, IPAddress: in.IPAddress,
	})
	return sess, rawToken, nil
}

func (s *Service) Login(ctx context.Context, email, password, ipAddr, userAgent string) (*SessionContext, string, error) {
	if !s.AllowLoginAttempt(ipAddr) {
		return nil, "", errors.New("too many login attempts; try again in one minute")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	var userID, pwHash string
	err := s.db.QueryRow(ctx, `SELECT id, password_hash FROM users WHERE email = ?`, email).Scan(&userID, &pwHash)
	if err != nil || !security.VerifyPassword(password, pwHash) {
		return nil, "", errors.New("invalid email or password")
	}

	var orgID string
	err = s.db.QueryRow(ctx, `
		SELECT organization_id FROM organization_memberships
		WHERE user_id = ? ORDER BY created_at ASC LIMIT 1`, userID).Scan(&orgID)
	if err != nil {
		return nil, "", errors.New("account has no organization membership")
	}

	sess, rawToken, err := s.createSession(ctx, userID, orgID, ipAddr, userAgent)
	if err != nil {
		return nil, "", err
	}
	_ = s.audit.Record(ctx, audit.RecordInput{
		OrgID: orgID, ActorUserID: userID, Action: "auth.login", ResourceType: "session", ResourceID: sess.SessionID, IPAddress: ipAddr,
	})
	return sess, rawToken, nil
}

func (s *Service) CreateSessionForUser(ctx context.Context, userID, orgID, ipAddr, userAgent string) (*SessionContext, string, error) {
	return s.createSession(ctx, userID, orgID, ipAddr, userAgent)
}

func (s *Service) createSession(ctx context.Context, userID, orgID, ipAddr, userAgent string) (*SessionContext, string, error) {
	rawToken, tokenHash, err := security.GenerateToken("infraos_ses", 32)
	if err != nil {
		return nil, "", err
	}
	csrfToken, _, err := security.GenerateToken("csrf", 24)
	if err != nil {
		return nil, "", err
	}
	sessionID := uuid.NewString()
	now := time.Now().UTC()
	expiresAt := now.Add(s.sessionTTL)

	_, err = s.db.Exec(ctx, `
		INSERT INTO sessions (id, user_id, organization_id, token_hash, csrf_token, expires_at, ip_address, user_agent, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, userID, orgID, tokenHash, csrfToken, expiresAt.Format(time.RFC3339), ipAddr, userAgent, now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, "", err
	}
	sess, err := s.AuthenticateToken(ctx, rawToken)
	return sess, rawToken, err
}

func (s *Service) AuthenticateToken(ctx context.Context, rawToken string) (*SessionContext, error) {
	if rawToken == "" {
		return nil, errors.New("missing session token")
	}
	tokenHash := security.HashToken(rawToken)
	var sess SessionContext
	var expiresRaw string
	var revokedAt sql.NullString
	var isDemoInt int
	var roleStr string

	err := s.db.QueryRow(ctx, `
		SELECT s.id, s.csrf_token, s.expires_at, s.revoked_at,
		       u.id, u.email, u.name, u.created_at,
		       o.id, o.name, o.slug, CASE WHEN o.is_demo THEN 1 ELSE 0 END, o.created_at,
		       m.role
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		JOIN organizations o ON o.id = s.organization_id
		JOIN organization_memberships m ON m.organization_id = o.id AND m.user_id = u.id
		WHERE s.token_hash = ?`, tokenHash).Scan(
		&sess.SessionID, &sess.CSRFToken, &expiresRaw, &revokedAt,
		&sess.User.ID, &sess.User.Email, &sess.User.Name, &sess.User.CreatedAt,
		&sess.Org.ID, &sess.Org.Name, &sess.Org.Slug, &isDemoInt, &sess.Org.CreatedAt,
		&roleStr,
	)
	if err != nil {
		return nil, errors.New("invalid session")
	}
	if revokedAt.Valid && revokedAt.String != "" {
		return nil, errors.New("session revoked")
	}
	exp, err := parseTimestamp(expiresRaw)
	if err != nil || time.Now().UTC().After(exp) {
		return nil, errors.New("session expired")
	}
	sess.ExpiresAt = exp
	sess.Org.IsDemo = isDemoInt == 1
	sess.Org.Role = Role(roleStr)
	return &sess, nil
}

func (s *Service) RevokeSession(ctx context.Context, sessionID, orgID, userID, ipAddr string) error {
	_, err := s.db.Exec(ctx, `UPDATE sessions SET revoked_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), sessionID)
	if err == nil {
		_ = s.audit.Record(ctx, audit.RecordInput{
			OrgID: orgID, ActorUserID: userID, Action: "auth.logout", ResourceType: "session", ResourceID: sessionID, IPAddress: ipAddr,
		})
	}
	return err
}

func (s *Service) ListUserOrganizations(ctx context.Context, userID string) ([]Organization, error) {
	rows, err := s.db.Query(ctx, `
		SELECT o.id, o.name, o.slug, CASE WHEN o.is_demo THEN 1 ELSE 0 END, m.role, o.created_at
		FROM organization_memberships m
		JOIN organizations o ON o.id = m.organization_id
		WHERE m.user_id = ?
		ORDER BY o.created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orgs := make([]Organization, 0)
	for rows.Next() {
		var o Organization
		var isDemoInt int
		var roleStr string
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &isDemoInt, &roleStr, &o.CreatedAt); err != nil {
			return nil, err
		}
		o.IsDemo = isDemoInt == 1
		o.Role = Role(roleStr)
		orgs = append(orgs, o)
	}
	return orgs, rows.Err()
}

func (s *Service) SwitchOrganization(ctx context.Context, sessionID, userID, targetOrgID string) (*Organization, error) {
	var o Organization
	var isDemoInt int
	var roleStr string
	err := s.db.QueryRow(ctx, `
		SELECT o.id, o.name, o.slug, CASE WHEN o.is_demo THEN 1 ELSE 0 END, m.role, o.created_at
		FROM organization_memberships m
		JOIN organizations o ON o.id = m.organization_id
		WHERE m.user_id = ? AND m.organization_id = ?`, userID, targetOrgID).Scan(
		&o.ID, &o.Name, &o.Slug, &isDemoInt, &roleStr, &o.CreatedAt,
	)
	if err != nil {
		return nil, errors.New("organization access denied")
	}
	if _, err := s.db.Exec(ctx, `UPDATE sessions SET organization_id = ? WHERE id = ?`, targetOrgID, sessionID); err != nil {
		return nil, err
	}
	o.IsDemo = isDemoInt == 1
	o.Role = Role(roleStr)
	return &o, nil
}

func (s *Service) ListMembers(ctx context.Context, orgID string) ([]TeamMember, error) {
	rows, err := s.db.Query(ctx, `
		SELECT u.id, u.email, u.name, m.role, m.created_at
		FROM organization_memberships m
		JOIN users u ON u.id = m.user_id
		WHERE m.organization_id = ?
		ORDER BY m.created_at ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TeamMember, 0)
	for rows.Next() {
		var tm TeamMember
		var r string
		if err := rows.Scan(&tm.UserID, &tm.Email, &tm.Name, &r, &tm.CreatedAt); err != nil {
			return nil, err
		}
		tm.Role = Role(r)
		out = append(out, tm)
	}
	return out, rows.Err()
}

func (s *Service) AddOrInviteMember(ctx context.Context, orgID, email, name, tempPassword string, role Role) (*TeamMember, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return nil, errors.New("valid email is required")
	}
	switch role {
	case RoleOwner, RoleAdmin, RoleOperator, RoleViewer:
	default:
		return nil, errors.New("invalid role")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var userID, resolvedName string
	err := s.db.QueryRow(ctx, `SELECT id, name FROM users WHERE email = ?`, email).Scan(&userID, &resolvedName)
	if err == sql.ErrNoRows {
		if strings.TrimSpace(name) == "" {
			name = strings.Split(email, "@")[0]
		}
		if len(tempPassword) < 8 {
			tempPassword = uuid.NewString() + "A1!"
		}
		pwHash, err := security.HashPassword(tempPassword)
		if err != nil {
			return nil, err
		}
		userID = uuid.NewString()
		resolvedName = name
		if _, err := s.db.Exec(ctx, `
			INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`, userID, email, resolvedName, pwHash, now, now); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO organization_memberships (id, organization_id, user_id, role, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(organization_id, user_id) DO UPDATE SET role = excluded.role`,
		uuid.NewString(), orgID, userID, string(role), now)
	if err != nil {
		return nil, err
	}
	return &TeamMember{UserID: userID, Email: email, Name: resolvedName, Role: role, CreatedAt: now}, nil
}

func (s *Service) UpdateMemberRole(ctx context.Context, orgID, targetUserID string, role Role) error {
	switch role {
	case RoleOwner, RoleAdmin, RoleOperator, RoleViewer:
	default:
		return errors.New("invalid role")
	}
	res, err := s.db.Exec(ctx, `UPDATE organization_memberships SET role = ? WHERE organization_id = ? AND user_id = ?`, string(role), orgID, targetUserID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("member not found")
	}
	return nil
}

func SetSessionCookies(w http.ResponseWriter, rawToken, csrfToken string, expiresAt time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    rawToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    csrfToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func ClearSessionCookies(w http.ResponseWriter, secure bool) {
	expired := time.Unix(0, 0)
	for _, name := range []string{SessionCookieName, CSRFCookieName} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			Expires:  expired,
			MaxAge:   -1,
			HttpOnly: name == SessionCookieName,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

func parseTimestamp(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, raw)
}

type ctxKey string

const sessionCtxKey ctxKey = "infraos_session_ctx"

func WithSession(ctx context.Context, sess *SessionContext) context.Context {
	return context.WithValue(ctx, sessionCtxKey, sess)
}

func FromContext(ctx context.Context) (*SessionContext, bool) {
	s, ok := ctx.Value(sessionCtxKey).(*SessionContext)
	return s, ok && s != nil
}
