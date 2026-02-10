package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"carecompanion/internal/repository"
)

var (
	ErrResetTokenExpired = errors.New("reset token has expired")
	ErrResetTokenUsed    = errors.New("reset token has already been used")
	ErrResetTokenInvalid = errors.New("invalid reset token")
)

type PasswordResetService struct {
	db           *sql.DB
	userRepo     repository.UserRepository
	emailService *EmailService
	appURL       string
}

func NewPasswordResetService(db *sql.DB, userRepo repository.UserRepository, emailService *EmailService, appURL string) *PasswordResetService {
	return &PasswordResetService{
		db:           db,
		userRepo:     userRepo,
		emailService: emailService,
		appURL:       appURL,
	}
}

// RequestReset generates a reset token and sends the reset email
func (s *PasswordResetService) RequestReset(ctx context.Context, email string) error {
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return err
	}
	if user == nil {
		// Don't reveal whether the email exists
		log.Printf("[PASSWORD-RESET] Reset requested for non-existent email: %s", email)
		return nil
	}

	// Generate a secure random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("failed to generate token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	// Hash the token for storage (don't store plaintext)
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	// Store the hashed token with 1-hour expiry
	expiresAt := time.Now().Add(1 * time.Hour)
	query := `
		INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`
	if _, err := s.db.ExecContext(ctx, query, user.ID, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("failed to store reset token: %w", err)
	}

	// Build reset URL
	resetURL := fmt.Sprintf("%s/reset-password?token=%s", s.appURL, token)

	// Send the email
	go func() {
		if err := s.emailService.SendPasswordResetEmail(user.Email, user.FirstName, resetURL); err != nil {
			log.Printf("[EMAIL] Failed to send password reset email to %s: %v", user.Email, err)
		}
	}()

	log.Printf("[PASSWORD-RESET] Reset token generated for user %s", user.Email)
	return nil
}

// ResetPassword validates the token and sets the new password
func (s *PasswordResetService) ResetPassword(ctx context.Context, token, newPassword string) error {
	// Hash the incoming token to look up
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	// Find the token
	var tokenID uuid.UUID
	var userID uuid.UUID
	var expiresAt time.Time
	var usedAt sql.NullTime
	query := `
		SELECT id, user_id, expires_at, used_at
		FROM password_reset_tokens
		WHERE token_hash = $1
	`
	err := s.db.QueryRowContext(ctx, query, tokenHash).Scan(&tokenID, &userID, &expiresAt, &usedAt)
	if err == sql.ErrNoRows {
		return ErrResetTokenInvalid
	}
	if err != nil {
		return err
	}

	if usedAt.Valid {
		return ErrResetTokenUsed
	}
	if time.Now().After(expiresAt) {
		return ErrResetTokenExpired
	}

	// Hash the new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password and mark token as used in a transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update password
	_, err = tx.ExecContext(ctx, `UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`, string(hashedPassword), userID)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Mark token as used
	_, err = tx.ExecContext(ctx, `UPDATE password_reset_tokens SET used_at = NOW() WHERE id = $1`, tokenID)
	if err != nil {
		return fmt.Errorf("failed to mark token as used: %w", err)
	}

	// Invalidate all other tokens for this user
	_, err = tx.ExecContext(ctx, `UPDATE password_reset_tokens SET used_at = NOW() WHERE user_id = $1 AND id != $2 AND used_at IS NULL`, userID, tokenID)
	if err != nil {
		return fmt.Errorf("failed to invalidate other tokens: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Printf("[PASSWORD-RESET] Password reset successfully for user %s", userID)
	return nil
}

// ValidateToken checks if a token is valid without using it
func (s *PasswordResetService) ValidateToken(ctx context.Context, token string) (bool, error) {
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	var expiresAt time.Time
	var usedAt sql.NullTime
	query := `SELECT expires_at, used_at FROM password_reset_tokens WHERE token_hash = $1`
	err := s.db.QueryRowContext(ctx, query, tokenHash).Scan(&expiresAt, &usedAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if usedAt.Valid || time.Now().After(expiresAt) {
		return false, nil
	}

	return true, nil
}
