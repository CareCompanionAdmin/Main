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

// SendEmail sends an email with the given parameters
func (s *EmailService) SendEmail(to, subject, htmlBody string) error {
	if !s.cfg.Enabled {
		log.Printf("[EMAIL] Skipping email to %s (SMTP disabled): %s", to, subject)
		return nil
	}

	from := s.cfg.FromAddress
	fromName := s.cfg.FromName

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

	// Connect to the SMTP server
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}

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
	subject := "Welcome to CareCompanion!"
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
	subject := fmt.Sprintf("You've been invited to join %s on CareCompanion", familyName)
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
	subject := "CareCompanion - Reset Your Password"
	body, err := renderTemplate(passwordResetTemplate, map[string]string{
		"FirstName": firstName,
		"ResetURL":  resetURL,
	})
	if err != nil {
		return fmt.Errorf("failed to render password reset email: %w", err)
	}
	return s.SendEmail(to, subject, body)
}

// SendFamilyMemberAddedEmail notifies a user they've been added to a family
func (s *EmailService) SendFamilyMemberAddedEmail(to, firstName, familyName, role, appURL string) error {
	subject := fmt.Sprintf("You've been added to %s on CareCompanion", familyName)
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
    <h1>CareCompanion</h1>
  </div>
  <div class="content">
    %s
  </div>
  <div class="footer">
    <p>&copy; 2026 CareCompanion. All rights reserved.</p>
    <p>This email was sent from notifications@mycarecompanion.net</p>
  </div>
</div>
</div>
</body>
</html>`

var welcomeEmailTemplate = fmt.Sprintf(emailWrapper, `
    <h2>Welcome, {{.FirstName}}!</h2>
    <p>Thank you for joining CareCompanion. We're here to help you track and manage care for your family.</p>
    <p>With CareCompanion, you can:</p>
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
    <p><strong>{{.InviterName}}</strong> has invited you to join the <strong>{{.FamilyName}}</strong> family on CareCompanion.</p>
    <p>CareCompanion helps families track and manage care for children with autism, including behaviors, medications, therapy, and more.</p>
    <p>To accept this invitation, create your account:</p>
    <p><a href="{{.AppURL}}/register" class="btn" style="color: #ffffff;">Create Your Account</a></p>
    <p>Once you register with this email address, you'll automatically be added to the family.</p>
    <p><small>This invitation expires in 7 days.</small></p>
`)

var passwordResetTemplate = fmt.Sprintf(emailWrapper, `
    <h2>Reset Your Password</h2>
    <p>Hi {{.FirstName}},</p>
    <p>We received a request to reset your CareCompanion password. Click the button below to set a new password:</p>
    <p><a href="{{.ResetURL}}" class="btn" style="color: #ffffff;">Reset Password</a></p>
    <p>This link will expire in 1 hour for security.</p>
    <p>If you didn't request this, you can safely ignore this email. Your password won't be changed.</p>
`)

var memberAddedTemplate = fmt.Sprintf(emailWrapper, `
    <h2>You've Been Added to a Family</h2>
    <p>Hi {{.FirstName}},</p>
    <p>You've been added to the <strong>{{.FamilyName}}</strong> family on CareCompanion as a <strong>{{.Role}}</strong>.</p>
    <p>You can now access this family's data, including children's profiles, logs, medications, and more.</p>
    <p><a href="{{.AppURL}}" class="btn" style="color: #ffffff;">Open CareCompanion</a></p>
`)
