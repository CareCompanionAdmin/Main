package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"

	"carecompanion/internal/config"
)

// AttachmentStorage abstracts where ticket attachment bytes live. Two
// implementations: local filesystem (dev / single-host) and S3 (prod, where
// ASG instances get replaced and local disk vanishes).
//
// Save returns a (driver, path) pair that's persisted in
// ticket_attachments.storage_driver / .storage_path so we always know how
// to fetch/delete a row even if the active driver changes later.
type AttachmentStorage interface {
	Driver() string
	Save(ctx context.Context, ticketID uuid.UUID, filename string, contentType string, body io.Reader) (path string, sizeBytes int64, err error)
	Open(ctx context.Context, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
}

// NewAttachmentStorage picks the driver based on config. If ATTACHMENT_S3_BUCKET
// is set we use S3; otherwise we fall back to the local filesystem under
// {UploadDir}/ticket_attachments. The chosen driver is logged at startup so
// it's obvious which is in play.
func NewAttachmentStorage(cfg *config.StorageConfig) AttachmentStorage {
	if cfg.S3Bucket != "" {
		s3s, err := newS3Storage(cfg)
		if err != nil {
			log.Printf("[STORAGE] S3 init failed (%v) — falling back to local filesystem", err)
		} else {
			log.Printf("[STORAGE] using S3 driver bucket=%s prefix=%s region=%s", cfg.S3Bucket, cfg.S3Prefix, cfg.S3Region)
			return s3s
		}
	}
	root := filepath.Join(cfg.UploadDir, "ticket_attachments")
	if err := os.MkdirAll(root, 0o750); err != nil {
		log.Printf("[STORAGE] could not create %s: %v (uploads will fail)", root, err)
	}
	log.Printf("[STORAGE] using local filesystem driver root=%s", root)
	return &localFSStorage{root: root}
}

// ----------------------------------------------------------------------------
// local filesystem driver
// ----------------------------------------------------------------------------

type localFSStorage struct {
	root string
}

func (l *localFSStorage) Driver() string { return "localfs" }

func (l *localFSStorage) Save(ctx context.Context, ticketID uuid.UUID, filename string, contentType string, body io.Reader) (string, int64, error) {
	dir := filepath.Join(l.root, ticketID.String())
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", 0, err
	}
	rel := filepath.Join(ticketID.String(), uuid.New().String()+extFromName(filename, contentType))
	full := filepath.Join(l.root, rel)
	f, err := os.Create(full)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	n, err := io.Copy(f, body)
	if err != nil {
		_ = os.Remove(full)
		return "", 0, err
	}
	return rel, n, nil
}

func (l *localFSStorage) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	full := filepath.Join(l.root, path)
	// Reject paths that try to escape the root.
	clean := filepath.Clean(full)
	if !strings.HasPrefix(clean, filepath.Clean(l.root)+string(os.PathSeparator)) {
		return nil, fmt.Errorf("invalid attachment path")
	}
	return os.Open(clean)
}

func (l *localFSStorage) Delete(ctx context.Context, path string) error {
	full := filepath.Join(l.root, path)
	clean := filepath.Clean(full)
	if !strings.HasPrefix(clean, filepath.Clean(l.root)+string(os.PathSeparator)) {
		return fmt.Errorf("invalid attachment path")
	}
	if err := os.Remove(clean); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ----------------------------------------------------------------------------
// S3 driver
// ----------------------------------------------------------------------------

type s3Storage struct {
	client *s3.Client
	bucket string
	prefix string
}

func newS3Storage(cfg *config.StorageConfig) (*s3Storage, error) {
	awscfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(cfg.S3Region))
	if err != nil {
		return nil, err
	}
	prefix := cfg.S3Prefix
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return &s3Storage{
		client: s3.NewFromConfig(awscfg),
		bucket: cfg.S3Bucket,
		prefix: prefix,
	}, nil
}

func (s *s3Storage) Driver() string { return "s3" }

func (s *s3Storage) Save(ctx context.Context, ticketID uuid.UUID, filename string, contentType string, body io.Reader) (string, int64, error) {
	key := s.prefix + ticketID.String() + "/" + uuid.New().String() + extFromName(filename, contentType)

	// PutObject needs a seeker for retries; buffer through a temp file so we
	// can both stream to S3 and report the byte count.
	tmp, err := os.CreateTemp("", "ticket-attach-*")
	if err != nil {
		return "", 0, err
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()
	n, err := io.Copy(tmp, body)
	if err != nil {
		return "", 0, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return "", 0, err
	}
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        tmp,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", 0, err
	}
	// Strip the prefix from the persisted path so changing the prefix later
	// doesn't break old rows: we always re-prepend at fetch time.
	rel := strings.TrimPrefix(key, s.prefix)
	return rel, n, nil
}

func (s *s3Storage) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	key := s.prefix + path
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (s *s3Storage) Delete(ctx context.Context, path string) error {
	key := s.prefix + path
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

// extFromName picks a sensible extension from the original filename, falling
// back to a content-type guess. Used so files written to disk/S3 keep a hint
// about their type even though their stored name is a UUID.
func extFromName(filename, contentType string) string {
	if i := strings.LastIndex(filename, "."); i >= 0 && i < len(filename)-1 {
		ext := strings.ToLower(filename[i:])
		// Sanity-check the extension is alphanum-only.
		ok := len(ext) >= 2 && len(ext) <= 6
		for j := 1; j < len(ext) && ok; j++ {
			c := ext[j]
			if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') {
				ok = false
			}
		}
		if ok {
			return ext
		}
	}
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/heic", "image/heif":
		return ".heic"
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "video/webm":
		return ".webm"
	case "video/3gpp":
		return ".3gp"
	}
	return ""
}
