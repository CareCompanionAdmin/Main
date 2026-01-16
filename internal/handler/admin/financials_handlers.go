package admin

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// GetFinancialOverview returns the financial overview dashboard data
func (h *Handler) GetFinancialOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.adminRepo.GetFinancialOverview(r.Context())
	if err != nil {
		http.Error(w, "Failed to fetch financial overview: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(overview)
}

// GetExpectedRevenueCalendar returns expected revenue for a date range
func (h *Handler) GetExpectedRevenueCalendar(w http.ResponseWriter, r *http.Request) {
	// Parse date range from query params
	startStr := r.URL.Query().Get("start_date")
	endStr := r.URL.Query().Get("end_date")

	var startDate, endDate time.Time
	var err error

	if startStr != "" {
		startDate, err = time.Parse("2006-01-02", startStr)
		if err != nil {
			http.Error(w, "Invalid start_date format (use YYYY-MM-DD)", http.StatusBadRequest)
			return
		}
	} else {
		// Default to start of current month
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}

	if endStr != "" {
		endDate, err = time.Parse("2006-01-02", endStr)
		if err != nil {
			http.Error(w, "Invalid end_date format (use YYYY-MM-DD)", http.StatusBadRequest)
			return
		}
	} else {
		// Default to end of next month
		now := time.Now()
		endDate = time.Date(now.Year(), now.Month()+2, 0, 0, 0, 0, 0, time.UTC)
	}

	calendar, err := h.adminRepo.GetExpectedRevenueCalendar(r.Context(), startDate, endDate)
	if err != nil {
		http.Error(w, "Failed to fetch revenue calendar: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"start_date": startDate.Format("2006-01-02"),
		"end_date":   endDate.Format("2006-01-02"),
		"days":       calendar,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetRecentPayments returns paginated list of recent payments
func (h *Handler) GetRecentPayments(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 25
	}

	payments, total, err := h.adminRepo.GetRecentPayments(r.Context(), page, limit)
	if err != nil {
		http.Error(w, "Failed to fetch payments: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"payments": payments,
		"total":    total,
		"page":     page,
		"limit":    limit,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetRecentSubscriptions returns paginated list of recent subscriptions
func (h *Handler) GetRecentSubscriptions(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 25
	}

	subscriptions, total, err := h.adminRepo.GetRecentSubscriptions(r.Context(), page, limit)
	if err != nil {
		http.Error(w, "Failed to fetch subscriptions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"subscriptions": subscriptions,
		"total":         total,
		"page":          page,
		"limit":         limit,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetSubscriptionPlans returns list of subscription plans
func (h *Handler) GetSubscriptionPlans(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("active_only") == "true"

	plans, err := h.adminRepo.ListSubscriptionPlans(r.Context(), activeOnly)
	if err != nil {
		http.Error(w, "Failed to fetch subscription plans: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plans)
}

// GenerateFinancialReport generates a financial report in the requested format
func (h *Handler) GenerateFinancialReport(w http.ResponseWriter, r *http.Request) {
	reportType := r.URL.Query().Get("type")
	format := r.URL.Query().Get("format")
	startStr := r.URL.Query().Get("start_date")
	endStr := r.URL.Query().Get("end_date")

	if reportType == "" {
		reportType = "revenue"
	}
	if format == "" {
		format = "csv"
	}

	var startDate, endDate time.Time
	var err error

	if startStr != "" {
		startDate, err = time.Parse("2006-01-02", startStr)
		if err != nil {
			http.Error(w, "Invalid start_date format", http.StatusBadRequest)
			return
		}
	} else {
		// Default to start of current year
		now := time.Now()
		startDate = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	}

	if endStr != "" {
		endDate, err = time.Parse("2006-01-02", endStr)
		if err != nil {
			http.Error(w, "Invalid end_date format", http.StatusBadRequest)
			return
		}
	} else {
		endDate = time.Now()
	}

	switch format {
	case "csv":
		h.generateCSVReport(w, r, reportType, startDate, endDate)
	case "json":
		h.generateJSONReport(w, r, reportType, startDate, endDate)
	default:
		http.Error(w, "Unsupported format. Use 'csv' or 'json'", http.StatusBadRequest)
	}
}

func (h *Handler) generateCSVReport(w http.ResponseWriter, r *http.Request, reportType string, startDate, endDate time.Time) {
	filename := fmt.Sprintf("carecompanion_%s_report_%s_to_%s.csv",
		reportType,
		startDate.Format("2006-01-02"),
		endDate.Format("2006-01-02"))

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)

	writer := csv.NewWriter(w)
	defer writer.Flush()

	switch reportType {
	case "revenue":
		// Write header
		writer.Write([]string{"Date", "Revenue (cents)", "New Subscriptions", "Cancellations", "Refunds (cents)", "Discounts (cents)"})

		snapshots, err := h.adminRepo.GetDailyRevenueSnapshots(r.Context(), startDate, endDate)
		if err != nil {
			http.Error(w, "Failed to generate report: "+err.Error(), http.StatusInternalServerError)
			return
		}

		for _, s := range snapshots {
			writer.Write([]string{
				s.SnapshotDate.Format("2006-01-02"),
				strconv.FormatInt(s.TotalRevenueCents, 10),
				strconv.Itoa(s.NewSubscriptions),
				strconv.Itoa(s.CancelledSubscriptions),
				strconv.FormatInt(s.RefundsCents, 10),
				strconv.FormatInt(s.PromoDiscountsCents, 10),
			})
		}

	case "payments":
		writer.Write([]string{"ID", "Date", "User Email", "Amount (cents)", "Status", "Type", "Promo Code", "Discount (cents)"})

		payments, _, err := h.adminRepo.GetRecentPayments(r.Context(), 1, 10000)
		if err != nil {
			http.Error(w, "Failed to generate report: "+err.Error(), http.StatusInternalServerError)
			return
		}

		for _, p := range payments {
			if p.CreatedAt.After(startDate) && p.CreatedAt.Before(endDate.Add(24*time.Hour)) {
				writer.Write([]string{
					p.ID.String(),
					p.CreatedAt.Format("2006-01-02 15:04:05"),
					p.UserEmail,
					strconv.Itoa(p.AmountCents),
					string(p.Status),
					string(p.PaymentType),
					p.PromoCode,
					strconv.Itoa(p.DiscountAmountCents),
				})
			}
		}

	case "subscriptions":
		writer.Write([]string{"ID", "Date", "User Email", "Plan", "Status", "Period Start", "Period End"})

		subs, _, err := h.adminRepo.GetRecentSubscriptions(r.Context(), 1, 10000)
		if err != nil {
			http.Error(w, "Failed to generate report: "+err.Error(), http.StatusInternalServerError)
			return
		}

		for _, s := range subs {
			if s.CreatedAt.After(startDate) && s.CreatedAt.Before(endDate.Add(24*time.Hour)) {
				writer.Write([]string{
					s.ID.String(),
					s.CreatedAt.Format("2006-01-02 15:04:05"),
					s.UserEmail,
					s.PlanName,
					string(s.Status),
					s.CurrentPeriodStart.Format("2006-01-02"),
					s.CurrentPeriodEnd.Format("2006-01-02"),
				})
			}
		}

	default:
		http.Error(w, "Unknown report type", http.StatusBadRequest)
	}
}

func (h *Handler) generateJSONReport(w http.ResponseWriter, r *http.Request, reportType string, startDate, endDate time.Time) {
	w.Header().Set("Content-Type", "application/json")

	report := map[string]interface{}{
		"report_type": reportType,
		"start_date":  startDate.Format("2006-01-02"),
		"end_date":    endDate.Format("2006-01-02"),
		"generated":   time.Now().Format(time.RFC3339),
	}

	switch reportType {
	case "revenue":
		snapshots, err := h.adminRepo.GetDailyRevenueSnapshots(r.Context(), startDate, endDate)
		if err != nil {
			http.Error(w, "Failed to generate report: "+err.Error(), http.StatusInternalServerError)
			return
		}
		report["data"] = snapshots

	case "payments":
		payments, total, err := h.adminRepo.GetRecentPayments(r.Context(), 1, 10000)
		if err != nil {
			http.Error(w, "Failed to generate report: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Filter by date range
		filtered := []interface{}{}
		for _, p := range payments {
			if p.CreatedAt.After(startDate) && p.CreatedAt.Before(endDate.Add(24*time.Hour)) {
				filtered = append(filtered, p)
			}
		}
		report["data"] = filtered
		report["total_records"] = total

	case "subscriptions":
		subs, total, err := h.adminRepo.GetRecentSubscriptions(r.Context(), 1, 10000)
		if err != nil {
			http.Error(w, "Failed to generate report: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Filter by date range
		filtered := []interface{}{}
		for _, s := range subs {
			if s.CreatedAt.After(startDate) && s.CreatedAt.Before(endDate.Add(24*time.Hour)) {
				filtered = append(filtered, s)
			}
		}
		report["data"] = filtered
		report["total_records"] = total

	default:
		http.Error(w, "Unknown report type", http.StatusBadRequest)
		return
	}

	json.NewEncoder(w).Encode(report)
}
