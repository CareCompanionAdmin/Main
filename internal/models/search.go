package models

import (
	"time"

	"github.com/google/uuid"
)

// SearchResult is a raw result from the database search query
type SearchResult struct {
	ID          uuid.UUID `json:"id"`
	Category    string    `json:"category"`
	ChildID     uuid.UUID `json:"child_id"`
	ChildName   string    `json:"child_name"`
	MatchedText string    `json:"matched_text"`
	LogDate     time.Time `json:"log_date"`
	CreatedAt   time.Time `json:"created_at"`
}

// SearchResultItem is a formatted result for the API response
type SearchResultItem struct {
	ID        uuid.UUID `json:"id"`
	ChildName string    `json:"child_name"`
	Snippet   string    `json:"snippet"`
	Date      string    `json:"date"`
	URL       string    `json:"url"`
}

// SearchCategory groups results by type
type SearchCategory struct {
	Name    string             `json:"name"`
	Key     string             `json:"key"`
	Results []SearchResultItem `json:"results"`
}

// SearchResponse is the API response for global search
type SearchResponse struct {
	Query      string           `json:"query"`
	TotalCount int              `json:"total_count"`
	Categories []SearchCategory `json:"categories"`
}
