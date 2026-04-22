package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"carecompanion/internal/config"
	"carecompanion/internal/database"
	"carecompanion/internal/handler/admin"
	"carecompanion/internal/handler/api"
	"carecompanion/internal/handler/web"
	"carecompanion/internal/middleware"
	"carecompanion/internal/repository"
	"carecompanion/internal/service"
)

const transferDir = "transfers"

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to database
	db, err := database.New(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Println("Connected to PostgreSQL")

	// Connect to Redis
	redis, err := database.NewRedis(&cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redis.Close()
	log.Println("Connected to Redis")

	// Initialize repositories
	repos := repository.NewRepositories(db.DB)

	// Initialize services
	services := service.NewServices(repos, redis, cfg, db.DB)

	// Initialize handlers
	apiHandlers := api.NewHandlers(services, cfg)
	webHandlers := web.NewWebHandlers(services)

	// Initialize templates (optional, will use fallback if not available)
	if err := web.InitTemplates("templates"); err != nil {
		log.Printf("Warning: Templates not loaded: %v", err)
	}

	// Create transfers directory for file transfer utility
	os.MkdirAll(transferDir, 0755)

	// Create router
	r := chi.NewRouter()

	// Initialize error tracker
	errorTracker := middleware.NewErrorTracker(db.DB)

	// Global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(errorTracker.Middleware) // Track errors and response times
	r.Use(middleware.LoggingMiddleware)
	r.Use(middleware.RecoverMiddleware)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CORSMiddleware(nil))
	r.Use(chimiddleware.Compress(5))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Maintenance status endpoint (no auth required, used by public pages)
	r.Get("/api/maintenance-status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		active := false
		message := ""
		val, err := repos.Admin.GetSetting(r.Context(), "maintenance_mode")
		if err == nil && val != nil {
			if boolVal, ok := val.(bool); ok {
				active = boolVal
			}
		}
		if active {
			msgVal, err := repos.Admin.GetSetting(r.Context(), "maintenance_message")
			if err == nil && msgVal != nil {
				if strVal, ok := msgVal.(string); ok {
					message = strVal
				}
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"active":  active,
			"message": message,
		})
	})

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.ContentTypeJSON)
		api.SetupRoutes(r, apiHandlers, services.Auth)
	})

	// Web routes
	web.SetupRoutes(r, webHandlers, services.Auth)

	// Admin portal routes
	adminHandler := admin.NewHandler(repos.Admin, services.Auth)

	// Initialize CloudWatch service for system metrics (production only)
	if cfg.App.Env == "production" {
		cwService, err := service.NewCloudWatchService(
			"carecompanion-asg",                                         // ASG name
			"carecompanion-db",                                          // RDS instance identifier
			"us-east-1",                                                 // Region
		)
		if err != nil {
			log.Printf("Warning: Failed to initialize CloudWatch service: %v", err)
		} else {
			// Configure ALB for target health monitoring (full ARNs required for ELB API)
			cwService.SetALBConfig(
				"app/carecompanion-alb/ec4daecf3b14c818",                                                                        // ALB suffix for CloudWatch metrics
				"arn:aws:elasticloadbalancing:us-east-1:943431294725:targetgroup/carecompanion-tg/bade3e56ae036ce7",             // Full Target group ARN for ELB API
			)
			adminHandler.SetCloudWatchService(cwService)
			log.Println("CloudWatch service initialized for metrics collection")
		}
	}

	// Initialize Marketing service for material generation
	marketingService := service.NewMarketingService(repos.Marketing, "static/marketing")
	adminHandler.SetMarketingService(marketingService)
	log.Println("Marketing service initialized")

	// Wire push notifications into admin handlers
	adminHandler.SetPushService(services.Push)

	// Initialize Development Mode service for SSH access control
	// In production, devServerURL is set so session ops call the dev server remotely.
	// On the dev server, devServerURL is empty so ops run locally.
	internalToken := "b8d5931b7ad0a11d82b85b3b1b91e301"
	devServerURL := ""
	if cfg.App.Env == "production" {
		devServerURL = "http://10.0.1.129:8090" // Dev server private IP (same VPC)
	}
	devModeService := service.NewDevModeService(
		repos.DevMode,
		"sg-0a4d8f146c6b6de24", // Dev server Security Group (carecompanion-dev-sg)
		"us-east-1",            // AWS Region
		"",                     // PEM key path - empty means use SSH_PRIVATE_KEY env var
		devServerURL,
		internalToken,
	)
	adminHandler.SetDevModeService(devModeService)
	log.Println("Development Mode service initialized")

	// Internal endpoints for cross-server dev mode session management
	// These are called by the production server to list/kill sessions on this dev server
	r.Get("/internal/dev-sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Token") != internalToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		sessions, err := devModeService.ListSSHSessions(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sessions)
	})
	r.Post("/internal/dev-kill-session", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Token") != internalToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		tty := r.FormValue("tty")
		if tty == "" {
			http.Error(w, "tty required", http.StatusBadRequest)
			return
		}
		if err := devModeService.KillSession(r.Context(), tty); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	r.Route("/api/admin", func(r chi.Router) {
		r.Use(middleware.ContentTypeJSON)
		r.Mount("/", adminHandler.Routes())
	})
	r.Mount("/admin", adminHandler.UIRoutes())

	// File transfer utility (keep for development)
	r.Get("/filextfer", handleFileTransfer)
	r.Post("/filextfer/upload", handleUpload)
	r.Post("/filextfer/save", handleSaveText)
	r.Get("/filextfer/download/*", handleDownload)
	r.Get("/filextfer/delete/*", handleDelete)
	r.Get("/filextfer/view/*", handleView)

	// Static files
	fileServer := http.FileServer(http.Dir("static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	// 404 handler
	r.NotFound(middleware.NotFoundHandler())

	// Create server
	addr := fmt.Sprintf("%s:%s", cfg.App.Host, cfg.App.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Starting MyCareCompanion server on %s", addr)
		log.Printf("Environment: %s", cfg.App.Env)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Start background services
	schedulerCtx, schedulerCancel := context.WithCancel(context.Background())
	reportScheduler := service.NewReportScheduler(services.Report)
	go reportScheduler.Start(schedulerCtx)

	// Create AI insight service if Claude is configured
	var aiInsightService *service.AIInsightService
	if cfg.Claude.Enabled && cfg.Claude.APIKey != "" {
		aiInsightService = service.NewAIInsightService(
			&cfg.Claude, repos.Log, repos.Child, repos.Medication, repos.Insight, services.Alert,
		)
		log.Println("Claude AI insights enabled")
	} else {
		log.Println("Claude AI insights disabled (set CLAUDE_ENABLED=true and CLAUDE_API_KEY to enable)")
	}

	insightGen := service.NewInsightGenerator(services.Alert, repos.Log, repos.Medication, repos.Alert, db.DB, aiInsightService, cfg.Claude.DailyRunHour)
	go insightGen.Start(schedulerCtx)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	schedulerCancel()
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}

// File transfer utility handlers (kept for development convenience)
func handleFileTransfer(w http.ResponseWriter, r *http.Request) {
	files, _ := os.ReadDir(transferDir)

	var fileList strings.Builder
	for _, f := range files {
		if !f.IsDir() {
			info, _ := f.Info()
			size := info.Size()
			sizeStr := fmt.Sprintf("%d B", size)
			if size > 1024 {
				sizeStr = fmt.Sprintf("%.1f KB", float64(size)/1024)
			}
			fileList.WriteString(fmt.Sprintf(`
				<tr>
					<td style="padding:5px;border:1px solid #ccc;">%s</td>
					<td style="padding:5px;border:1px solid #ccc;">%s</td>
					<td style="padding:5px;border:1px solid #ccc;">
						<a href="/filextfer/download/%s">Download</a> |
						<a href="/filextfer/view/%s">View</a> |
						<a href="/filextfer/delete/%s" onclick="return confirm('Delete?')">Delete</a>
					</td>
				</tr>`, f.Name(), sizeStr, f.Name(), f.Name(), f.Name()))
		}
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>File Transfer</title></head>
<body style="font-family:monospace;max-width:900px;margin:20px auto;padding:0 20px;">
<h2>File Transfer Utility</h2>

<h3>Upload File</h3>
<form action="/filextfer/upload" method="post" enctype="multipart/form-data">
	<input type="file" name="file" required>
	<button type="submit">Upload</button>
</form>

<h3>Save Text as File</h3>
<form action="/filextfer/save" method="post">
	<input type="text" name="filename" placeholder="filename.md" required style="width:200px;">
	<br><br>
	<textarea name="content" rows="10" style="width:100%%;font-family:monospace;" placeholder="Paste content here..."></textarea>
	<br>
	<button type="submit">Save</button>
</form>

<h3>Files (%d)</h3>
<table style="border-collapse:collapse;width:100%%;">
	<tr style="background:#eee;">
		<th style="padding:5px;border:1px solid #ccc;text-align:left;">Name</th>
		<th style="padding:5px;border:1px solid #ccc;text-align:left;">Size</th>
		<th style="padding:5px;border:1px solid #ccc;text-align:left;">Actions</th>
	</tr>
	%s
</table>

<p><a href="/">Back to Home</a></p>
</body>
</html>`, len(files), fileList.String())
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20)
	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error reading file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	filename := filepath.Base(handler.Filename)
	dst, err := os.Create(filepath.Join(transferDir, filename))
	if err != nil {
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	io.Copy(dst, file)
	http.Redirect(w, r, "/filextfer", http.StatusSeeOther)
}

func handleSaveText(w http.ResponseWriter, r *http.Request) {
	filename := filepath.Base(r.FormValue("filename"))
	content := r.FormValue("content")

	if filename == "" {
		http.Error(w, "Filename required", http.StatusBadRequest)
		return
	}

	err := os.WriteFile(filepath.Join(transferDir, filename), []byte(content), 0644)
	if err != nil {
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/filextfer", http.StatusSeeOther)
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	filename := filepath.Base(strings.TrimPrefix(r.URL.Path, "/filextfer/download/"))
	fp := filepath.Join(transferDir, filename)

	if _, err := os.Stat(fp); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	http.ServeFile(w, r, fp)
}

func handleView(w http.ResponseWriter, r *http.Request) {
	filename := filepath.Base(strings.TrimPrefix(r.URL.Path, "/filextfer/view/"))
	fp := filepath.Join(transferDir, filename)

	content, err := os.ReadFile(fp)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>%s</title></head>
<body style="font-family:monospace;max-width:900px;margin:20px auto;padding:0 20px;">
<h2>%s</h2>
<p><a href="/filextfer">Back</a> | <a href="/filextfer/download/%s">Download</a></p>
<hr>
<pre style="background:#f5f5f5;padding:15px;overflow-x:auto;white-space:pre-wrap;">%s</pre>
</body>
</html>`, filename, filename, filename, strings.ReplaceAll(string(content), "<", "&lt;"))
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	filename := filepath.Base(strings.TrimPrefix(r.URL.Path, "/filextfer/delete/"))
	fp := filepath.Join(transferDir, filename)

	os.Remove(fp)
	http.Redirect(w, r, "/filextfer", http.StatusSeeOther)
}
