package service

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"carecompanion/internal/config"
)

func TestLocalFSBlobStorage_RoundTrip(t *testing.T) {
	tmp, err := os.MkdirTemp("", "blob-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	cfg := &config.StorageConfig{UploadDir: tmp}
	store := NewBlobStorage(cfg, "reports", "")

	if got := store.Driver(); got != "localfs" {
		t.Fatalf("Driver() = %q, want localfs", got)
	}

	want := []byte("hello pdf bytes")
	path, n, err := store.Save(context.Background(), "abc-123", "report.pdf", "application/pdf", bytes.NewReader(want))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if n != int64(len(want)) {
		t.Fatalf("Save returned size %d, want %d", n, len(want))
	}
	if !strings.HasPrefix(path, "abc-123/") {
		t.Fatalf("Save returned path %q, want prefix abc-123/", path)
	}

	rc, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("read back %q, want %q", got, want)
	}

	if err := store.Delete(context.Background(), path); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Open(context.Background(), path); err == nil {
		t.Fatal("Open after Delete returned no error; expected NotExist")
	} else if !os.IsNotExist(err) {
		t.Fatalf("Open after Delete returned %v, want NotExist", err)
	}
}
