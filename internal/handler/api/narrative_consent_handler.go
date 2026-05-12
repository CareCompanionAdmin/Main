package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"carecompanion/internal/middleware"
	"carecompanion/internal/service"
)

// clientIP returns the client IP without the port, in a form the inet
// column type accepts. Prefers X-Forwarded-For (first hop) when present.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if comma := strings.Index(xff, ","); comma >= 0 {
			xff = xff[:comma]
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// NarrativeConsentHandler exposes the Phase 3 opt-in narrative-analysis
// consent state to the parent-facing settings page. The actual gate
// behavior lives in service.AINarrativeConsentService — this handler
// just wraps Get/Set for the UI.
type NarrativeConsentHandler struct {
	svc *service.AINarrativeConsentService
}

func NewNarrativeConsentHandler(svc *service.AINarrativeConsentService) *NarrativeConsentHandler {
	return &NarrativeConsentHandler{svc: svc}
}

type narrativeConsentResponse struct {
	FeatureAvailable bool   `json:"feature_available"`
	Enabled          bool   `json:"enabled"`
	NeedsReConsent   bool   `json:"needs_re_consent"`
	Version          int    `json:"version"`
	ConsentedAt      string `json:"consented_at,omitempty"`
	DisclosureText   string `json:"disclosure_text"`
	DisclosureSHA    string `json:"disclosure_sha"`
	CurrentVersion   int    `json:"current_version"`
}

// Get returns the current consent state for the authenticated user.
// The disclosure text + version are included so the UI can render the
// exact text the user is being asked to agree to without an extra
// round-trip.
func (h *NarrativeConsentHandler) Get(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		http.Error(w, "narrative consent service unavailable", http.StatusServiceUnavailable)
		return
	}
	userID := middleware.GetUserID(r.Context())
	c, err := h.svc.GetConsent(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to load consent: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := narrativeConsentResponse{
		FeatureAvailable: c.FeatureAvailable,
		Enabled:          c.Enabled,
		NeedsReConsent:   c.NeedsReConsent,
		Version:          c.Version,
		DisclosureText:   service.CurrentNarrativeDisclosureText,
		DisclosureSHA:    service.CurrentNarrativeDisclosureSHA(),
		CurrentVersion:   service.CurrentNarrativeDisclosureVersion,
	}
	if !c.ConsentedAt.IsZero() {
		resp.ConsentedAt = c.ConsentedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

type narrativeConsentRequest struct {
	Enabled            bool   `json:"enabled"`
	AcknowledgedSHA    string `json:"acknowledged_sha"`
}

// Put updates the user's consent state. When enabling, the client must
// echo back the disclosure SHA they just saw — that prevents an old
// cached UI from setting consent against a previous disclosure version.
func (h *NarrativeConsentHandler) Put(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		http.Error(w, "narrative consent service unavailable", http.StatusServiceUnavailable)
		return
	}
	var req narrativeConsentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Enabled {
		if !h.svc.FeatureAvailable() {
			http.Error(w, "AI narrative analysis is not available on this server", http.StatusServiceUnavailable)
			return
		}
		if req.AcknowledgedSHA != service.CurrentNarrativeDisclosureSHA() {
			http.Error(w, "disclosure has changed; please reload Settings and re-read before enabling", http.StatusConflict)
			return
		}
	}

	userID := middleware.GetUserID(r.Context())
	ip := clientIP(r)
	ua := r.UserAgent()

	if err := h.svc.SetConsent(r.Context(), userID, req.Enabled, ip, ua); err != nil {
		http.Error(w, "failed to update consent: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Echo the new state back so the UI doesn't need to refetch.
	c, _ := h.svc.GetConsent(r.Context(), userID)
	resp := narrativeConsentResponse{
		FeatureAvailable: c.FeatureAvailable,
		Enabled:          c.Enabled,
		NeedsReConsent:   c.NeedsReConsent,
		Version:          c.Version,
		DisclosureText:   service.CurrentNarrativeDisclosureText,
		DisclosureSHA:    service.CurrentNarrativeDisclosureSHA(),
		CurrentVersion:   service.CurrentNarrativeDisclosureVersion,
	}
	if !c.ConsentedAt.IsZero() {
		resp.ConsentedAt = c.ConsentedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
