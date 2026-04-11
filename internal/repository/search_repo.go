package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// SearchRepository handles global search across all data
type SearchRepository interface {
	Search(ctx context.Context, familyID, userID uuid.UUID, query string) ([]models.SearchResult, error)
}

type searchRepo struct {
	db *sql.DB
}

// NewSearchRepo creates a new search repository
func NewSearchRepo(db *sql.DB) SearchRepository {
	return &searchRepo{db: db}
}

func (r *searchRepo) Search(ctx context.Context, familyID, userID uuid.UUID, query string) ([]models.SearchResult, error) {
	// Escape ILIKE wildcards in the search term
	escaped := strings.ReplaceAll(query, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `%`, `\%`)
	escaped = strings.ReplaceAll(escaped, `_`, `\_`)

	sqlQuery := `
		WITH family_children AS (
			SELECT c.id, c.first_name
			FROM children c
			WHERE c.family_id = $1 AND c.is_active = true
		)
		(
			SELECT bl.id, 'behavior' AS category, fc.id AS child_id, fc.first_name AS child_name,
				   COALESCE(bl.notes, '') AS matched_text, bl.log_date, bl.created_at
			FROM behavior_logs bl
			JOIN family_children fc ON bl.child_id = fc.id
			WHERE bl.notes ILIKE '%' || $2 || '%' ESCAPE '\'
			ORDER BY bl.created_at DESC LIMIT 5
		)
		UNION ALL
		(
			SELECT sl.id, 'sleep', fc.id, fc.first_name,
				   COALESCE(sl.notes, ''), sl.log_date, sl.created_at
			FROM sleep_logs sl
			JOIN family_children fc ON sl.child_id = fc.id
			WHERE sl.notes ILIKE '%' || $2 || '%' ESCAPE '\'
			ORDER BY sl.created_at DESC LIMIT 5
		)
		UNION ALL
		(
			SELECT dl.id, 'diet', fc.id, fc.first_name,
				   COALESCE(dl.notes, ''), dl.log_date, dl.created_at
			FROM diet_logs dl
			JOIN family_children fc ON dl.child_id = fc.id
			WHERE dl.notes ILIKE '%' || $2 || '%' ESCAPE '\'
			ORDER BY dl.created_at DESC LIMIT 5
		)
		UNION ALL
		(
			SELECT bwl.id, 'bowel', fc.id, fc.first_name,
				   COALESCE(bwl.notes, ''), bwl.log_date, bwl.created_at
			FROM bowel_logs bwl
			JOIN family_children fc ON bwl.child_id = fc.id
			WHERE bwl.notes ILIKE '%' || $2 || '%' ESCAPE '\'
			ORDER BY bwl.created_at DESC LIMIT 5
		)
		UNION ALL
		(
			SELECT snl.id, 'sensory', fc.id, fc.first_name,
				   COALESCE(snl.notes, ''), snl.log_date, snl.created_at
			FROM sensory_logs snl
			JOIN family_children fc ON snl.child_id = fc.id
			WHERE snl.notes ILIKE '%' || $2 || '%' ESCAPE '\'
			ORDER BY snl.created_at DESC LIMIT 5
		)
		UNION ALL
		(
			SELECT sol.id, 'social', fc.id, fc.first_name,
				   COALESCE(sol.notes, ''), sol.log_date, sol.created_at
			FROM social_logs sol
			JOIN family_children fc ON sol.child_id = fc.id
			WHERE sol.notes ILIKE '%' || $2 || '%' ESCAPE '\'
			ORDER BY sol.created_at DESC LIMIT 5
		)
		UNION ALL
		(
			SELECT spl.id, 'speech', fc.id, fc.first_name,
				   COALESCE(spl.notes, ''), spl.log_date, spl.created_at
			FROM speech_logs spl
			JOIN family_children fc ON spl.child_id = fc.id
			WHERE spl.notes ILIKE '%' || $2 || '%' ESCAPE '\'
			ORDER BY spl.created_at DESC LIMIT 5
		)
		UNION ALL
		(
			SELECT szl.id, 'seizure', fc.id, fc.first_name,
				   COALESCE(szl.seizure_type, '') || ' ' || COALESCE(szl.notes, ''), szl.log_date, szl.created_at
			FROM seizure_logs szl
			JOIN family_children fc ON szl.child_id = fc.id
			WHERE szl.notes ILIKE '%' || $2 || '%' ESCAPE '\'
			   OR szl.seizure_type ILIKE '%' || $2 || '%' ESCAPE '\'
			ORDER BY szl.created_at DESC LIMIT 5
		)
		UNION ALL
		(
			SELECT tl.id, 'therapy', fc.id, fc.first_name,
				   COALESCE(tl.therapy_type, '') || ' - ' || COALESCE(tl.progress_notes, '') || ' ' || COALESCE(tl.parent_notes, ''),
				   tl.log_date, tl.created_at
			FROM therapy_logs tl
			JOIN family_children fc ON tl.child_id = fc.id
			WHERE tl.therapy_type ILIKE '%' || $2 || '%' ESCAPE '\'
			   OR tl.progress_notes ILIKE '%' || $2 || '%' ESCAPE '\'
			   OR tl.parent_notes ILIKE '%' || $2 || '%' ESCAPE '\'
			ORDER BY tl.created_at DESC LIMIT 5
		)
		UNION ALL
		(
			SELECT hel.id, 'health_events', fc.id, fc.first_name,
				   COALESCE(hel.event_type, '') || ' - ' || COALESCE(hel.description, '') || ' ' || COALESCE(hel.notes, ''),
				   hel.log_date, hel.created_at
			FROM health_event_logs hel
			JOIN family_children fc ON hel.child_id = fc.id
			WHERE hel.event_type ILIKE '%' || $2 || '%' ESCAPE '\'
			   OR hel.description ILIKE '%' || $2 || '%' ESCAPE '\'
			   OR hel.notes ILIKE '%' || $2 || '%' ESCAPE '\'
			ORDER BY hel.created_at DESC LIMIT 5
		)
		UNION ALL
		(
			SELECT ml.id, 'medication_logs', fc.id, fc.first_name,
				   COALESCE(m.name, '') || ': ' || COALESCE(ml.notes, ''),
				   ml.log_date, ml.created_at
			FROM medication_logs ml
			JOIN family_children fc ON ml.child_id = fc.id
			LEFT JOIN medications m ON ml.medication_id = m.id
			WHERE ml.notes ILIKE '%' || $2 || '%' ESCAPE '\'
			   OR m.name ILIKE '%' || $2 || '%' ESCAPE '\'
			ORDER BY ml.created_at DESC LIMIT 5
		)
		UNION ALL
		(
			SELECT cm.id, 'chat', $1::uuid AS child_id, '' AS child_name,
				   cm.message_text, cm.created_at::date, cm.created_at
			FROM chat_messages cm
			JOIN chat_threads ct ON cm.thread_id = ct.id
			JOIN chat_participants cp ON cp.thread_id = ct.id AND cp.user_id = $3
			WHERE ct.family_id = $1 AND cm.message_text ILIKE '%' || $2 || '%' ESCAPE '\'
			ORDER BY cm.created_at DESC LIMIT 5
		)
		UNION ALL
		(
			SELECT a.id, 'alerts', fc.id, fc.first_name,
				   COALESCE(a.title, '') || ' - ' || COALESCE(a.description, ''),
				   a.created_at::date, a.created_at
			FROM alerts a
			JOIN family_children fc ON a.child_id = fc.id
			WHERE a.title ILIKE '%' || $2 || '%' ESCAPE '\'
			   OR a.description ILIKE '%' || $2 || '%' ESCAPE '\'
			ORDER BY a.created_at DESC LIMIT 5
		)
		UNION ALL
		(
			SELECT m.id, 'medications', fc.id, fc.first_name,
				   m.name, m.created_at::date, m.created_at
			FROM medications m
			JOIN family_children fc ON m.child_id = fc.id
			WHERE m.is_active = true AND m.name ILIKE '%' || $2 || '%' ESCAPE '\'
			ORDER BY m.name ASC LIMIT 5
		)
		ORDER BY created_at DESC
		LIMIT 30
	`

	rows, err := r.db.QueryContext(ctx, sqlQuery, familyID, escaped, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.SearchResult
	for rows.Next() {
		var sr models.SearchResult
		var logDate time.Time
		if err := rows.Scan(&sr.ID, &sr.Category, &sr.ChildID, &sr.ChildName,
			&sr.MatchedText, &logDate, &sr.CreatedAt); err != nil {
			return nil, err
		}
		sr.LogDate = logDate
		results = append(results, sr)
	}
	return results, rows.Err()
}
