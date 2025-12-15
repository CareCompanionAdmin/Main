package web

import (
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"

	"github.com/google/uuid"
)

var templates *template.Template

// InitTemplates loads all templates
func InitTemplates(templatesDir string) error {
	var err error
	templates, err = template.ParseGlob(filepath.Join(templatesDir, "*.html"))
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

	err := templates.ExecuteTemplate(w, name+".html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
