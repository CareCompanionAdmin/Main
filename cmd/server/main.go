package main

import (
	"context"
	"database/sql"
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
	"carecompanion/internal/migrate"
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

	// Optional: a separate connection pool for the support-ticket tables.
	// When SUPPORT_DB_DSN is set, the admin/user-support/ticket-attachment
	// repos read+write tickets through this pool while still using the main
	// pool for everything else (users lookup, audit log, etc.). This is how
	// dev shares prod's ticket DB. When unset (the prod default), the
	// support pool is the same handle as the main pool — no behavior change.
	supportDB := db.DB
	if cfg.Database.SupportDSN != "" {
		s, err := database.NewWithDSN(
			cfg.Database.SupportDSN,
			cfg.Database.MaxOpenConns,
			cfg.Database.MaxIdleConns,
			cfg.Database.ConnMaxLifetime,
		)
		if err != nil {
			log.Fatalf("Failed to connect to support database: %v", err)
		}
		defer s.Close()
		supportDB = s.DB
		log.Println("Connected to separate support PostgreSQL (SUPPORT_DB_DSN set)")
	}

	// Optional cross-env sessions pool (read-only display) for the Live Sessions
	// admin page. A misconfigured DSN logs a warning and is skipped — must NOT
	// prevent the local server from starting.
	var sessionsProdDB *sql.DB
	if cfg.Database.SessionsProdDSN != "" {
		s, err := database.NewWithDSN(
			cfg.Database.SessionsProdDSN,
			cfg.Database.MaxOpenConns,
			cfg.Database.MaxIdleConns,
			cfg.Database.ConnMaxLifetime,
		)
		if err != nil {
			log.Printf("[SESSIONS] cross-env pool init failed (%v) — continuing without it", err)
		} else {
			defer s.Close()
			sessionsProdDB = s.DB
			log.Println("Connected to cross-env sessions pool (SESSIONS_PROD_DB_DSN set)")
		}
	}

	// Optional admin-mirror pool for bidirectional admin_users replication.
	// When set, every admin user CRUD dual-writes to both the local DB and
	// this mirror. Same fail-soft behavior as SESSIONS_PROD_DB_DSN — if the
	// mirror DSN is misconfigured we log and continue local-only; admin CRUD
	// then warns the operator that replication is offline. Boot does NOT fail.
	var adminMirrorDB *sql.DB
	if cfg.Database.AdminMirrorDSN != "" {
		s, err := database.NewWithDSN(
			cfg.Database.AdminMirrorDSN,
			cfg.Database.MaxOpenConns,
			cfg.Database.MaxIdleConns,
			cfg.Database.ConnMaxLifetime,
		)
		if err != nil {
			log.Printf("[ADMIN-MIRROR] pool init failed (%v) — continuing without replication", err)
		} else {
			defer s.Close()
			adminMirrorDB = s.DB
			log.Println("Connected to admin-mirror pool (ADMIN_MIRROR_DB_DSN set) — bidirectional admin replication ON")
		}
	}

	// Apply any pending DB migrations before anything else touches the schema.
	// Fatal on failure: better to fail fast than serve traffic against a
	// half-migrated DB. ASG keeps the previous instance up if the new one
	// dies during boot, so a bad migration self-rolls-back the deploy.
	migCtx, migCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	if err := migrate.Run(migCtx, db.DB, "migrations"); err != nil {
		migCancel()
		log.Fatalf("Failed to apply migrations: %v", err)
	}
	migCancel()

	// Connect to Redis
	redis, err := database.NewRedis(&cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redis.Close()
	log.Println("Connected to Redis")

	// Initialize repositories
	repos := repository.NewRepositories(db.DB, supportDB, sessionsProdDB, adminMirrorDB)

	// One-shot bidirectional reconciliation of admin_users between local and
	// mirror — runs once per boot when ADMIN_MIRROR_DB_DSN is set. Catches any
	// drift accumulated while the mirror was offline (or pre-existing rows on
	// either side that never went through the dual-write path). Conflicts on
	// same-email/different-id rows resolve last-writer-wins by updated_at;
	// the wrapper logs each conflict for an operator to review.
	if syncer, ok := repos.Admin.(*repository.ReplicatingAdminRepo); ok {
		syncCtx, syncCancel := context.WithTimeout(context.Background(), 30*time.Second)
		if pushed, pulled, conflicts, err := syncer.SyncAdminUsers(syncCtx); err != nil {
			log.Printf("[ADMIN-MIRROR] initial sync failed (continuing without reconciliation): %v", err)
		} else {
			log.Printf("[ADMIN-MIRROR] initial sync OK: pushed=%d pulled=%d conflicts=%d", pushed, pulled, conflicts)
		}
		syncCancel()
	}

	// Initialize services
	services := service.NewServices(repos, redis, cfg, db.DB)

	// Initialize handlers
	apiHandlers := api.NewHandlers(services, cfg)
	webHandlers := web.NewWebHandlers(services, cfg.App.Env)

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

	// Dev-gate: in non-prod environments, fronts the app with a one-time
	// passphrase so casual visitors who find the dev URL can't reach the
	// app. Native Capacitor shell bypasses via its User-Agent marker; users
	// who entered the code keep a 30-day cookie. No-op in production.
	r.Use(middleware.DevGateMiddleware(
		cfg.App.Env,
		os.Getenv("DEV_GATE_CODE"),
		os.Getenv("DEV_GATE_APP_UA_MARKER"),
	))

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
		api.SetupRoutes(r, apiHandlers, services.Auth, db.DB)
	})

	// Public report PDF — signed URL, no auth. SFSafariViewController and
	// Chrome Custom Tabs don't carry the WKWebView's JWT; the URL itself
	// embeds a short-lived HMAC signature instead.
	r.Get("/r/signed/{reportID}", apiHandlers.Report.ServeSignedPDF)

	// Web routes
	web.SetupRoutes(r, webHandlers, services.Auth, db.DB)

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

	// Wire roadmap service into admin handlers
	adminHandler.SetRoadmapService(services.Roadmap)

	// Wire ticket-duplicate service into admin handlers
	adminHandler.SetTicketDuplicateService(services.TicketDuplicate)

	// Wire ticket-attachment service into admin handlers
	adminHandler.SetTicketAttachmentService(services.TicketAttachment)

	// Wire beta-invitation service into admin handlers
	adminHandler.SetBetaService(services.Beta)

	// Wire bounty-rewards service into admin handlers
	adminHandler.SetBountyService(services.Bounty)

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
	// Wire DevMode into the live-sessions aggregator so SSH rows show up
	// alongside JWT sessions on the Live Sessions admin page. Without this,
	// snap.SSH stays empty.
	services.LiveSessions.SetDevModeService(devModeService)
	adminHandler.SetLiveSessionsService(services.LiveSessions)
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

	// Public admin refresh endpoint. Registered at the TOP LEVEL (not inside
	// the /api/admin Route block) because chi's Mount("/", adminHandler.Routes())
	// inside that block uses its own AuthMiddleware and will intercept any
	// path that lands on the mounted subrouter — even routes registered as
	// siblings before the Mount. Keeping refresh at the top level guarantees
	// it bypasses AuthMiddleware, which is required because refresh must work
	// AFTER the access token has lapsed.
	r.With(middleware.ContentTypeJSON).Post("/api/admin/auth/refresh", adminHandler.AdminRefreshToken)

	r.Route("/api/admin", func(r chi.Router) {
		r.Use(middleware.ContentTypeJSON)
		r.Mount("/", adminHandler.Routes())
	})
	r.Mount("/admin", adminHandler.UIRoutes())

	// Public beta onboarding page (no auth — tokenized URL is the access control)
	r.Get("/beta/onboard/{token}", adminHandler.BetaOnboardPage)
	r.Post("/beta/onboard/{token}", adminHandler.BetaOnboardSubmit)

	// Public bounty/rewards criteria page (no auth — purely informational)
	r.Get("/rewards", adminHandler.RewardsPage)

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

	// Phase 2 internal-AI scanners — all reuse existing repos. Each is
	// independent and skips gracefully if its inputs aren't available.
	autoCorrScanner := service.NewAutoCorrelationScanner(repos.Correlation, repos.Child, services.Alert)
	perMetricScanner := service.NewPerMetricScanner(repos.Correlation, repos.Child, services.Alert)
	clinicalRuleScanner := service.NewClinicalRuleScanner(repos.Medication, repos.Correlation, repos.Child, repos.Insight, services.Alert, services.DrugDatabase)

	insightGen := service.NewInsightGenerator(services.Alert, repos.Log, repos.Medication, repos.Alert, db.DB, aiInsightService, cfg.Claude.DailyRunHour, autoCorrScanner, perMetricScanner, clinicalRuleScanner)
	go insightGen.Start(schedulerCtx)

	// Subscription expiry sweeper — transitions trialing→past_due and
	// past_due→terminated. No-op when the subscription service couldn't
	// initialize (e.g. plan rows missing).
	if services.Subscription != nil {
		subScheduler := service.NewSubscriptionScheduler(services.Subscription)
		go subScheduler.Start(schedulerCtx)
	}

	// Daily revenue snapshot — aggregates yesterday's payments at 01:00 UTC
	// and rebuilds the next-90-days expected_revenue_calendar. Reads from
	// the same payments + family_subscriptions tables already populated by
	// the Stripe webhooks, so it works whether or not Stripe is enabled
	// (just produces zeros until payments start landing).
	revSvc := service.NewRevenueSnapshotService(db.DB)
	revScheduler := service.NewRevenueSnapshotScheduler(revSvc)
	go revScheduler.Start(schedulerCtx)

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
