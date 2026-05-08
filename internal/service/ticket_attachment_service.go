package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

var (
	ErrAttachmentTooBig         = errors.New("attachment exceeds maximum allowed size")
	ErrAttachmentLimitReached   = errors.New("maximum number of attachments per ticket reached")
	ErrAttachmentTypeNotAllowed = errors.New("file type not allowed")
	ErrAttachmentNotFound       = errors.New("attachment not found")
	ErrAttachmentForbidden      = errors.New("not allowed to access this attachment")
)

// allowedMimeTypes is the whitelist of content types accepted for upload.
// Includes iOS-native HEIC/MOV and Android-native 3GP. Image/video only —
// no PDFs, docs, etc., to keep PHI surface narrow.
var allowedMimeTypes = map[string]string{
	"image/jpeg":      "image",
	"image/jpg":       "image",
	"image/png":       "image",
	"image/gif":       "image",
	"image/webp":      "image",
	"image/heic":      "image",
	"image/heif":      "image",
	"video/mp4":       "video",
	"video/quicktime": "video", // .mov, iOS native
	"video/webm":      "video", // also used by in-browser MediaRecorder
	"video/3gpp":      "video", // .3gp, Android
	"video/3gpp2":     "video",
	"video/x-m4v":     "video",
	"audio/webm":      "video", // recording-only, no display capture
}

// TicketAttachmentService coordinates uploads, listing, and bulk deletion of
// ticket attachments. The actual bytes are handed off to AttachmentStorage;
// this service owns the validation + DB row + cross-table close hooks.
type TicketAttachmentService struct {
	repo        repository.TicketAttachmentRepository
	adminRepo   repository.AdminRepository
	storage     AttachmentStorage
	maxBytes    int64
	maxPerTkt   int
}

// NewTicketAttachmentService builds the service.
func NewTicketAttachmentService(repo repository.TicketAttachmentRepository, adminRepo repository.AdminRepository, storage AttachmentStorage, maxBytes int64, maxPerTkt int) *TicketAttachmentService {
	if maxBytes <= 0 {
		maxBytes = 25 * 1024 * 1024
	}
	if maxPerTkt <= 0 {
		maxPerTkt = 5
	}
	return &TicketAttachmentService{
		repo: repo, adminRepo: adminRepo, storage: storage,
		maxBytes: maxBytes, maxPerTkt: maxPerTkt,
	}
}

// MaxBytes / MaxPerTicket expose the active limits so handlers can enforce
// the same cap before accepting the multipart form.
func (s *TicketAttachmentService) MaxBytes() int64    { return s.maxBytes }
func (s *TicketAttachmentService) MaxPerTicket() int  { return s.maxPerTkt }

// UploadInput carries everything the upload path needs.
type UploadInput struct {
	TicketID    uuid.UUID
	UploaderID  uuid.UUID
	Filename    string
	ContentType string
	Body        io.Reader
	// Kind override — used so in-browser screen recordings get tagged as
	// "recording" instead of generic "video". Empty means "infer from type".
	KindOverride string
}

// Upload validates, persists the bytes via storage, and writes the row.
func (s *TicketAttachmentService) Upload(ctx context.Context, in UploadInput) (*repository.TicketAttachment, error) {
	if in.TicketID == uuid.Nil {
		return nil, errors.New("ticket id is required")
	}
	contentType := strings.ToLower(in.ContentType)
	kind, ok := allowedMimeTypes[contentType]
	if !ok {
		return nil, ErrAttachmentTypeNotAllowed
	}
	if in.KindOverride != "" {
		kind = in.KindOverride
	}

	// Per-ticket cap. Cheap query — runs before we touch storage.
	count, err := s.repo.CountByTicket(ctx, in.TicketID)
	if err != nil {
		return nil, err
	}
	if count >= s.maxPerTkt {
		return nil, ErrAttachmentLimitReached
	}

	// Wrap the body in a size-limited reader so a lying Content-Length
	// can't blow past the cap.
	limited := io.LimitReader(in.Body, s.maxBytes+1)
	path, sizeBytes, err := s.storage.Save(ctx, in.TicketID.String(), in.Filename, contentType, limited)
	if err != nil {
		return nil, err
	}
	if sizeBytes > s.maxBytes {
		// Rollback on oversize. Best-effort; log if cleanup fails.
		if delErr := s.storage.Delete(ctx, path); delErr != nil {
			log.Printf("[ATTACH] cleanup of oversize upload failed: %v", delErr)
		}
		return nil, ErrAttachmentTooBig
	}

	att := &repository.TicketAttachment{
		TicketID:      in.TicketID,
		UploaderID:    nullUUID(in.UploaderID),
		Kind:          kind,
		ContentType:   contentType,
		OriginalName:  sanitizeOriginalName(in.Filename),
		StoragePath:   path,
		StorageDriver: s.storage.Driver(),
		SizeBytes:     sizeBytes,
	}
	if err := s.repo.Create(ctx, att); err != nil {
		// Roll back the storage write so we don't orphan bytes.
		if delErr := s.storage.Delete(ctx, path); delErr != nil {
			log.Printf("[ATTACH] cleanup after DB insert failed: %v (orphaned path %s)", delErr, path)
		}
		return nil, err
	}
	return att, nil
}

// List returns attachments for a ticket. Caller must enforce auth.
func (s *TicketAttachmentService) List(ctx context.Context, ticketID uuid.UUID) ([]repository.TicketAttachment, error) {
	return s.repo.ListByTicket(ctx, ticketID)
}

// FetchForUser opens an attachment after verifying the user owns the ticket.
// Returns the file body + the attachment metadata; caller is responsible for
// closing the body.
func (s *TicketAttachmentService) FetchForUser(ctx context.Context, attID, userID uuid.UUID) (io.ReadCloser, *repository.TicketAttachment, error) {
	att, err := s.repo.GetByID(ctx, attID)
	if err != nil {
		return nil, nil, err
	}
	if att == nil {
		return nil, nil, ErrAttachmentNotFound
	}
	ticket, err := s.adminRepo.GetTicketByID(ctx, att.TicketID)
	if err != nil {
		return nil, nil, err
	}
	if ticket == nil || !ticket.UserID.Valid || ticket.UserID.UUID != userID {
		return nil, nil, ErrAttachmentForbidden
	}
	body, err := s.storage.Open(ctx, att.StoragePath)
	if err != nil {
		return nil, nil, err
	}
	return body, att, nil
}

// FetchForAdmin opens an attachment without ownership checks. Caller must
// already have passed the support/super_admin middleware.
func (s *TicketAttachmentService) FetchForAdmin(ctx context.Context, attID uuid.UUID) (io.ReadCloser, *repository.TicketAttachment, error) {
	att, err := s.repo.GetByID(ctx, attID)
	if err != nil {
		return nil, nil, err
	}
	if att == nil {
		return nil, nil, ErrAttachmentNotFound
	}
	body, err := s.storage.Open(ctx, att.StoragePath)
	if err != nil {
		return nil, nil, err
	}
	return body, att, nil
}

// DeleteByOwner lets a user delete their own attachment from a ticket they
// own (e.g. they uploaded the wrong screenshot before submitting).
func (s *TicketAttachmentService) DeleteByOwner(ctx context.Context, attID, userID uuid.UUID) error {
	att, err := s.repo.GetByID(ctx, attID)
	if err != nil {
		return err
	}
	if att == nil {
		return ErrAttachmentNotFound
	}
	ticket, err := s.adminRepo.GetTicketByID(ctx, att.TicketID)
	if err != nil {
		return err
	}
	if ticket == nil || !ticket.UserID.Valid || ticket.UserID.UUID != userID {
		return ErrAttachmentForbidden
	}
	if err := s.storage.Delete(ctx, att.StoragePath); err != nil {
		log.Printf("[ATTACH] storage delete failed for %s: %v", att.ID, err)
	}
	return s.repo.DeleteByID(ctx, attID)
}

// DeleteAllForTicket nukes every attachment row + storage object for the
// given ticket. Idempotent: safe to call multiple times. This is the hook
// fired when a ticket transitions to closed/resolved.
func (s *TicketAttachmentService) DeleteAllForTicket(ctx context.Context, ticketID uuid.UUID) {
	atts, err := s.repo.DeleteAllByTicket(ctx, ticketID)
	if err != nil {
		log.Printf("[ATTACH] delete-all DB step failed for %s: %v", ticketID, err)
		return
	}
	for _, a := range atts {
		if err := s.storage.Delete(ctx, a.StoragePath); err != nil {
			log.Printf("[ATTACH] delete-all storage step failed for %s (path %s): %v", a.ID, a.StoragePath, err)
		}
	}
	if len(atts) > 0 {
		log.Printf("[ATTACH] purged %d attachment(s) on close of ticket %s", len(atts), ticketID)
	}
}

// sanitizeOriginalName trims path separators and clamps length so the
// original filename can be safely echoed back to admins/users.
func sanitizeOriginalName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	if len(name) > 200 {
		name = name[:200]
	}
	if name == "" {
		return "upload"
	}
	return name
}

// Compile-time helper so we can construct NullUUID values inline elsewhere
// in the service package without importing models.
var _ = models.NullUUID{}

// fmt is used by callers; this prevents goimports from stripping the import
// when this file is edited in isolation.
var _ = fmt.Sprintf
