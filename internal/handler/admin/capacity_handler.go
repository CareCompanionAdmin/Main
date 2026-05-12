package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"carecompanion/internal/middleware"
	"carecompanion/internal/repository"
)

// CapacityPage renders the admin capacity-monitoring page. Phase 4 of the
// AI insights initiative — gives Bryan one place to answer "are we close
// to needing to upgrade infra?"
func (h *Handler) CapacityPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	tmpl, err := parseTemplates("layout.html", "capacity.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Capacity",
		CurrentUser: currentUser,
	})
}

// CapacityResponse is the JSON payload returned to the /admin/capacity page.
type CapacityResponse struct {
	GeneratedAt time.Time                  `json:"generated_at"`
	DB          *repository.CapacityCounts `json:"db"`
	EC2         *capacityWidget            `json:"ec2"`
	RDS         *capacityWidget            `json:"rds"`
	Cache       *capacityWidget            `json:"cache"`
	ASG         *capacityWidget            `json:"asg"`
	LLM         *capacityWidget            `json:"llm"`
}

// capacityWidget is the rendered shape for a single headroom indicator.
// Percent + label + status. Status is one of "ok" / "warn" / "crit" /
// "unknown"; UI picks color from that. Thresholds: ok ≤ 70, warn ≤ 85,
// crit > 85.
type capacityWidget struct {
	Label   string  `json:"label"`
	Value   string  `json:"value"`   // human-readable e.g. "12 of 100"
	Percent float64 `json:"percent"` // 0-100, used for the bar fill
	Status  string  `json:"status"`  // ok | warn | crit | unknown
	Note    string  `json:"note,omitempty"`
}

func statusFromPercent(p float64) string {
	switch {
	case p < 0:
		return "unknown"
	case p <= 70:
		return "ok"
	case p <= 85:
		return "warn"
	default:
		return "crit"
	}
}

// GetCapacity returns combined DB + CloudWatch metrics as JSON. Each block
// is independently best-effort — if CloudWatch is unavailable we still
// return the DB-side numbers so the page renders something useful.
func (h *Handler) GetCapacity(w http.ResponseWriter, r *http.Request) {
	resp := CapacityResponse{GeneratedAt: time.Now()}

	dbCtx, dbCancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer dbCancel()
	if counts, err := h.adminRepo.GetCapacityCounts(dbCtx); err == nil {
		resp.DB = counts
	}

	if h.cloudwatchService != nil {
		cwCtx, cwCancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cwCancel()
		if cw, err := h.cloudwatchService.GetMetrics(cwCtx); err == nil && cw != nil {
			// EC2 CPU. MemoryUtilization is only populated when the CloudWatch
			// agent is installed on the instance — usually not, so we keep it
			// off the panel for now.
			ec2Pct := cw.CPUUtilization
			resp.EC2 = &capacityWidget{
				Label:   "EC2 CPU (avg)",
				Value:   fmtPct(ec2Pct),
				Percent: clampPct(ec2Pct),
				Status:  statusFromPercent(ec2Pct),
			}

			// RDS: surface the worse of CPU% and storage%; that's the resource
			// that'd hit the wall first.
			rdsPct := cw.DBCPUUtilization
			rdsLabel := "RDS CPU (avg)"
			if cw.DBStorageUtilization > rdsPct {
				rdsPct = cw.DBStorageUtilization
				rdsLabel = "RDS storage used"
			}
			resp.RDS = &capacityWidget{
				Label:   rdsLabel,
				Value:   fmtPct(rdsPct),
				Percent: clampPct(rdsPct),
				Status:  statusFromPercent(rdsPct),
				Note:    "Connections: " + itoa(cw.DBConnections),
			}

			// ElastiCache CPU + memory. CloudWatchMetrics.CacheMemoryUsage is
			// raw bytes — we have no % without the instance class, so just
			// surface CPU% here and bytes as the note.
			cachePct := cw.CacheCPUUtilization
			resp.Cache = &capacityWidget{
				Label:   "ElastiCache CPU",
				Value:   fmtPct(cachePct),
				Percent: clampPct(cachePct),
				Status:  statusFromPercent(cachePct),
				Note:    "Memory: " + fmtBytes(cw.CacheMemoryUsage),
			}

			// ASG: headroom is already computed by the service.
			if cw.ASG != nil {
				// scaling_headroom is "% of capacity available before max".
				// Convert to "% used vs max" so the indicator reads
				// consistently with the others (high = bad).
				used := 100 - cw.ASG.ScalingHeadroom
				resp.ASG = &capacityWidget{
					Label:   "ASG capacity used",
					Value:   itoa(cw.ASG.CurrentCapacity) + " of " + itoa(cw.ASG.MaxSize),
					Percent: clampPct(used),
					Status:  statusFromPercent(used),
					Note:    "Status: " + cw.ASG.CapacityStatus,
				}
			}
		}
	}

	// LLM spend placeholder — wired in Phase 5 when Bedrock + spend metrics
	// land. Until then the panel shows "—" + the Phase 5 note so Bryan
	// can see the shape of what's coming.
	resp.LLM = &capacityWidget{
		Label:   "LLM spend (7d)",
		Value:   "—",
		Percent: -1,
		Status:  "unknown",
		Note:    "Wires in Phase 5 (Bedrock + cost metrics).",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func fmtPct(p float64) string {
	if p < 0 {
		return "—"
	}
	// One decimal, no trailing zero. Strconv would be cleaner but the
	// dependency surface is fine.
	whole := int(p)
	frac := int((p - float64(whole)) * 10)
	if frac == 0 {
		return itoa(whole) + "%"
	}
	return itoa(whole) + "." + itoa(frac) + "%"
}

func clampPct(p float64) float64 {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func fmtBytes(b float64) string {
	if b <= 0 {
		return "—"
	}
	const (
		mb = 1024 * 1024
		gb = 1024 * mb
	)
	if b >= gb {
		return itoa(int(b/gb)) + " GB"
	}
	if b >= mb {
		return itoa(int(b/mb)) + " MB"
	}
	return itoa(int(b)) + " B"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	s := string(buf[i:])
	if neg {
		return "-" + s
	}
	return s
}
