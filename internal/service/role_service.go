package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/auth"
	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// RoleService manages custom admin roles and serves as the runtime
// PermResolver consulted by auth.Matrix() for non-builtin role names.
// Implements an in-memory cache with 60s TTL + explicit invalidation
// on mutation so the per-request Matrix() lookup stays sub-microsecond.
type RoleService struct {
	repo  repository.RoleRepository
	cache *permCache
}

func NewRoleService(repo repository.RoleRepository) *RoleService {
	return &RoleService{
		repo:  repo,
		cache: newPermCache(60 * time.Second),
	}
}

var nameFormat = regexp.MustCompile(`^[a-z][a-z0-9_]{1,49}$`)
var builtinNames = map[string]bool{
	"super_admin": true,
	"support":     true,
	"marketing":   true,
	"partner":     true,
}

func (s *RoleService) List(ctx context.Context) ([]models.CustomRole, error) {
	return s.repo.List(ctx)
}

func (s *RoleService) Get(ctx context.Context, id uuid.UUID) (*models.CustomRole, error) {
	return s.repo.Get(ctx, id)
}

func (s *RoleService) GetByName(ctx context.Context, name string) (*models.CustomRole, error) {
	return s.repo.GetByName(ctx, name)
}

// Create validates and persists a new custom role.
func (s *RoleService) Create(ctx context.Context, role *models.CustomRole) error {
	if err := s.validate(role); err != nil {
		return err
	}
	if err := s.repo.Create(ctx, role); err != nil {
		return err
	}
	s.cache.invalidate()
	return nil
}

func (s *RoleService) Update(ctx context.Context, role *models.CustomRole) error {
	if strings.TrimSpace(role.DisplayName) == "" {
		return fmt.Errorf("display name required")
	}
	for section, level := range role.Permissions {
		if level != "" && level != "none" && level != "read" && level != "write" {
			return fmt.Errorf("invalid level %q for section %q", level, section)
		}
	}
	if err := s.repo.Update(ctx, role); err != nil {
		return err
	}
	s.cache.invalidate()
	return nil
}

// Delete refuses to drop a role still assigned to one or more admin
// users; callers get a structured error so the UI can render the
// offending emails.
type RoleInUseError struct {
	Count  int
	Emails []string
}

func (e *RoleInUseError) Error() string {
	return fmt.Sprintf("role still assigned to %d admin user(s)", e.Count)
}

func (s *RoleService) Delete(ctx context.Context, id uuid.UUID) error {
	role, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	count, emails, err := s.repo.CountAdminsByRoleName(ctx, role.Name)
	if err != nil {
		return err
	}
	if count > 0 {
		return &RoleInUseError{Count: count, Emails: emails}
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.cache.invalidate()
	return nil
}

func (s *RoleService) validate(role *models.CustomRole) error {
	role.Name = strings.ToLower(strings.TrimSpace(role.Name))
	if !nameFormat.MatchString(role.Name) {
		return fmt.Errorf("name must be lowercase letters/digits/underscore, start with a letter, max 50 chars")
	}
	if builtinNames[role.Name] {
		return fmt.Errorf("name %q is a built-in role and cannot be reused", role.Name)
	}
	role.DisplayName = strings.TrimSpace(role.DisplayName)
	if role.DisplayName == "" {
		return fmt.Errorf("display name required")
	}
	for section, level := range role.Permissions {
		if level != "" && level != "none" && level != "read" && level != "write" {
			return fmt.Errorf("invalid level %q for section %q", level, section)
		}
	}
	return nil
}

// LookupCustomRole implements auth.PermResolver. Called by Matrix() for
// any role name that didn't hit the built-in matrix.
func (s *RoleService) LookupCustomRole(roleName, section string) auth.Level {
	if builtinNames[roleName] || roleName == "" {
		return auth.LevelNone
	}
	if lvl, ok := s.cache.get(roleName, section); ok {
		return lvl
	}
	level, found, err := s.repo.GetLevel(context.Background(), roleName, section)
	if err != nil {
		// Fail closed on transient DB errors. Don't cache.
		return auth.LevelNone
	}
	var lvl auth.Level
	if !found {
		lvl = auth.LevelNone
	} else {
		lvl = auth.Level(level)
	}
	s.cache.set(roleName, section, lvl)
	return lvl
}

// ---- cache ----

type permCacheEntry struct {
	level     auth.Level
	expiresAt time.Time
}

type permCache struct {
	mu    sync.RWMutex
	ttl   time.Duration
	rows  map[string]permCacheEntry // key = roleName+"\x00"+section
}

func newPermCache(ttl time.Duration) *permCache {
	return &permCache{ttl: ttl, rows: map[string]permCacheEntry{}}
}

func (c *permCache) key(role, section string) string {
	return role + "\x00" + section
}

func (c *permCache) get(role, section string) (auth.Level, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.rows[c.key(role, section)]
	if !ok {
		return auth.LevelNone, false
	}
	if time.Now().After(e.expiresAt) {
		return auth.LevelNone, false
	}
	return e.level, true
}

func (c *permCache) set(role, section string, lvl auth.Level) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows[c.key(role, section)] = permCacheEntry{level: lvl, expiresAt: time.Now().Add(c.ttl)}
}

func (c *permCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows = map[string]permCacheEntry{}
}
