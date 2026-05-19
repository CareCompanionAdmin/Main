package service

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"

	"carecompanion/internal/config"
)

// loginAuth implements smtp.Auth for the LOGIN mechanism (required by Microsoft 365)
type loginAuth struct {
	username, password string
}

func newLoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username, password}
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:", "Username", "username:":
			return []byte(a.username), nil
		case "Password:", "Password", "password:":
			return []byte(a.password), nil
		default:
			return nil, errors.New("unexpected server challenge: " + string(fromServer))
		}
	}
	return nil, nil
}

// EmailService handles sending emails via SMTP
type EmailService struct {
	cfg *config.SMTPConfig
}

// NewEmailService creates a new email service
func NewEmailService(cfg *config.SMTPConfig) *EmailService {
	return &EmailService{cfg: cfg}
}

// IsEnabled returns whether email sending is enabled
func (s *EmailService) IsEnabled() bool {
	return s.cfg.Enabled
}

// sanitizeHeader removes \r and \n characters from email header values
// to prevent header injection attacks via user-controlled input.
func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	return value
}

// SendEmail sends an email with the given parameters
func (s *EmailService) SendEmail(to, subject, htmlBody string) error {
	if !s.cfg.Enabled {
		log.Printf("[EMAIL] Skipping email to %s (SMTP disabled): %s", to, subject)
		return nil
	}

	from := s.cfg.FromAddress
	fromName := sanitizeHeader(s.cfg.FromName)

	// Sanitize header values to prevent injection
	to = sanitizeHeader(to)
	subject = sanitizeHeader(subject)

	// Build the email message
	var msg bytes.Buffer
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", fromName, from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)

	addr := fmt.Sprintf("%s:%s", s.cfg.Host, s.cfg.Port)

	// Connect to the SMTP server with an explicit dial timeout so an
	// unreachable / black-holed SMTP host doesn't block the caller
	// indefinitely. Also apply per-operation deadlines once the
	// connection is established (covers a server that accepts the TCP
	// connection but never responds to commands).
	const smtpDialTimeout = 10 * time.Second
	const smtpOpTimeout = 20 * time.Second
	conn, err := net.DialTimeout("tcp", addr, smtpDialTimeout)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(smtpOpTimeout))

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// Start TLS (required by Microsoft 365)
	tlsConfig := &tls.Config{
		ServerName: s.cfg.Host,
	}
	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("failed to start TLS: %w", err)
	}

	// Authenticate using LOGIN (required by Microsoft 365)
	auth := newLoginAuth(s.cfg.Username, s.cfg.Password)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}

	// Set sender and recipient
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("failed to set recipient: %w", err)
	}

	// Send the body
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to open data writer: %w", err)
	}
	if _, err := w.Write(msg.Bytes()); err != nil {
		return fmt.Errorf("failed to write email body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	client.Quit()

	log.Printf("[EMAIL] Sent to %s: %s", to, subject)
	return nil
}

// --- Email template methods ---

// SendWelcomeEmail sends a welcome email to a newly registered user
func (s *EmailService) SendWelcomeEmail(to, firstName, appURL string) error {
	subject := "Welcome to MyCareCompanion!"
	body, err := renderTemplate(welcomeEmailTemplate, map[string]string{
		"FirstName": firstName,
		"AppURL":    appURL,
	})
	if err != nil {
		return fmt.Errorf("failed to render welcome email: %w", err)
	}
	return s.SendEmail(to, subject, body)
}

// SendFamilyInvitationEmail sends an invitation email to a new user
func (s *EmailService) SendFamilyInvitationEmail(to, inviteeName, familyName, inviterName, appURL string) error {
	subject := fmt.Sprintf("You've been invited to join %s on MyCareCompanion", familyName)
	body, err := renderTemplate(familyInvitationTemplate, map[string]string{
		"InviteeName": inviteeName,
		"FamilyName":  familyName,
		"InviterName": inviterName,
		"AppURL":      appURL,
	})
	if err != nil {
		return fmt.Errorf("failed to render invitation email: %w", err)
	}
	return s.SendEmail(to, subject, body)
}

// SendPasswordResetEmail sends a password reset email
func (s *EmailService) SendPasswordResetEmail(to, firstName, resetURL string) error {
	subject := "MyCareCompanion - Reset Your Password"
	body, err := renderTemplate(passwordResetTemplate, map[string]string{
		"FirstName": firstName,
		"ResetURL":  resetURL,
	})
	if err != nil {
		return fmt.Errorf("failed to render password reset email: %w", err)
	}
	return s.SendEmail(to, subject, body)
}

// SendAccountDeletionCodeEmail sends the 6-digit OTP that begins the
// in-app account-deletion flow. ttlMinutes is the validity window we
// want to communicate to the user (15 by default).
func (s *EmailService) SendAccountDeletionCodeEmail(to, firstName, code string, ttlMinutes int) error {
	subject := "MyCareCompanion — Confirm your account deletion"
	body, err := renderTemplate(accountDeletionCodeTemplate, map[string]string{
		"FirstName":  firstName,
		"Code":       code,
		"TTLMinutes": fmt.Sprintf("%d", ttlMinutes),
	})
	if err != nil {
		return fmt.Errorf("failed to render deletion-code email: %w", err)
	}
	return s.SendEmail(to, subject, body)
}

// SendAccountDeletionStartedEmail confirms that the deletion is now in
// progress and gives the user two next-step links: the one-click undo and
// the data-export consent page.
func (s *EmailService) SendAccountDeletionStartedEmail(to, firstName, restoreURL, exportURL string, scheduledHardDelete time.Time, selfRestoreDays, totalGraceDays int) error {
	subject := "MyCareCompanion — Your account deletion has started"
	body, err := renderTemplate(accountDeletionStartedTemplate, map[string]string{
		"FirstName":              firstName,
		"RestoreURL":             restoreURL,
		"ExportURL":              exportURL,
		"ScheduledHardDeleteAt":  scheduledHardDelete.Format("January 2, 2006"),
		"SelfRestoreWindowDays":  fmt.Sprintf("%d", selfRestoreDays),
		"TotalGraceWindowDays":   fmt.Sprintf("%d", totalGraceDays),
	})
	if err != nil {
		return fmt.Errorf("failed to render deletion-started email: %w", err)
	}
	return s.SendEmail(to, subject, body)
}

// SendAccountRestoredEmail confirms that the soft-delete was reversed
// (either by the user via the email link or by support).
func (s *EmailService) SendAccountRestoredEmail(to, firstName, appURL string) error {
	subject := "MyCareCompanion — Welcome back, your account is restored"
	body, err := renderTemplate(accountRestoredTemplate, map[string]string{
		"FirstName": firstName,
		"AppURL":    appURL,
	})
	if err != nil {
		return fmt.Errorf("failed to render restored email: %w", err)
	}
	return s.SendEmail(to, subject, body)
}

// SendAccountHardDeletedEmail is the final notice — after the 30-day grace,
// the data has been permanently removed from production.
func (s *EmailService) SendAccountHardDeletedEmail(to, firstName string) error {
	subject := "MyCareCompanion — Your account has been permanently deleted"
	body, err := renderTemplate(accountHardDeletedTemplate, map[string]string{
		"FirstName": firstName,
	})
	if err != nil {
		return fmt.Errorf("failed to render hard-deleted email: %w", err)
	}
	return s.SendEmail(to, subject, body)
}

// SendFamilyMemberAddedEmail notifies a user they've been added to a family
func (s *EmailService) SendFamilyMemberAddedEmail(to, firstName, familyName, role, appURL string) error {
	subject := fmt.Sprintf("You've been added to %s on MyCareCompanion", familyName)
	body, err := renderTemplate(memberAddedTemplate, map[string]string{
		"FirstName":  firstName,
		"FamilyName": familyName,
		"Role":       formatRole(role),
		"AppURL":     appURL,
	})
	if err != nil {
		return fmt.Errorf("failed to render member added email: %w", err)
	}
	return s.SendEmail(to, subject, body)
}

func renderTemplate(tmpl string, data map[string]string) (string, error) {
	t, err := template.New("email").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func formatRole(role string) string {
	switch role {
	case "parent":
		return "Parent"
	case "caregiver":
		return "Caregiver"
	case "medical_provider":
		return "Medical Provider"
	default:
		if len(role) > 0 {
			return strings.ToUpper(role[:1]) + role[1:]
		}
		return role
	}
}

// --- Email Templates ---

const emailWrapper = `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 0; background-color: #f4f7fa; }
  .container { max-width: 600px; margin: 0 auto; background: #ffffff; border-radius: 8px; overflow: hidden; }
  .header { background: #2563eb; padding: 24px; text-align: center; }
  .header h1 { color: #ffffff; margin: 0; font-size: 24px; }
  .content { padding: 32px 24px; color: #374151; line-height: 1.6; }
  .content h2 { color: #1f2937; margin-top: 0; }
  .btn { display: inline-block; background: #2563eb; color: #ffffff; text-decoration: none; padding: 12px 24px; border-radius: 6px; font-weight: 600; margin: 16px 0; }
  .footer { padding: 16px 24px; text-align: center; color: #9ca3af; font-size: 12px; border-top: 1px solid #e5e7eb; }
</style>
</head>
<body>
<div style="padding: 20px;">
<div class="container">
  <div class="header">
    <h1>MyCareCompanion</h1>
  </div>
  <div class="content">
    %s
  </div>
  <div class="footer">
    <p>&copy; 2026 MyCareCompanion. All rights reserved.</p>
    <p>This email was sent from notifications@mycarecompanion.net</p>
  </div>
</div>
</div>
</body>
</html>`

var welcomeEmailTemplate = fmt.Sprintf(emailWrapper, `
    <h2>Welcome, {{.FirstName}}!</h2>
    <p>Thank you for joining MyCareCompanion. We're here to help you track and manage care for your family.</p>
    <p>With MyCareCompanion, you can:</p>
    <ul>
      <li>Track behaviors, medications, sleep, diet, and more</li>
      <li>Monitor patterns and receive intelligent alerts</li>
      <li>Collaborate with caregivers and medical providers</li>
      <li>Access insights to improve care decisions</li>
    </ul>
    <p><a href="{{.AppURL}}" class="btn" style="color: #ffffff;">Get Started</a></p>
    <p>If you have any questions, don't hesitate to reach out through our support system.</p>
`)

var familyInvitationTemplate = fmt.Sprintf(emailWrapper, `
    <h2>You're Invited!</h2>
    <p>Hi {{.InviteeName}},</p>
    <p><strong>{{.InviterName}}</strong> has invited you to join the <strong>{{.FamilyName}}</strong> family on MyCareCompanion.</p>
    <p>MyCareCompanion helps families track and manage care for children with autism, including behaviors, medications, therapy, and more.</p>
    <p>To accept this invitation, create your account:</p>
    <p><a href="{{.AppURL}}/register" class="btn" style="color: #ffffff;">Create Your Account</a></p>
    <p>Once you register with this email address, you'll automatically be added to the family.</p>
    <p><small>This invitation expires in 7 days.</small></p>
`)

var passwordResetTemplate = fmt.Sprintf(emailWrapper, `
    <h2>Reset Your Password</h2>
    <p>Hi {{.FirstName}},</p>
    <p>We received a request to reset your MyCareCompanion password. Click the button below to set a new password:</p>
    <p><a href="{{.ResetURL}}" class="btn" style="color: #ffffff;">Reset Password</a></p>
    <p>This link will expire in 1 hour for security.</p>
    <p>If you didn't request this, you can safely ignore this email. Your password won't be changed.</p>
`)

var memberAddedTemplate = fmt.Sprintf(emailWrapper, `
    <h2>You've Been Added to a Family</h2>
    <p>Hi {{.FirstName}},</p>
    <p>You've been added to the <strong>{{.FamilyName}}</strong> family on MyCareCompanion as a <strong>{{.Role}}</strong>.</p>
    <p>You can now access this family's data, including children's profiles, logs, medications, and more.</p>
    <p><a href="{{.AppURL}}" class="btn" style="color: #ffffff;">Open MyCareCompanion</a></p>
`)

// --- Account deletion templates ---

var accountDeletionCodeTemplate = fmt.Sprintf(emailWrapper, `
    <h2>Confirm Your Account Deletion</h2>
    <p>Hi {{.FirstName}},</p>
    <p>You started the process of deleting your MyCareCompanion account. To confirm, enter this code in the app:</p>
    <p style="text-align:center; font-size:2rem; letter-spacing:0.4em; font-family:monospace; padding:1rem; background:#fbf6ee; border-radius:12px;"><strong>{{.Code}}</strong></p>
    <p>This code expires in {{.TTLMinutes}} minutes.</p>
    <p>If you didn't start an account deletion, you can ignore this email — nothing has happened to your account yet. If you keep seeing these messages, please email support@mycarecompanion.net so we can investigate.</p>
`)

var accountDeletionStartedTemplate = fmt.Sprintf(emailWrapper, `
    <h2>Your Account Deletion Has Started</h2>
    <p>Hi {{.FirstName}},</p>
    <p>We've received your confirmation and started the process of deleting your MyCareCompanion account. Your data will be permanently removed on <strong>{{.ScheduledHardDeleteAt}}</strong>.</p>

    <h3>Changed your mind?</h3>
    <p>You have <strong>{{.SelfRestoreWindowDays}} days</strong> to undo this deletion yourself. Click the button below at any time before then:</p>
    <p><a href="{{.RestoreURL}}" class="btn" style="color: #ffffff;">Undo Deletion</a></p>
    <p>After that, you have up to <strong>{{.TotalGraceWindowDays}} days</strong> total to email <a href="mailto:support@mycarecompanion.net">support@mycarecompanion.net</a> and ask us to restore your account.</p>

    <h3>Want a copy of your data?</h3>
    <p>You can request a downloadable copy of your family's care history in CSV, Excel, or SQLite formats. Once you submit your format choices, we'll email you the download link within 24 hours.</p>
    <p><a href="{{.ExportURL}}" class="btn" style="color: #ffffff;">Download My Data</a></p>
    <p style="font-size:0.85rem; color:#78716c;">Once your data leaves our system, you're responsible for storing it securely. Don't share it casually — it contains protected health information about your child.</p>

    <p style="margin-top:2rem; font-size:0.85rem; color:#78716c;">If you didn't initiate this deletion, please reply to this email immediately so we can secure your account.</p>
`)

var accountRestoredTemplate = fmt.Sprintf(emailWrapper, `
    <h2>Welcome back, {{.FirstName}}</h2>
    <p>Your MyCareCompanion account has been restored. Your data is back exactly as it was, and you can sign in normally:</p>
    <p><a href="{{.AppURL}}/login" class="btn" style="color: #ffffff;">Sign In</a></p>
    <p>If you didn't ask to restore your account, please reply to this email — someone else may have your account credentials.</p>
`)

var accountHardDeletedTemplate = fmt.Sprintf(emailWrapper, `
    <h2>Your Account Has Been Permanently Deleted</h2>
    <p>Hi {{.FirstName}},</p>
    <p>The 30-day grace period for your MyCareCompanion account has ended. Your account and all family data have been removed from our active systems.</p>
    <p>If you ever want to use MyCareCompanion again, you're welcome to create a new account at <a href="https://www.mycarecompanion.net">mycarecompanion.net</a>.</p>
    <p>Thanks for being with us.</p>
`)
