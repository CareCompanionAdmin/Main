package service

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"carecompanion/internal/config"
	"carecompanion/internal/database"
	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserInactive       = errors.New("user account is inactive")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrEmailExists        = errors.New("email already registered")
)

type AuthService struct {
	userRepo   repository.UserRepository
	familyRepo repository.FamilyRepository
	redis      *database.Redis
	jwtConfig  *config.JWTConfig
}

func NewAuthService(
	userRepo repository.UserRepository,
	familyRepo repository.FamilyRepository,
	redis *database.Redis,
	jwtConfig *config.JWTConfig,
) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		familyRepo: familyRepo,
		redis:      redis,
		jwtConfig:  jwtConfig,
	}
}

type AuthClaims struct {
	jwt.RegisteredClaims
	UserID    uuid.UUID        `json:"user_id"`
	Email     string           `json:"email"`
	FamilyID  uuid.UUID        `json:"family_id,omitempty"`
	Role      models.FamilyRole `json:"role,omitempty"`
	FirstName string           `json:"first_name"`
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
	}

	// Generate tokens
	tokens, err := s.generateTokens(user, familyID, models.FamilyRoleParent)
	if err != nil {
		return nil, nil, err
	}

	return user, tokens, nil
}

func (s *AuthService) Login(ctx context.Context, req *LoginRequest) (*models.User, *TokenPair, error) {
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

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, nil, ErrInvalidCredentials
	}

	// Update last login
	s.userRepo.UpdateLastLogin(ctx, user.ID)

	// Get user's families
	memberships, err := s.familyRepo.GetUserFamilies(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}

	var familyID uuid.UUID
	var role models.FamilyRole
	if len(memberships) > 0 {
		// Use first active family
		familyID = memberships[0].FamilyID
		role = memberships[0].Role
	}

	tokens, err := s.generateTokens(user, familyID, role)
	if err != nil {
		return nil, nil, err
	}

	return user, tokens, nil
}

func (s *AuthService) Logout(ctx context.Context, userID uuid.UUID) error {
	return s.redis.DeleteSession(ctx, userID.String())
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

	// Access token claims
	accessClaims := &AuthClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "carecompanion",
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(accessExpiry),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		UserID:    user.ID,
		Email:     user.Email,
		FamilyID:  familyID,
		Role:      role,
		FirstName: user.FirstName,
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
		UserID:   user.ID,
		FamilyID: familyID,
		Role:     role,
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

func (s *AuthService) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func (s *AuthService) VerifyPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}
