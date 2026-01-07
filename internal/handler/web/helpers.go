package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// formatTimeInTZ formats a time in the given timezone
func formatTimeInTZ(t time.Time, tz string, layout string) string {
	if tz == "" {
		tz = "America/New_York"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return t.Format(layout)
	}
	return t.In(loc).Format(layout)
}

// LogWithTime wraps any log entry with a formatted time string
type LogWithTime struct {
	Entry         interface{}
	FormattedTime string
}

// DailyLogsTemplateData is the template data for daily logs page
type DailyLogsTemplateData struct {
	Child          models.Child
	Date           time.Time
	FormattedDate  string
	UserTimezone   string
	Logs           *LogsWithFormattedTimes
	MedicationsDue []models.MedicationDue
}

// LogsWithFormattedTimes contains all log types with formatted times
type LogsWithFormattedTimes struct {
	BehaviorLogs    []BehaviorLogWithTime
	DietLogs        []DietLogWithTime
	SleepLogs       []SleepLogWithTime
	BowelLogs       []BowelLogWithTime
	MedicationLogs  []MedicationLogWithTime
	SpeechLogs      []SpeechLogWithTime
	WeightLogs      []WeightLogWithTime
	SensoryLogs     []SensoryLogWithTime
	SocialLogs      []SocialLogWithTime
	TherapyLogs     []TherapyLogWithTime
	SeizureLogs     []SeizureLogWithTime
	HealthEventLogs []HealthEventLogWithTime
}

// Log wrapper types - embed original + add FormattedTime
type BehaviorLogWithTime struct {
	models.BehaviorLog
	FormattedTime string
}

type DietLogWithTime struct {
	models.DietLog
	FormattedTime string
}

type SleepLogWithTime struct {
	models.SleepLog
	FormattedTime string
}

type BowelLogWithTime struct {
	models.BowelLog
	FormattedTime string
}

type MedicationLogWithTime struct {
	models.MedicationLog
	FormattedTime string
}

type SpeechLogWithTime struct {
	models.SpeechLog
	FormattedTime string
}

type WeightLogWithTime struct {
	models.WeightLog
	FormattedTime string
}

type SensoryLogWithTime struct {
	models.SensoryLog
	FormattedTime string
}

type SocialLogWithTime struct {
	models.SocialLog
	FormattedTime string
}

type TherapyLogWithTime struct {
	models.TherapyLog
	FormattedTime string
}

type SeizureLogWithTime struct {
	models.SeizureLog
	FormattedTime string
}

type HealthEventLogWithTime struct {
	models.HealthEventLog
	FormattedTime string
}

// convertLogsWithTimezone converts a DailyLogPage to LogsWithFormattedTimes
func convertLogsWithTimezone(logs *models.DailyLogPage, tz string) *LogsWithFormattedTimes {
	if logs == nil {
		return &LogsWithFormattedTimes{}
	}

	result := &LogsWithFormattedTimes{}

	for _, log := range logs.BehaviorLogs {
		result.BehaviorLogs = append(result.BehaviorLogs, BehaviorLogWithTime{
			BehaviorLog:   log,
			FormattedTime: formatTimeInTZ(log.CreatedAt, tz, "3:04 PM"),
		})
	}

	for _, log := range logs.DietLogs {
		result.DietLogs = append(result.DietLogs, DietLogWithTime{
			DietLog:       log,
			FormattedTime: formatTimeInTZ(log.CreatedAt, tz, "3:04 PM"),
		})
	}

	for _, log := range logs.SleepLogs {
		result.SleepLogs = append(result.SleepLogs, SleepLogWithTime{
			SleepLog:      log,
			FormattedTime: formatTimeInTZ(log.CreatedAt, tz, "3:04 PM"),
		})
	}

	for _, log := range logs.BowelLogs {
		result.BowelLogs = append(result.BowelLogs, BowelLogWithTime{
			BowelLog:      log,
			FormattedTime: formatTimeInTZ(log.CreatedAt, tz, "3:04 PM"),
		})
	}

	for _, log := range logs.MedicationLogs {
		ft := formatTimeInTZ(log.CreatedAt, tz, "3:04 PM")
		if log.ActualTime.Valid {
			ft = log.ActualTime.String
		}
		result.MedicationLogs = append(result.MedicationLogs, MedicationLogWithTime{
			MedicationLog: log,
			FormattedTime: ft,
		})
	}

	for _, log := range logs.SpeechLogs {
		result.SpeechLogs = append(result.SpeechLogs, SpeechLogWithTime{
			SpeechLog:     log,
			FormattedTime: formatTimeInTZ(log.CreatedAt, tz, "3:04 PM"),
		})
	}

	for _, log := range logs.WeightLogs {
		result.WeightLogs = append(result.WeightLogs, WeightLogWithTime{
			WeightLog:     log,
			FormattedTime: formatTimeInTZ(log.CreatedAt, tz, "3:04 PM"),
		})
	}

	for _, log := range logs.SensoryLogs {
		result.SensoryLogs = append(result.SensoryLogs, SensoryLogWithTime{
			SensoryLog:    log,
			FormattedTime: formatTimeInTZ(log.CreatedAt, tz, "3:04 PM"),
		})
	}

	for _, log := range logs.SocialLogs {
		result.SocialLogs = append(result.SocialLogs, SocialLogWithTime{
			SocialLog:     log,
			FormattedTime: formatTimeInTZ(log.CreatedAt, tz, "3:04 PM"),
		})
	}

	for _, log := range logs.TherapyLogs {
		result.TherapyLogs = append(result.TherapyLogs, TherapyLogWithTime{
			TherapyLog:    log,
			FormattedTime: formatTimeInTZ(log.CreatedAt, tz, "3:04 PM"),
		})
	}

	for _, log := range logs.SeizureLogs {
		result.SeizureLogs = append(result.SeizureLogs, SeizureLogWithTime{
			SeizureLog:    log,
			FormattedTime: formatTimeInTZ(log.CreatedAt, tz, "3:04 PM"),
		})
	}

	for _, log := range logs.HealthEventLogs {
		result.HealthEventLogs = append(result.HealthEventLogs, HealthEventLogWithTime{
			HealthEventLog: log,
			FormattedTime:  formatTimeInTZ(log.CreatedAt, tz, "3:04 PM"),
		})
	}

	return result
}

var templates *template.Template

// Template functions
var templateFuncs = template.FuncMap{
	// toJSON converts a value to JSON for use in JavaScript
	"toJSON": func(v interface{}) template.JS {
		bytes, err := json.Marshal(v)
		if err != nil {
			return template.JS("[]")
		}
		return template.JS(bytes)
	},
	// deref dereferences a pointer and returns the value, or 0 if nil
	"deref": func(ptr interface{}) interface{} {
		if ptr == nil {
			return 0.0
		}
		switch v := ptr.(type) {
		case *float64:
			if v == nil {
				return 0.0
			}
			return *v
		case *int:
			if v == nil {
				return 0
			}
			return *v
		case *string:
			if v == nil {
				return ""
			}
			return *v
		default:
			return ptr
		}
	},
	// mul multiplies two numbers
	"mul": func(a, b interface{}) float64 {
		var af, bf float64
		switch v := a.(type) {
		case float64:
			af = v
		case *float64:
			if v != nil {
				af = *v
			}
		case int:
			af = float64(v)
		}
		switch v := b.(type) {
		case float64:
			bf = v
		case int:
			bf = float64(v)
		}
		return af * bf
	},
	// formatTime formats a time in the given timezone with the specified layout
	// Usage: {{formatTime .CreatedAt $.UserTimezone "3:04 PM"}}
	"formatTime": func(t time.Time, tz string, layout string) string {
		if tz == "" {
			tz = "UTC"
		}
		loc, err := time.LoadLocation(tz)
		if err != nil {
			// Fall back to UTC if timezone is invalid
			return t.UTC().Format(layout)
		}
		return t.In(loc).Format(layout)
	},
	// formatDate formats a date in the given timezone with the specified layout
	// Usage: {{formatDate .LogDate $.UserTimezone "Jan 2, 2006"}}
	"formatDate": func(t time.Time, tz string, layout string) string {
		if tz == "" {
			tz = "UTC"
		}
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return t.UTC().Format(layout)
		}
		return t.In(loc).Format(layout)
	},
}

// InitTemplates loads all templates
func InitTemplates(templatesDir string) error {
	var err error
	templates, err = template.New("").Funcs(templateFuncs).ParseGlob(filepath.Join(templatesDir, "*.html"))
	if err != nil {
		return fmt.Errorf("failed to parse templates: %w", err)
	}

	// Parse partials
	_, err = templates.ParseGlob(filepath.Join(templatesDir, "partials", "*.html"))
	if err != nil {
		// Partials are optional
		return nil
	}

	return nil
}

// renderTemplate renders a template with the given data
func renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if templates == nil {
		// Fallback for when templates aren't loaded
		renderFallback(w, name, data)
		return
	}

	// Buffer the output to catch errors before writing
	var buf bytes.Buffer
	err := templates.ExecuteTemplate(&buf, name+".html", data)
	if err != nil {
		// Log the error and show error page
		fmt.Printf("Template error for %s: %v\n", name, err)
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	buf.WriteTo(w)
}

// renderError renders an error page
func renderError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)

	data := map[string]interface{}{
		"Error":      message,
		"StatusCode": statusCode,
	}

	if templates != nil {
		templates.ExecuteTemplate(w, "error.html", data)
		return
	}

	// Fallback
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Error</title></head>
<body>
<h1>Error %d</h1>
<p>%s</p>
<a href="/">Go Home</a>
</body>
</html>`, statusCode, message)
}

// renderFallback renders a basic HTML response when templates aren't available
func renderFallback(w http.ResponseWriter, name string, data interface{}) {
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>CareCompanion</title>
    <script src="https://unpkg.com/htmx.org@1.9.10"></script>
    <script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="bg-gray-100">
    <div class="container mx-auto p-4">
        <h1 class="text-2xl font-bold mb-4">CareCompanion</h1>
        <p>Template: %s</p>
        <p>Data: %v</p>
        <p class="text-gray-500 mt-4">Templates not yet generated. Run 'templ generate' to create templates.</p>
    </div>
</body>
</html>`, name, data)
}

// parseUUID parses a UUID string
func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

// HTMXRequest checks if the request is an HTMX request
func HTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// HTMXRedirect sends an HX-Redirect header for HTMX requests
func HTMXRedirect(w http.ResponseWriter, url string) {
	w.Header().Set("HX-Redirect", url)
	w.WriteHeader(http.StatusOK)
}
