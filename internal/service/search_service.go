package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// categoryMeta maps category keys to display names and icons
var categoryMeta = map[string]string{
	"behavior":        "Behavior",
	"sleep":           "Sleep",
	"diet":            "Diet",
	"bowel":           "Bowel",
	"sensory":         "Sensory",
	"social":          "Social",
	"speech":          "Speech",
	"seizure":         "Seizure",
	"therapy":         "Therapy",
	"health_events":   "Health Events",
	"medication_logs": "Medication Logs",
	"medications":     "Medications",
	"chat":            "Chat",
	"alerts":          "Alerts",
}

// categoryOrder defines the display order
var categoryOrder = []string{
	"behavior", "sleep", "diet", "medications", "medication_logs",
	"therapy", "health_events", "sensory", "social", "seizure",
	"speech", "bowel", "alerts", "chat",
}

// SearchService handles global search
type SearchService struct {
	searchRepo repository.SearchRepository
}

// NewSearchService creates a new search service
func NewSearchService(searchRepo repository.SearchRepository) *SearchService {
	return &SearchService{searchRepo: searchRepo}
}

// Search performs a global search across all data types
func (s *SearchService) Search(ctx context.Context, familyID, userID uuid.UUID, query string) (*models.SearchResponse, error) {
	query = strings.TrimSpace(query)
	if len(query) < 2 {
		return &models.SearchResponse{Query: query}, nil
	}
	if len(query) > 100 {
		query = query[:100]
	}

	results, err := s.searchRepo.Search(ctx, familyID, userID, query)
	if err != nil {
		return nil, err
	}

	// Group results by category
	grouped := make(map[string][]models.SearchResultItem)
	for _, r := range results {
		item := models.SearchResultItem{
			ID:        r.ID,
			ChildName: r.ChildName,
			Snippet:   buildSnippet(r.MatchedText, query, 80),
			Date:      r.LogDate.Format("Jan 2, 2006"),
			URL:       buildURL(r),
		}
		grouped[r.Category] = append(grouped[r.Category], item)
	}

	// Build ordered categories
	var categories []models.SearchCategory
	totalCount := 0
	for _, key := range categoryOrder {
		items, ok := grouped[key]
		if !ok || len(items) == 0 {
			continue
		}
		name := categoryMeta[key]
		if name == "" {
			name = key
		}
		categories = append(categories, models.SearchCategory{
			Name:    name,
			Key:     key,
			Results: items,
		})
		totalCount += len(items)
	}

	return &models.SearchResponse{
		Query:      query,
		TotalCount: totalCount,
		Categories: categories,
	}, nil
}

// buildSnippet extracts a snippet around the matched text
func buildSnippet(text, query string, maxLen int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	// Find match position (case-insensitive)
	lower := strings.ToLower(text)
	queryLower := strings.ToLower(query)
	pos := strings.Index(lower, queryLower)

	if pos < 0 {
		// Match not found in combined text, just truncate
		if len(text) > maxLen {
			return text[:maxLen] + "..."
		}
		return text
	}

	// Center the window around the match
	start := pos - (maxLen-len(query))/2
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(text) {
		end = len(text)
		start = end - maxLen
		if start < 0 {
			start = 0
		}
	}

	snippet := text[start:end]
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(text) {
		snippet = snippet + "..."
	}

	return snippet
}

// buildURL creates the navigation URL for a search result
func buildURL(r models.SearchResult) string {
	dateStr := r.LogDate.Format("2006-01-02")
	childID := r.ChildID.String()
	id := r.ID.String()

	switch r.Category {
	case "behavior", "sleep", "diet", "bowel", "sensory", "social", "speech",
		"seizure", "therapy", "health_events", "medication_logs":
		return fmt.Sprintf("/child/%s/logs?date=%s&highlight=%s-%s", childID, dateStr, r.Category, id)
	case "medications":
		return fmt.Sprintf("/child/%s/medications#med-%s", childID, id)
	case "alerts":
		return fmt.Sprintf("/child/%s/alerts#alert-%s", childID, id)
	case "chat":
		return fmt.Sprintf("/chat#msg-%s", id)
	default:
		return "/dashboard"
	}
}
