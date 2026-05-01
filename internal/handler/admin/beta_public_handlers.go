package admin

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"carecompanion/internal/repository"
)

// BetaOnboardPage renders the public no-auth onboarding form for an
// invitation token. Mounted at GET /beta/onboard/{token} from main.go.
//
// The page is intentionally NOT under /admin and skips auth middleware —
// invited users are not (yet) admins. The token is the only access control;
// it is a UUID v4, generated server-side, single-use-ish (we accept resubmits
// since users may correct typos in their Apple ID).
func (h *Handler) BetaOnboardPage(w http.ResponseWriter, r *http.Request) {
	if h.betaService == nil {
		http.Error(w, "Beta program is not currently available.", http.StatusServiceUnavailable)
		return
	}

	tokenStr := chi.URLParam(r, "token")
	token, err := uuid.Parse(tokenStr)
	if err != nil {
		http.Error(w, "This link is not valid. Please contact whoever invited you.", http.StatusBadRequest)
		return
	}

	// Look up the invitation so we can show a friendly message for bad tokens
	// or for users who already finished onboarding.
	inv, lookupErr := h.lookupInvitation(r, token)
	if lookupErr != nil || inv == nil {
		http.Error(w, "This link isn't valid or has expired. Please contact whoever invited you.", http.StatusNotFound)
		return
	}

	tmpl, err := parsePublicTemplate("beta_onboard.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	flash := ""
	if r.URL.Query().Get("submitted") == "1" || inv.Status == "added_to_testflight" {
		flash = "All set. Apple will email you a TestFlight invitation shortly — open it on your iPhone, install TestFlight if needed, then accept the invite."
	} else if inv.Status == "apple_id_collected" || inv.Status == "error" {
		flash = "We have your Apple ID on file. If you don't see Apple's TestFlight email within a few minutes, check your spam folder or contact whoever invited you."
	}

	_ = tmpl.ExecuteTemplate(w, "beta_onboard.html", map[string]interface{}{
		"Token":  token.String(),
		"Flash":  flash,
		"PDFURL": h.betaService.PDFURL(),
		"Done":   inv.Status == "added_to_testflight",
	})
}

// lookupInvitation is a tiny indirection so the GET handler doesn't import
// repository types directly.
func (h *Handler) lookupInvitation(r *http.Request, token uuid.UUID) (*repository.BetaInvitation, error) {
	return h.betaService.GetByToken(r.Context(), token)
}

// BetaOnboardSubmit accepts the form POST. Mounted at POST /beta/onboard/{token}.
func (h *Handler) BetaOnboardSubmit(w http.ResponseWriter, r *http.Request) {
	if h.betaService == nil {
		http.Error(w, "Beta program is not currently available.", http.StatusServiceUnavailable)
		return
	}
	tokenStr := chi.URLParam(r, "token")
	token, err := uuid.Parse(tokenStr)
	if err != nil {
		http.Error(w, "This link is not valid.", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad form: "+err.Error(), http.StatusBadRequest)
		return
	}
	appleID := r.PostFormValue("apple_id")
	firstName := r.PostFormValue("first_name")
	lastName := r.PostFormValue("last_name")

	if _, err := h.betaService.CollectAppleID(r.Context(), token, appleID, firstName, lastName); err != nil {
		// Render the form again with an error message rather than 500 — the
		// most likely failure here is "Apple ID already in TestFlight" or a
		// validation error, both user-recoverable.
		tmpl, terr := parsePublicTemplate("beta_onboard.html")
		if terr != nil {
			http.Error(w, terr.Error(), http.StatusInternalServerError)
			return
		}
		_ = tmpl.ExecuteTemplate(w, "beta_onboard.html", map[string]interface{}{
			"Token":    token.String(),
			"Flash":    "We hit a snag: " + err.Error() + ". Please double-check the values and try again.",
			"PDFURL":   h.betaService.PDFURL(),
			"AppleID":  appleID,
			"First":    firstName,
			"Last":     lastName,
		})
		return
	}

	// Redirect (PRG) to avoid a re-submit on refresh.
	http.Redirect(w, r, "/beta/onboard/"+tokenStr+"?submitted=1", http.StatusSeeOther)
}
