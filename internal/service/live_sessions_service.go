package service

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// LiveSessionRow is a unified row across the three session sources
// (user JWT, admin JWT, SSH) so the admin page renders a single template
// structure regardless of source.
type LiveSessionRow struct {
	ID             string    `json:"id"`   // session UUID for JWT, "ssh:<tty>" for SSH
	Kind           string    `json:"kind"` // "user" | "admin" | "ssh"
	Env            string    `json:"env"`  // "development" | "production"
	UserEmail      string    `json:"user_email,omitempty"`
	UserFirstName  string    `json:"user_first_name,omitempty"`
	UserLastName   string    `json:"user_last_name,omitempty"`
	FamilyName     string    `json:"family_name,omitempty"`
	SystemRole     string    `json:"system_role,omitempty"`
	IPAtStart      string    `json:"ip_at_start,omitempty"`
	UserAgent      string    `json:"user_agent,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	LastSeenAt     time.Time `json:"last_seen_at"`
	StartedDisplay string    `json:"started_display,omitempty"` // raw login-time string for SSH (LoginTime is already pre-formatted)
	TTY            string    `json:"tty,omitempty"`             // SSH only
	IsLocalEnv     bool      `json:"is_local_env"`              // controls kill-button visibility client-side
}

// LiveSnapshot is what the page handler returns to the template / API client.
// CrossEnvError is non-empty when the cross-env pool is configured but the
// query failed; the page still renders local + SSH rows alongside a banner.
type LiveSnapshot struct {
	Users         []LiveSessionRow `json:"users"`
	Admins        []LiveSessionRow `json:"admins"`
	SSH           []LiveSessionRow `json:"ssh"`
	LocalEnv      string           `json:"local_env"`
	CrossEnvShown bool             `json:"cross_env_shown"`
	CrossEnvError string           `json:"cross_env_error,omitempty"`
}

type LiveSessionsService struct {
	localRepo  repository.SessionRepository
	prodRepo   repository.SessionRepository // nil when SESSIONS_PROD_DB_DSN unset
	devModeSvc *DevModeService              // may be nil (e.g., on prod where DevMode isn't initialized, or pre-wiring)
	localEnv   string
}

func NewLiveSessionsService(localRepo, prodRepo repository.SessionRepository, devModeSvc *DevModeService, localEnv string) *LiveSessionsService {
	return &LiveSessionsService{
		localRepo:  localRepo,
		prodRepo:   prodRepo,
		devModeSvc: devModeSvc,
		localEnv:   localEnv,
	}
}

// SetDevModeService allows post-construction wiring when DevModeService is
// built later in the boot sequence (currently in cmd/server/main.go after
// NewServices returns). Safe to call multiple times; nil is allowed.
func (s *LiveSessionsService) SetDevModeService(d *DevModeService) {
	s.devModeSvc = d
}

// Snapshot reads all three sources and merges them. Errors are logged and
// surfaced (cross-env error in the snapshot, others as log warnings) but
// never propagate — the page must render even when one source is degraded.
func (s *LiveSessionsService) Snapshot(ctx context.Context) LiveSnapshot {
	out := LiveSnapshot{LocalEnv: s.localEnv, CrossEnvShown: s.prodRepo != nil}

	userKind := models.SessionKindUser
	adminKind := models.SessionKindAdmin

	// Local sessions.
	if localUsers, err := s.localRepo.ListActive(ctx, &userKind, 500); err != nil {
		log.Printf("[LIVE_SESSIONS] local user list failed: %v", err)
	} else {
		for _, r := range localUsers {
			out.Users = append(out.Users, toLiveRow(r, true))
		}
	}
	if localAdmins, err := s.localRepo.ListActive(ctx, &adminKind, 500); err != nil {
		log.Printf("[LIVE_SESSIONS] local admin list failed: %v", err)
	} else {
		for _, r := range localAdmins {
			out.Admins = append(out.Admins, toLiveRow(r, true))
		}
	}

	// Cross-env sessions (read-only).
	if s.prodRepo != nil {
		if prodUsers, err := s.prodRepo.ListActive(ctx, &userKind, 500); err != nil {
			out.CrossEnvError = err.Error()
		} else {
			for _, r := range prodUsers {
				out.Users = append(out.Users, toLiveRow(r, false))
			}
			if prodAdmins, err := s.prodRepo.ListActive(ctx, &adminKind, 500); err != nil {
				log.Printf("[LIVE_SESSIONS] cross-env admin list failed: %v", err)
			} else {
				for _, r := range prodAdmins {
					out.Admins = append(out.Admins, toLiveRow(r, false))
				}
			}
		}
	}

	// SSH sessions (local only). DevModeService may be nil on prod where
	// the service isn't constructed, or pre-wiring during boot.
	if s.devModeSvc != nil {
		if ssh, err := s.devModeSvc.ListSSHSessions(ctx); err != nil {
			log.Printf("[LIVE_SESSIONS] ssh list failed: %v", err)
		} else {
			for _, sh := range ssh {
				out.SSH = append(out.SSH, LiveSessionRow{
					ID:             "ssh:" + sh.TTY,
					Kind:           "ssh",
					Env:            s.localEnv,
					UserEmail:      sh.Username,
					IPAtStart:      sh.SourceIP,
					StartedDisplay: sh.LoginTime,
					TTY:            sh.TTY,
					IsLocalEnv:     true,
				})
			}
		}
	}

	return out
}

func toLiveRow(r models.Session, isLocal bool) LiveSessionRow {
	row := LiveSessionRow{
		ID:         r.ID.String(),
		Kind:       string(r.Kind),
		CreatedAt:  r.CreatedAt,
		LastSeenAt: r.LastSeenAt,
		IsLocalEnv: isLocal,
	}
	if r.UserEmail.Valid {
		row.UserEmail = r.UserEmail.String
	}
	if r.UserFirstName.Valid {
		row.UserFirstName = r.UserFirstName.String
	}
	if r.UserLastName.Valid {
		row.UserLastName = r.UserLastName.String
	}
	if r.FamilyName.Valid {
		row.FamilyName = r.FamilyName.String
	}
	if r.SystemRole.Valid {
		row.SystemRole = r.SystemRole.String
	}
	if r.IPAtStart.Valid {
		row.IPAtStart = r.IPAtStart.String
	}
	if r.UserAgent.Valid {
		row.UserAgent = r.UserAgent.String
	}
	if r.EnvName.Valid {
		row.Env = r.EnvName.String
	}
	return row
}

// RevokeBulk loops auth.RevokeSession over a list of UUIDs (all assumed
// LOCAL env — the caller is responsible for filtering cross-env IDs out
// before calling, since cross-env kill is not supported).
func (s *LiveSessionsService) RevokeBulk(ctx context.Context, ids []uuid.UUID, auth *AuthService) (revoked int) {
	for _, id := range ids {
		if err := auth.RevokeSession(ctx, id); err == nil {
			revoked++
		}
	}
	return
}
