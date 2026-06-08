package service

import (
	"context"
	"testing"
	"time"

	"carecompanion/internal/config"
	"carecompanion/internal/models"
	"carecompanion/internal/repository"

	"github.com/google/uuid"
)

// fakeInsightRepo embeds the InsightRepository interface so any method we
// don't override panics if called. storeInsights for a non-alert tier-1
// result only touches ExistsRecentByDedupeKey + Create, so those are the
// only two we implement.
type fakeInsightRepo struct {
	repository.InsightRepository
	existing map[string]bool
	created  []*models.Insight
}

func (f *fakeInsightRepo) ExistsRecentByDedupeKey(ctx context.Context, childID uuid.UUID, key string, window time.Duration) (bool, error) {
	return f.existing[key], nil
}

func (f *fakeInsightRepo) Create(ctx context.Context, ins *models.Insight) error {
	f.created = append(f.created, ins)
	return nil
}

func newDedupeTestService(repo repository.InsightRepository) *AIInsightService {
	return &AIInsightService{
		insightRepo: repo,
		config:      &config.ClaudeConfig{LookbackDays: 30},
	}
}

func tier1Result(title string) aiInsightResult {
	return aiInsightResult{
		Tier:              1,
		Category:          "sleep",
		Title:             title,
		SimpleDescription: "desc",
		Confidence:        0.9,
		AlertWorthy:       false,
	}
}

// Cross-run dedup: an insight whose dedupe key already exists in the DB
// (from a previous scan) must not be re-created. This is the core bug —
// the AI scanner re-emitted the same concept on every daily run because
// it never consulted the persisted dedupe key.
func TestStoreInsightsSkipsKeyAlreadyInDB(t *testing.T) {
	title := "Stimulant Appetite & Growth Monitoring for Alex"
	key := aiInsightDedupeKey(1, "sleep", title)

	repo := &fakeInsightRepo{existing: map[string]bool{key: true}}
	svc := newDedupeTestService(repo)

	created, _ := svc.storeInsights(context.Background(), uuid.New(), uuid.New(), "Alex",
		[]aiInsightResult{tier1Result(title)}, nil)

	if created != 0 {
		t.Fatalf("expected 0 insights created (key already in DB), got %d", created)
	}
	if len(repo.created) != 0 {
		t.Fatalf("expected Create not called, got %d", len(repo.created))
	}
}

// In-batch dedup: two results in the same run that collapse to the same
// dedupe key must produce only one insight.
func TestStoreInsightsSkipsInBatchDuplicateKey(t *testing.T) {
	title := "Stimulant Appetite & Growth Monitoring for Alex"

	repo := &fakeInsightRepo{existing: map[string]bool{}}
	svc := newDedupeTestService(repo)

	created, _ := svc.storeInsights(context.Background(), uuid.New(), uuid.New(), "Alex",
		[]aiInsightResult{tier1Result(title), tier1Result(title)}, nil)

	if created != 1 {
		t.Fatalf("expected exactly 1 insight created from duplicate batch, got %d", created)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected Create called once, got %d", len(repo.created))
	}
}
