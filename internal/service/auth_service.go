package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"carecompanion/internal/config"
	"carecompanion/internal/database"
	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// stripPort removes the :port suffix from net/http RemoteAddr strings so the
// host can be stored in a Postgres INET column. Returns "" for empty input.
// IPv6 addresses arrive bracketed ("[::1]:5432") — net.SplitHostPort handles
// both forms; on parse failure we return the input unchanged so a caller
// passing an already-bare IP keeps working.
func stripPort(addr string) string {
	if addr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserInactive       = errors.New("user account is inactive")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrEmailExists        = errors.New("email already registered")
)

type AuthService struct {
	userRepo     repository.UserRepository
	familyRepo   repository.FamilyRepository
	sessionRepo  repository.SessionRepository
	sessionCache *SessionCache
	redis        *database.Redis
	jwtConfig    *config.JWTConfig
	emailService *EmailService
	appURL       string
	appEnv       string
	subSvc       *SubscriptionService // wired post-construction; nil-safe
}

// SetSubscriptionService wires the subscription lifecycle service so
// Register can start a 14-day trial when a new family is created.
func (s *AuthService) SetSubscriptionService(sub *SubscriptionService) {
	s.subSvc = sub
}

func NewAuthService(
	userRepo repository.UserRepository,
	familyRepo repository.FamilyRepository,
	sessionRepo repository.SessionRepository,
	sessionCache *SessionCache,
	redis *database.Redis,
	jwtConfig *config.JWTConfig,
	emailService *EmailService,
	appURL string,
	appEnv string,
) *AuthService {
	return &AuthService{
		userRepo:     userRepo,
		familyRepo:   familyRepo,
		sessionRepo:  sessionRepo,
		sessionCache: sessionCache,
		redis:        redis,
		jwtConfig:    jwtConfig,
		emailService: emailService,
		appURL:       appURL,
		appEnv:       appEnv,
	}
}

type AuthClaims struct {
	jwt.RegisteredClaims
	Sid        uuid.UUID          `json:"sid,omitempty"`
	UserID     uuid.UUID          `json:"user_id"`
	Email      string             `json:"email"`
	FamilyID   uuid.UUID          `json:"family_id,omitempty"`
	Role       models.FamilyRole  `json:"role,omitempty"`
	SystemRole models.SystemRole  `json:"system_role,omitempty"` // super_admin, support, marketing
	FirstName  string             `json:"first_name"`
}

// HasSystemRole checks if the claims have a system admin role
func (c *AuthClaims) HasSystemRole() bool {
	return c.SystemRole != ""
}

// IsSuperAdmin checks if the claims have super_admin role
func (c *AuthClaims) IsSuperAdmin() bool {
	return c.SystemRole == models.SystemRoleSuperAdmin
}

// IsSupport checks if the claims have support role
func (c *AuthClaims) IsSupport() bool {
	return c.SystemRole == models.SystemRoleSupport
}

// IsMarketing checks if the claims have marketing role
func (c *AuthClaims) IsMarketing() bool {
	return c.SystemRole == models.SystemRoleMarketing
}

// HasAnySystemRole checks if claims have any of the given system roles
func (c *AuthClaims) HasAnySystemRole(roles ...models.SystemRole) bool {
	for _, role := range roles {
		if c.SystemRole == role {
			return true
		}
	}
	return false
}

type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type RegisterRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	Phone      string `json:"phone,omitempty"`
	FamilyName string `json:"family_name,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginContext struct {
	Kind      models.SessionKind
	IP        string
	UserAgent string
}

func (s *AuthService) Register(ctx context.Context, req *RegisterRequest) (*models.User, *TokenPair, error) {
	// Check if email exists
	existing, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, nil, err
	}
	if existing != nil {
		return nil, nil, ErrEmailExists
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, nil, err
	}

	// Create user
	user := &models.User{
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		FirstName:    req.FirstName,
		Status:       models.UserStatusActive,
	}
	user.LastName = req.LastName
	user.Phone.String = req.Phone
	user.Phone.Valid = req.Phone != ""

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, nil, err
	}

	// Create family if name provided
	var familyID uuid.UUID
	if req.FamilyName != "" {
		family := &models.Family{
			Name:      req.FamilyName,
			CreatedBy: user.ID,
		}
		if err := s.familyRepo.Create(ctx, family); err != nil {
			return nil, nil, err
		}
		familyID = family.ID

		// Add user as parent
		membership := &models.FamilyMembership{
			FamilyID: family.ID,
			UserID:   user.ID,
			Role:     models.FamilyRoleParent,
		}
		if err := s.familyRepo.AddMember(ctx, membership); err != nil {
			return nil, nil, err
		}

		// Start 14-day Single-Child trial. Best-effort — a trial-creation
		// failure shouldn't block signup; the user will see a paywall on
		// next request and an admin can comp them manually if needed.
		if s.subSvc != nil {
			if err := s.subSvc.StartTrialIfNew(ctx, family.ID); err != nil {
				log.Printf("[AUTH] StartTrialIfNew failed for family %s: %v", family.ID, err)
			}
		}
	}

	// Check for pending family invitations and accept them
	var assignedRole models.FamilyRole
	invitations, invErr := s.familyRepo.GetPendingInvitations(ctx, req.Email)
	if invErr == nil && len(invitations) > 0 {
		for _, inv := range invitations {
			membership := &models.FamilyMembership{
				FamilyID: inv.FamilyID,
				UserID:   user.ID,
				Role:     inv.Role,
			}
			if addErr := s.familyRepo.AddMember(ctx, membership); addErr != nil {
				log.Printf("[AUTH] Failed to auto-add user %s to family %s: %v", user.Email, inv.FamilyID, addErr)
				continue
			}
			if acceptErr := s.familyRepo.AcceptInvitation(ctx, inv.ID); acceptErr != nil {
				log.Printf("[AUTH] Failed to mark invitation %s as accepted: %v", inv.ID, acceptErr)
			}
			// Use the first invited family as the default if no family was created
			if familyID == uuid.Nil {
				familyID = inv.FamilyID
				assignedRole = inv.Role
			}
			log.Printf("[AUTH] Auto-accepted invitation for %s to family %s with role %s", user.Email, inv.FamilyID, inv.Role)
		}
	}

	// Send welcome email (async, don't block registration)
	go func() {
		if err := s.emailService.SendWelcomeEmail(user.Email, user.FirstName, s.appURL); err != nil {
			log.Printf("[EMAIL] Failed to send welcome email to %s: %v", user.Email, err)
		}
	}()

	// Determine the role for token generation
	// If user created their own family, they're a parent; if they joined via invitation, use that role
	tokenRole := models.FamilyRoleParent
	if assignedRole != "" {
		tokenRole = assignedRole
	}

	// Generate tokens
	tokens, err := s.generateTokens(user, familyID, tokenRole)
	if err != nil {
		return nil, nil, err
	}

	return user, tokens, nil
}

func (s *AuthService) Login(ctx context.Context, req *LoginRequest) (*models.User, *TokenPair, error) {
	return s.LoginWithContext(ctx, req, LoginContext{Kind: models.SessionKindUser})
}

func (s *AuthService) LoginWithContext(ctx context.Context, req *LoginRequest, lc LoginContext) (*models.User, *TokenPair, error) {
	user, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, nil, err
	}
	if user == nil {
		return nil, nil, ErrInvalidCredentials
	}
	if user.Status != models.UserStatusActive {
		return nil, nil, ErrUserInactive
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, nil, ErrInvalidCredentials
	}

	s.userRepo.UpdateLastLogin(ctx, user.ID)

	memberships, err := s.familyRepo.GetUserFamilies(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}
	var familyID uuid.UUID
	var role models.FamilyRole
	if len(memberships) > 0 {
		familyID = memberships[0].FamilyID
		role = memberships[0].Role
	}

	// Default the kind so direct callers of Login (no LoginContext) still work.
	if lc.Kind == "" {
		lc.Kind = models.SessionKindUser
	}

	// At most one active session per (user_id, kind). Revoke any existing one.
	_ = s.sessionRepo.RevokeForUserKind(ctx, user.ID, lc.Kind)

	expires := time.Now().Add(s.jwtConfig.AccessExpiry)
	sess := &models.Session{
		UserID:    user.ID,
		Kind:      lc.Kind,
		ExpiresAt: expires,
	}
	if user.HasSystemRole() {
		sess.SystemRole = models.NullString{NullString: sql.NullString{String: string(user.GetSystemRole()), Valid: true}}
	}
	if familyID != uuid.Nil {
		sess.FamilyID = models.NullUUID{UUID: familyID, Valid: true}
	}
	if ip := stripPort(lc.IP); ip != "" {
		sess.IPAtStart = models.NullString{NullString: sql.NullString{String: ip, Valid: true}}
	}
	if lc.UserAgent != "" {
		sess.UserAgent = models.NullString{NullString: sql.NullString{String: lc.UserAgent, Valid: true}}
	}
	sess.UserEmail = models.NullString{NullString: sql.NullString{String: user.Email, Valid: user.Email != ""}}
	sess.UserFirstName = models.NullString{NullString: sql.NullString{String: user.FirstName, Valid: user.FirstName != ""}}
	sess.UserLastName = models.NullString{NullString: sql.NullString{String: user.LastName, Valid: user.LastName != ""}}
	if s.appEnv != "" {
		sess.EnvName = models.NullString{NullString: sql.NullString{String: s.appEnv, Valid: true}}
	}
	if familyID != uuid.Nil {
		if family, ferr := s.familyRepo.GetByID(ctx, familyID); ferr == nil && family != nil && family.Name != "" {
			sess.FamilyName = models.NullString{NullString: sql.NullString{String: family.Name, Valid: true}}
		}
	}
	if err := s.sessionRepo.Create(ctx, sess); err != nil {
		return nil, nil, fmt.Errorf("create session: %w", err)
	}

	tokens, err := s.generateTokensWithSid(user, familyID, role, sess.ID)
	if err != nil {
		return nil, nil, err
	}
	s.sessionCache.MarkValid(ctx, sess.ID)
	return user, tokens, nil
}

func (s *AuthService) Logout(ctx context.Context, userID uuid.UUID) error {
	return s.sessionRepo.RevokeForUserKind(ctx, userID, models.SessionKindUser)
}

func (s *AuthService) LogoutAdmin(ctx context.Context, userID uuid.UUID) error {
	return s.sessionRepo.RevokeForUserKind(ctx, userID, models.SessionKindAdmin)
}

func (s *AuthService) LogoutAll(ctx context.Context, userID uuid.UUID) error {
	if err := s.sessionRepo.RevokeForUserKind(ctx, userID, models.SessionKindUser); err != nil {
		return err
	}
	return s.sessionRepo.RevokeForUserKind(ctx, userID, models.SessionKindAdmin)
}

var (
	ErrSessionRevoked  = errors.New("session revoked")
	ErrSessionExpired  = errors.New("session expired")
	ErrSessionNotFound = errors.New("session not found")
)

func (s *AuthService) RevokeSession(ctx context.Context, sid uuid.UUID) error {
	if err := s.sessionRepo.Revoke(ctx, sid); err != nil {
		return err
	}
	s.sessionCache.MarkRevoked(ctx, sid)
	return nil
}

func (s *AuthService) ValidateSession(ctx context.Context, sid uuid.UUID) error {
	switch s.sessionCache.Lookup(ctx, sid) {
	case "valid":
		return nil
	case "revoked":
		return ErrSessionRevoked
	}
	row, err := s.sessionRepo.GetByID(ctx, sid)
	if err != nil {
		return err
	}
	if row == nil {
		return ErrSessionNotFound
	}
	if row.RevokedAt != nil {
		s.sessionCache.MarkRevoked(ctx, sid)
		return ErrSessionRevoked
	}
	if time.Now().After(row.ExpiresAt) {
		return ErrSessionExpired
	}
	s.sessionCache.MarkValid(ctx, sid)
	return nil
}

// TouchSession updates last_seen_at off the request hot path. Best-effort —
// errors are swallowed.
func (s *AuthService) TouchSession(sid uuid.UUID) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.sessionRepo.TouchLastSeen(ctx, sid)
	}()
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := s.ValidateToken(refreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// Get user
	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil || user.Status != models.UserStatusActive {
		return nil, ErrUserInactive
	}

	// Preserve the sid across refresh so AuthMiddleware can keep validating
	// against the sessions table. Without this, a refreshed access token
	// carries Sid=uuid.Nil and the middleware accepts on signature alone —
	// which means a revoked session can be kept alive indefinitely by
	// refreshing. For legacy pre-sid refresh tokens (Sid=uuid.Nil) we
	// fall back to generateTokens so the JWT shape stays unchanged.
	if claims.Sid != uuid.Nil {
		return s.generateTokensWithSid(user, claims.FamilyID, claims.Role, claims.Sid)
	}
	return s.generateTokens(user, claims.FamilyID, claims.Role)
}

func (s *AuthService) ValidateToken(tokenString string) (*AuthClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AuthClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.jwtConfig.Secret), nil
	})

	if err != nil {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*AuthClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

func (s *AuthService) SwitchFamily(ctx context.Context, userID, familyID uuid.UUID) (*TokenPair, error) {
	// Verify membership
	membership, err := s.familyRepo.GetMembership(ctx, familyID, userID)
	if err != nil {
		return nil, err
	}
	if membership == nil {
		return nil, errors.New("not a member of this family")
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return s.generateTokens(user, familyID, membership.Role)
}

func (s *AuthService) generateTokens(user *models.User, familyID uuid.UUID, role models.FamilyRole) (*TokenPair, error) {
	now := time.Now()
	accessExpiry := now.Add(s.jwtConfig.AccessExpiry)
	refreshExpiry := now.Add(s.jwtConfig.RefreshExpiry)

	// Get system role if present
	var systemRole models.SystemRole
	if user.HasSystemRole() {
		systemRole = user.GetSystemRole()
	}

	// Access token claims
	accessClaims := &AuthClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "carecompanion",
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(accessExpiry),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		UserID:     user.ID,
		Email:      user.Email,
		FamilyID:   familyID,
		Role:       role,
		SystemRole: systemRole,
		FirstName:  user.FirstName,
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(s.jwtConfig.Secret))
	if err != nil {
		return nil, err
	}

	// Refresh token claims (minimal)
	refreshClaims := &AuthClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "carecompanion",
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(refreshExpiry),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		UserID:     user.ID,
		FamilyID:   familyID,
		Role:       role,
		SystemRole: systemRole,
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString([]byte(s.jwtConfig.Secret))
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
		ExpiresAt:    accessExpiry,
	}, nil
}

func (s *AuthService) generateTokensWithSid(user *models.User, familyID uuid.UUID, role models.FamilyRole, sid uuid.UUID) (*TokenPair, error) {
	now := time.Now()
	accessExpiry := now.Add(s.jwtConfig.AccessExpiry)
	refreshExpiry := now.Add(s.jwtConfig.RefreshExpiry)

	var systemRole models.SystemRole
	if user.HasSystemRole() {
		systemRole = user.GetSystemRole()
	}

	accessClaims := &AuthClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "carecompanion",
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(accessExpiry),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Sid:        sid,
		UserID:     user.ID,
		Email:      user.Email,
		FamilyID:   familyID,
		Role:       role,
		SystemRole: systemRole,
		FirstName:  user.FirstName,
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessStr, err := accessToken.SignedString([]byte(s.jwtConfig.Secret))
	if err != nil {
		return nil, err
	}

	refreshClaims := &AuthClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "carecompanion",
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(refreshExpiry),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Sid:        sid,
		UserID:     user.ID,
		FamilyID:   familyID,
		Role:       role,
		SystemRole: systemRole,
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshStr, err := refreshToken.SignedString([]byte(s.jwtConfig.Secret))
	if err != nil {
		return nil, err
	}

	return &TokenPair{AccessToken: accessStr, RefreshToken: refreshStr, ExpiresAt: accessExpiry}, nil
}

func (s *AuthService) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func (s *AuthService) VerifyPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}
