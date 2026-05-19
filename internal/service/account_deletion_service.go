package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/config"
	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

var (
	ErrDeletionAlreadyPending  = errors.New("a deletion request is already pending for this user")
	ErrDeletionCodeInvalid     = errors.New("confirmation code is invalid or expired")
	ErrDeletionCodeMaxAttempts = errors.New("too many failed attempts; request a new code")
	ErrDeletionRestoreInvalid  = errors.New("restore link is invalid or expired")
	ErrDeletionNotFound        = errors.New("no active deletion request")
)

// AccountDeletionService orchestrates the user-initiated account deletion
// flow. See migrations/00034_account_deletion.sql + the App Store Approval
// initiative memory for the lifecycle.
type AccountDeletionService struct {
	repo         repository.AccountDeletionRepository
	userRepo     repository.UserRepository
	authService  *AuthService
	emailService *EmailService
	billingRepo  repository.BillingRepository
	appURL       string

	// Tunables. Default values applied in NewAccountDeletionService; all are
	// overridable from config so prod and dev can diverge if needed.
	otpTTL                 time.Duration // 15 min
	otpMaxAttempts         int           // 5
	softDeleteGrace        time.Duration // 30 days — total window
	selfRestoreWindow      time.Duration // 14 days — link in email valid this long
	coldBackupRetention    time.Duration // 90 days — config: COLD_BACKUP_RETENTION_DAYS
	repeatCancelWindow     time.Duration // 365 days — refund forfeit if cancel within

	// confirmMu serializes ConfirmDeletion attempts per user inside this
	// process so the check-then-increment on confirmation_attempts can't
	// be raced by concurrent requests for the same user. This is a single-
	// process guard — running multiple app instances against the same DB
	// would still allow cross-process races, but the repo's UPDATE on
	// confirmation_attempts is atomic at the row level and the
	// AccountDeletionRequest itself is single-user-keyed, so the practical
	// blast radius is bounded.
	confirmMu  sync.Mutex
	confirmLocks map[uuid.UUID]*sync.Mutex
}

// NewAccountDeletionService wires the dependencies.
func NewAccountDeletionService(
	repo repository.AccountDeletionRepository,
	userRepo repository.UserRepository,
	authService *AuthService,
	emailService *EmailService,
	billingRepo repository.BillingRepository,
	cfg *config.Config,
) *AccountDeletionService {
	return &AccountDeletionService{
		repo:                repo,
		userRepo:            userRepo,
		authService:         authService,
		emailService:        emailService,
		billingRepo:         billingRepo,
		appURL:              cfg.App.URL,
		otpTTL:              15 * time.Minute,
		otpMaxAttempts:      5,
		softDeleteGrace:     30 * 24 * time.Hour,
		selfRestoreWindow:   14 * 24 * time.Hour,
		coldBackupRetention: durationFromEnvDays("COLD_BACKUP_RETENTION_DAYS", 90),
		repeatCancelWindow:  365 * 24 * time.Hour,
		confirmLocks:        make(map[uuid.UUID]*sync.Mutex),
	}
}

// lockForUser returns (and lazily allocates) a per-user mutex used by
// ConfirmDeletion to serialize the check-then-increment on
// confirmation_attempts. The lock map itself is guarded by confirmMu.
func (s *AccountDeletionService) lockForUser(userID uuid.UUID) *sync.Mutex {
	s.confirmMu.Lock()
	defer s.confirmMu.Unlock()
	if mu, ok := s.confirmLocks[userID]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	s.confirmLocks[userID] = mu
	return mu
}

func durationFromEnvDays(key string, defaultDays int) time.Duration {
	v := getenv(key)
	if v == "" {
		return time.Duration(defaultDays) * 24 * time.Hour
	}
	n, err := parseInt(v)
	if err != nil || n <= 0 {
		log.Printf("[ADS] Invalid %s=%q, using default %d days", key, v, defaultDays)
		return time.Duration(defaultDays) * 24 * time.Hour
	}
	return time.Duration(n) * 24 * time.Hour
}

// RequestDeletion is step 1: user clicks Delete My Account → we generate
// an OTP, store it hashed, email the plaintext to their on-file address.
// The disclaimer copy (per-role, per-family) is rendered by the handler /
// frontend — this service just kicks off the verification flow.
//
// If a pending_code or confirmed request already exists for this user, we
// return ErrDeletionAlreadyPending. The frontend should surface the
// existing request rather than start a new one.
func (s *AccountDeletionService) RequestDeletion(ctx context.Context, userID uuid.UUID, ip, userAgent string) (*models.AccountDeletionRequest, error) {
	existing, err := s.repo.GetActiveByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, ErrDeletionAlreadyPending
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	// Build the primary-of-families snapshot.
	roles, err := s.repo.GetFamilyRolesForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load family roles: %w", err)
	}
	var primaryFamilyIDs []uuid.UUID
	for _, r := range roles {
		if r.IsPrimary {
			primaryFamilyIDs = append(primaryFamilyIDs, r.FamilyID)
		}
	}
	primaryJSON, _ := json.Marshal(primaryFamilyIDs)
	if string(primaryJSON) == "null" {
		primaryJSON = []byte("[]")
	}

	// Generate the 6-digit OTP. crypto/rand on a million-element range; not
	// cryptographically critical (5-attempt cap + 15-min TTL + email-bound)
	// but it costs nothing to do it right.
	code, err := randomNumericCode(6)
	if err != nil {
		return nil, err
	}

	req := &models.AccountDeletionRequest{
		ID:                        uuid.New(),
		UserID:                    userID,
		UserEmailAtRequest:        user.Email,
		ConfirmationCodeHash:      sha256Hex(code),
		ConfirmationCodeExpiresAt: time.Now().Add(s.otpTTL),
		Status:                    models.DeletionStatusPendingCode,
		PrimaryOfFamilies:         primaryJSON,
	}
	req.UserFirstNameAtRequest.String = user.FirstName
	req.UserFirstNameAtRequest.Valid = user.FirstName != ""
	req.UserLastNameAtRequest.String = user.LastName
	req.UserLastNameAtRequest.Valid = user.LastName != ""
	if ip != "" {
		req.IPAtRequest.String = ip
		req.IPAtRequest.Valid = true
	}
	if userAgent != "" {
		req.UserAgentAtRequest.String = userAgent
		req.UserAgentAtRequest.Valid = true
	}

	if err := s.repo.CreateRequest(ctx, req); err != nil {
		return nil, fmt.Errorf("create deletion request: %w", err)
	}

	// Fire the email asynchronously so the API responds fast. Logged-only
	// on failure — the user can request a new code if they don't receive it.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ADS] panic in deletion-code email goroutine: %v\n%s", r, debug.Stack())
			}
		}()
		if err := s.emailService.SendAccountDeletionCodeEmail(user.Email, user.FirstName, code, int(s.otpTTL.Minutes())); err != nil {
			log.Printf("[ADS] Failed to send deletion-code email to %s: %v", user.Email, err)
		}
	}()

	return req, nil
}

// ConfirmDeletion is step 2: user enters the OTP. If valid, we run the
// soft-delete (Confirm in the repo) and kick off the side effects:
//   - Stripe pro-rata refund (skipped for v1 — wire later)
//   - Cancellation history row
//   - Email: deletion-started (with restore link + export-offer link)
//
// Returns the updated request row.
func (s *AccountDeletionService) ConfirmDeletion(ctx context.Context, userID uuid.UUID, code string) (*models.AccountDeletionRequest, error) {
	// Serialize confirmation attempts for the same user so the
	// check-attempts → increment race can't slip extra attempts through.
	// The repository-level IncrementAttempts is itself a single UPDATE so
	// it's atomic; the window we're closing here is between reading
	// req.ConfirmationAttempts and calling IncrementAttempts.
	mu := s.lockForUser(userID)
	mu.Lock()
	defer mu.Unlock()

	req, err := s.repo.GetActiveByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, ErrDeletionNotFound
	}
	if req.Status != models.DeletionStatusPendingCode {
		return nil, fmt.Errorf("request is not in pending_code state (status=%s)", req.Status)
	}
	if time.Now().After(req.ConfirmationCodeExpiresAt) {
		return nil, ErrDeletionCodeInvalid
	}
	if req.ConfirmationAttempts >= s.otpMaxAttempts {
		return nil, ErrDeletionCodeMaxAttempts
	}

	if sha256Hex(code) != req.ConfirmationCodeHash {
		_ = s.repo.IncrementAttempts(ctx, req.ID)
		return nil, ErrDeletionCodeInvalid
	}

	// Mint the restore token. 32 random bytes → 64 hex chars in the URL.
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	tokenPlain := hex.EncodeToString(tokenBytes)
	tokenHash := sha256Hex(tokenPlain)

	now := time.Now()
	scheduled := now.Add(s.softDeleteGrace)
	restoreExpires := now.Add(s.selfRestoreWindow)

	confirmed, err := s.repo.Confirm(ctx, req.ID, tokenHash, restoreExpires, scheduled)
	if err != nil {
		return nil, fmt.Errorf("confirm deletion: %w", err)
	}

	// Side effects — async, log-only on failure. They're recoverable:
	// the deletion is real either way; emails can be re-sent.
	go func(req *models.AccountDeletionRequest, restoreToken string) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ADS] panic in post-confirm goroutine for request %s: %v\n%s", req.ID, r, debug.Stack())
			}
		}()
		// Stripe cancel + refund handled by SubscriptionCancelOnDeletion
		// (separate method so it can be tested + retried). Best-effort.
		if err := s.handlePostConfirm(context.Background(), req, restoreToken); err != nil {
			log.Printf("[ADS] post-confirm side effects for request %s failed: %v", req.ID, err)
		}
	}(confirmed, tokenPlain)

	return confirmed, nil
}

// handlePostConfirm runs the side effects after a successful confirmation:
// Stripe cancel + pro-rata refund (if applicable), creates the export job
// (status=awaiting_consent), sends the deletion-started email with both
// the restore link and the export-offer link.
func (s *AccountDeletionService) handlePostConfirm(ctx context.Context, req *models.AccountDeletionRequest, restoreToken string) error {
	// TODO Phase 2: Stripe cancel + pro-rata refund. For now, just record
	// the cancellation row so we can backfill once Stripe wiring lands.
	_ = s.recordCancellationStub(ctx, req)

	// Create the export job in awaiting_consent state.
	consentBytes := make([]byte, 32)
	if _, err := rand.Read(consentBytes); err != nil {
		return err
	}
	consentToken := hex.EncodeToString(consentBytes)
	exportJob := &models.DataExportJob{
		ID:                    uuid.New(),
		DeletionRequestID:     req.ID,
		ConsentTokenHash:      sha256Hex(consentToken),
		ConsentTokenExpiresAt: time.Now().Add(s.softDeleteGrace),
		Status:                models.ExportStatusAwaitingConsent,
	}
	if err := s.repo.CreateExportJob(ctx, exportJob); err != nil {
		return fmt.Errorf("create export job: %w", err)
	}

	// Build the URLs we embed in the email.
	restoreURL := fmt.Sprintf("%s/account/restore?token=%s", s.appURL, restoreToken)
	exportURL := fmt.Sprintf("%s/account/export?token=%s", s.appURL, consentToken)
	firstName := ""
	if req.UserFirstNameAtRequest.Valid {
		firstName = req.UserFirstNameAtRequest.String
	}

	if err := s.emailService.SendAccountDeletionStartedEmail(
		req.UserEmailAtRequest, firstName, restoreURL, exportURL,
		req.ScheduledHardDeleteAt.Time,
		int(s.selfRestoreWindow.Hours()/24),
		int(s.softDeleteGrace.Hours()/24),
	); err != nil {
		log.Printf("[ADS] Failed to send deletion-started email to %s: %v", req.UserEmailAtRequest, err)
	}

	return nil
}

func (s *AccountDeletionService) recordCancellationStub(ctx context.Context, req *models.AccountDeletionRequest) error {
	// Phase-2 placeholder: write a cancellation history row with refund=0
	// and a note explaining Stripe hasn't been wired yet. Once Stripe is in,
	// this becomes the real refund calculation. We still want the row so
	// the 12-month-repeat-cancel check can fire when Stripe lands.
	c := &models.SubscriptionCancellation{
		ID:                  uuid.New(),
		UserID:              req.UserID,
		CancelledAt:         time.Now(),
		RefundAmountCents:   0,
		AdminFeeCents:       0,
	}
	c.Notes.String = "Phase-2: Stripe pro-rata refund not yet wired. Cancellation triggered by account deletion (request " + req.ID.String() + ")."
	c.Notes.Valid = true
	return s.repo.CreateCancellation(ctx, c)
}

// RestoreByToken handles the "Undo deletion" email link. Validates the
// token (one-shot, expires after selfRestoreWindow), reverses the
// soft-delete, returns the restored request row.
func (s *AccountDeletionService) RestoreByToken(ctx context.Context, token string) (*models.AccountDeletionRequest, error) {
	if token == "" {
		return nil, ErrDeletionRestoreInvalid
	}
	tokenHash := sha256Hex(token)
	req, err := s.repo.FindByRestoreTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, ErrDeletionRestoreInvalid
	}
	if req.RestoreTokenExpiresAt.Valid && time.Now().After(req.RestoreTokenExpiresAt.Time) {
		return nil, ErrDeletionRestoreInvalid
	}
	if err := s.repo.Restore(ctx, req.ID, nil); err != nil {
		return nil, fmt.Errorf("restore: %w", err)
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ADS] panic in restored-email goroutine: %v\n%s", r, debug.Stack())
			}
		}()
		firstName := ""
		if req.UserFirstNameAtRequest.Valid {
			firstName = req.UserFirstNameAtRequest.String
		}
		if err := s.emailService.SendAccountRestoredEmail(req.UserEmailAtRequest, firstName, s.appURL); err != nil {
			log.Printf("[ADS] Failed to send restored-confirmation email: %v", err)
		}
	}()
	return s.repo.GetByID(ctx, req.ID)
}

// RestoreByAdmin is the support-mediated path used during days 14-30 (or
// at any other admin discretion). adminUserID is recorded.
func (s *AccountDeletionService) RestoreByAdmin(ctx context.Context, requestID, adminUserID uuid.UUID) error {
	return s.repo.Restore(ctx, requestID, &adminUserID)
}

// ProcessDueHardDeletes is the cron entry. Runs the cascade for any
// confirmed deletion whose 30-day clock has elapsed. Cold-backup
// hand-off is stubbed for v1 (we record an empty s3_key + retention TS
// so the cold-purge cron still picks it up).
func (s *AccountDeletionService) ProcessDueHardDeletes(ctx context.Context, limit int) (int, error) {
	due, err := s.repo.FindHardDeleteDue(ctx, limit)
	if err != nil {
		return 0, err
	}
	var n int
	for _, req := range due {
		var familyIDs []uuid.UUID
		_ = json.Unmarshal(req.PrimaryOfFamilies, &familyIDs)

		// TODO Phase 2: archive PII + logs to S3 Intelligent Tiering BEFORE
		// the cascade. For now, just run the cascade and mark with an
		// empty cold backup key; the cold-purge cron will treat the row as
		// "nothing to purge."
		if err := s.repo.PerformHardDelete(ctx, req.UserID, familyIDs); err != nil {
			log.Printf("[ADS] PerformHardDelete failed for request %s: %v", req.ID, err)
			continue
		}
		coldExpires := time.Now().Add(s.coldBackupRetention)
		if err := s.repo.MarkHardDeleted(ctx, req.ID, "", coldExpires); err != nil {
			log.Printf("[ADS] MarkHardDeleted failed for request %s: %v", req.ID, err)
			continue
		}

		// Final confirmation email.
		if err := s.emailService.SendAccountHardDeletedEmail(req.UserEmailAtRequest, firstNameOf(req)); err != nil {
			log.Printf("[ADS] hard-deleted email failed: %v", err)
		}
		n++
	}
	return n, nil
}

// ProcessDueColdPurges is the second cron — purges cold backups whose
// retention has expired. v1 has no actual backups (no S3 hand-off yet),
// so this just transitions status. Once the cold-backup pipeline lands,
// this method also deletes the S3 object.
func (s *AccountDeletionService) ProcessDueColdPurges(ctx context.Context, limit int) (int, error) {
	due, err := s.repo.FindColdPurgeDue(ctx, limit)
	if err != nil {
		return 0, err
	}
	var n int
	for _, req := range due {
		// TODO Phase 2: actual S3 DELETE on req.ColdBackupS3Key
		if err := s.repo.MarkColdPurged(ctx, req.ID); err != nil {
			log.Printf("[ADS] MarkColdPurged failed: %v", err)
			continue
		}
		n++
	}
	return n, nil
}

// ----------------------------------------------------------------------------
// Data export — consent page + worker
// ----------------------------------------------------------------------------

// LookupExportJobByConsentToken validates the consent-page link.
func (s *AccountDeletionService) LookupExportJobByConsentToken(ctx context.Context, token string) (*models.DataExportJob, error) {
	if token == "" {
		return nil, errors.New("missing token")
	}
	return s.repo.FindExportByConsentTokenHash(ctx, sha256Hex(token))
}

// SubmitExportConsent moves the job to 'queued' for the worker to pick up.
func (s *AccountDeletionService) SubmitExportConsent(ctx context.Context, jobID uuid.UUID, csv, xlsx, sqlite bool) error {
	if !csv && !xlsx && !sqlite {
		return errors.New("select at least one format")
	}
	return s.repo.SubmitExportConsent(ctx, jobID, csv, xlsx, sqlite)
}

// ProcessExportJobs is the worker cron. v1 stubs the actual generation;
// real bundle building lands in Phase 2.
func (s *AccountDeletionService) ProcessExportJobs(ctx context.Context, limit int) (int, error) {
	queued, err := s.repo.FindQueuedExportJobs(ctx, limit)
	if err != nil {
		return 0, err
	}
	var n int
	for _, j := range queued {
		_ = s.repo.MarkExportProcessing(ctx, j.ID)
		// TODO Phase 2: actually generate the bundle, upload to S3, get a
		// pre-signed URL. For now mark failed with a helpful reason so the
		// user understands what to do.
		if err := s.repo.MarkExportFailed(ctx, j.ID, "Data export generation is not yet enabled in this build. Please contact support@mycarecompanion.net to receive your data."); err != nil {
			log.Printf("[ADS] MarkExportFailed: %v", err)
			continue
		}
		n++
	}
	return n, nil
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func randomNumericCode(digits int) (string, error) {
	max := big.NewInt(1)
	for i := 0; i < digits; i++ {
		max.Mul(max, big.NewInt(10))
	}
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%0*d", digits, n.Int64()), nil
}

func firstNameOf(req models.AccountDeletionRequest) string {
	if req.UserFirstNameAtRequest.Valid {
		return req.UserFirstNameAtRequest.String
	}
	return ""
}

func getenv(k string) string { return os.Getenv(k) }
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
