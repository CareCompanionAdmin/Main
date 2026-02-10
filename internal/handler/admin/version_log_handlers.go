package admin

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"carecompanion/internal/middleware"
)

// CommitEntry represents a single git commit
type CommitEntry struct {
	Hash        string `json:"hash"`
	ShortHash   string `json:"short_hash"`
	Date        string `json:"date"`
	Message     string `json:"message"`
	EntryType   string `json:"entry_type"`
	Title       string `json:"title"`
	Environment string `json:"environment"`
	IsDeployed  bool   `json:"is_deployed"`
}

// UncommittedChange represents a modified or new file not yet committed
type UncommittedChange struct {
	FilePath    string `json:"file_path"`
	Status      string `json:"status"` // "modified", "new", "deleted"
	Description string `json:"description"`
}

// DeploymentRecord represents a production deployment
type DeploymentRecord struct {
	Timestamp string `json:"timestamp"`
	RawLine   string `json:"raw_line"`
}

// VersionLogResponse is the API response for the version log
type VersionLogResponse struct {
	Uncommitted      []UncommittedChange `json:"uncommitted"`
	UncommittedCount int                 `json:"uncommitted_count"`
	Pending          []CommitEntry       `json:"pending"`
	History          []CommitEntry       `json:"history"`
	Deployments      []DeploymentRecord  `json:"deployments"`
	LastDeploy       string              `json:"last_deploy"`
	PendingCount     int                 `json:"pending_count"`
	TotalCommits     int                 `json:"total_commits"`
}

// parseCommitMessage categorizes a commit message by its prefix
func parseCommitMessage(msg string) (entryType, title string) {
	prefixes := map[string]string{
		"feat:":     "Feature",
		"fix:":      "Bug Fix",
		"refactor:": "Refactor",
		"perf:":     "Performance",
		"security:": "Security",
		"docs:":     "Docs",
		"style:":    "Style",
		"test:":     "Test",
		"chore:":    "Chore",
	}

	lower := strings.ToLower(msg)
	for prefix, label := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			title = strings.TrimSpace(msg[len(prefix):])
			return label, title
		}
	}

	return "Update", msg
}

// getGitLog gets commit history from live git (dev) or git-log.txt (prod)
func getGitLog() ([]CommitEntry, error) {
	// Try live git first (works in dev where git is available)
	out, err := exec.Command("git", "log", "--format=%H|%ai|%s").Output()
	if err != nil {
		// Fall back to static git-log.txt (generated at Docker build time)
		out, err = os.ReadFile("git-log.txt")
		if err != nil {
			return nil, fmt.Errorf("no git log available: git command failed and git-log.txt not found")
		}
	}

	return parseGitLogOutput(string(out))
}

// parseGitLogOutput parses "hash|date|message" lines into CommitEntry slices
func parseGitLogOutput(data string) ([]CommitEntry, error) {
	var commits []CommitEntry
	lines := strings.Split(strings.TrimSpace(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 3 {
			continue
		}

		hash := parts[0]
		dateStr := parts[1]
		message := parts[2]

		entryType, title := parseCommitMessage(message)

		commits = append(commits, CommitEntry{
			Hash:      hash,
			ShortHash: hash[:7],
			Date:      dateStr,
			Message:   message,
			EntryType: entryType,
			Title:     title,
		})
	}
	return commits, nil
}

// getDeployments reads deployments.log and extracts completed full deploy timestamps
func getDeployments() ([]DeploymentRecord, error) {
	f, err := os.Open("deployments.log")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open deployments.log: %w", err)
	}
	defer f.Close()

	var deployments []DeploymentRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "Full Deploy") && strings.Contains(line, "COMPLETE") {
			// Extract timestamp: everything before the first " |"
			idx := strings.Index(line, " |")
			if idx > 0 {
				deployments = append(deployments, DeploymentRecord{
					Timestamp: strings.TrimSpace(line[:idx]),
					RawLine:   line,
				})
			}
		}
	}
	return deployments, scanner.Err()
}

// describeFileChange generates a human-readable description of what changed in a file
func describeFileChange(filePath, status string) string {
	ext := filepath.Ext(filePath)

	switch {
	case status == "new" && ext == ".go":
		return describeNewGoFile(filePath)
	case status == "modified" && ext == ".go":
		return describeModifiedGoFile(filePath)
	case status == "new" && ext == ".sql":
		return describeNewSQLFile(filePath)
	case ext == ".html":
		return describeHTMLChange(filePath, status)
	case status == "deleted":
		return "Removed " + readableFileName(filePath)
	default:
		verb := "Updated"
		if status == "new" {
			verb = "New"
		}
		return verb + " " + readableFileName(filePath)
	}
}

// describeNewGoFile reads a new Go file and summarizes its exported types/functions
func describeNewGoFile(filePath string) string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "New Go source file"
	}
	names := extractGoNames(string(content))
	if len(names) == 0 {
		return "New " + readableFileName(filePath)
	}
	if len(names) <= 4 {
		return "Adding " + strings.Join(names, ", ")
	}
	return fmt.Sprintf("Adding %s, %s + %d more", names[0], names[1], len(names)-2)
}

// describeModifiedGoFile runs git diff and summarizes added/removed exports
func describeModifiedGoFile(filePath string) string {
	cmd := exec.Command("git", "diff", "HEAD", "--", filePath)
	out, err := cmd.Output()
	if err != nil {
		return "Updated " + readableFileName(filePath)
	}

	var added, removed []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			if name := extractSingleGoName(line[1:]); name != "" {
				added = append(added, name)
			}
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			if name := extractSingleGoName(line[1:]); name != "" {
				removed = append(removed, name)
			}
		}
	}

	var parts []string
	if len(added) > 0 {
		if len(added) <= 3 {
			parts = append(parts, "Adding "+strings.Join(added, ", "))
		} else {
			parts = append(parts, fmt.Sprintf("Adding %d items", len(added)))
		}
	}
	if len(removed) > 0 {
		if len(removed) <= 3 {
			parts = append(parts, "Removing "+strings.Join(removed, ", "))
		} else {
			parts = append(parts, fmt.Sprintf("Removing %d items", len(removed)))
		}
	}

	if len(parts) > 0 {
		return strings.Join(parts, "; ")
	}
	return "Updated " + readableFileName(filePath)
}

// describeNewSQLFile reads a migration file and summarizes the SQL operations
func describeNewSQLFile(filePath string) string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "New database migration"
	}
	upper := strings.ToUpper(string(content))
	var actions []string
	if strings.Contains(upper, "CREATE TABLE") {
		actions = append(actions, "creates table")
	}
	if strings.Contains(upper, "ALTER TABLE") {
		actions = append(actions, "alters table")
	}
	if strings.Contains(upper, "CREATE INDEX") {
		actions = append(actions, "adds indexes")
	}
	if len(actions) > 0 {
		return "Database migration: " + strings.Join(actions, ", ")
	}
	return "New database migration"
}

// describeHTMLChange describes a template file change
func describeHTMLChange(filePath, status string) string {
	verb := "Updated"
	if status == "new" {
		verb = "New"
	}
	dir := filepath.Dir(filePath)
	name := readableFileName(filePath)
	if strings.Contains(dir, "admin") {
		return verb + " admin " + name + " page"
	}
	if strings.Contains(dir, "partials") {
		return verb + " " + name + " UI component"
	}
	return verb + " " + name + " template"
}

// extractGoNames finds all exported func/type names in Go source
func extractGoNames(content string) []string {
	var names []string
	for _, line := range strings.Split(content, "\n") {
		if name := extractSingleGoName(line); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// extractSingleGoName extracts an exported func or type name from a single line
func extractSingleGoName(line string) string {
	line = strings.TrimSpace(line)

	if strings.HasPrefix(line, "type ") {
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			name := fields[1]
			if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
				return name
			}
		}
		return ""
	}

	if strings.HasPrefix(line, "func ") {
		rest := line[5:]
		// Skip receiver: (r *Type)
		if strings.HasPrefix(rest, "(") {
			closeIdx := strings.Index(rest, ")")
			if closeIdx < 0 {
				return ""
			}
			rest = strings.TrimSpace(rest[closeIdx+1:])
		}
		parenIdx := strings.Index(rest, "(")
		if parenIdx <= 0 {
			return ""
		}
		name := strings.TrimSpace(rest[:parenIdx])
		if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
			return name
		}
	}

	return ""
}

// readableFileName converts a file path to a readable name
func readableFileName(filePath string) string {
	base := filepath.Base(filePath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	readable := strings.ReplaceAll(name, "_", " ")
	words := strings.Fields(readable)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// getUncommittedChanges runs git status --porcelain and parses the output
func getUncommittedChanges() ([]UncommittedChange, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}

	var changes []UncommittedChange
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		indicator := line[:2]
		filePath := strings.TrimSpace(line[3:])

		// Skip binary build artifacts
		if filePath == "server" || strings.HasSuffix(filePath, ".exe") {
			continue
		}

		var status string
		switch {
		case strings.Contains(indicator, "D"):
			status = "deleted"
		case strings.Contains(indicator, "?"):
			status = "new"
		default:
			status = "modified"
		}

		changes = append(changes, UncommittedChange{
			FilePath:    filePath,
			Status:      status,
			Description: describeFileChange(filePath, status),
		})
	}
	return changes, nil
}

// GetVersionLog handles GET /api/admin/super/version-log
func (h *Handler) GetVersionLog(w http.ResponseWriter, r *http.Request) {
	commits, err := getGitLog()
	if err != nil {
		http.Error(w, "Failed to read git log: "+err.Error(), http.StatusInternalServerError)
		return
	}

	deployments, err := getDeployments()
	if err != nil {
		http.Error(w, "Failed to read deployments: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Determine last deploy time
	var lastDeployTime time.Time
	var lastDeployStr string
	if len(deployments) > 0 {
		last := deployments[len(deployments)-1]
		lastDeployStr = last.Timestamp
		// Parse "2026-02-10 07:01:33 UTC"
		parsed, err := time.Parse("2006-01-02 15:04:05 MST", last.Timestamp)
		if err == nil {
			lastDeployTime = parsed
		}
	}

	// Classify commits
	var pending, history []CommitEntry
	for i := range commits {
		c := &commits[i]
		// Parse commit date "2026-02-10 07:01:33 -0500"
		commitTime, err := time.Parse("2006-01-02 15:04:05 -0700", c.Date)
		if err != nil {
			// Try without timezone offset
			commitTime, _ = time.Parse("2006-01-02 15:04:05", c.Date[:19])
		}

		if !lastDeployTime.IsZero() && commitTime.After(lastDeployTime) {
			c.IsDeployed = false
			c.Environment = "dev"
			pending = append(pending, *c)
		} else {
			c.IsDeployed = true
			c.Environment = "production"
		}
		history = append(history, *c)
	}

	// Get uncommitted changes (modified/new files in working tree)
	uncommitted, err := getUncommittedChanges()
	if err != nil {
		uncommitted = nil // non-fatal, just skip
	}

	respondJSON(w, VersionLogResponse{
		Uncommitted:      uncommitted,
		UncommittedCount: len(uncommitted),
		Pending:          pending,
		History:          history,
		Deployments:      deployments,
		LastDeploy:       lastDeployStr,
		PendingCount:     len(pending) + len(uncommitted),
		TotalCommits:     len(commits),
	})
}

// VersionLogPage renders the version log UI page
func (h *Handler) VersionLogPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	tmpl, err := parseTemplates("layout.html", "version_log.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Version Log",
		CurrentUser: currentUser,
	})
}
