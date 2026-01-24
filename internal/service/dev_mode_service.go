package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"

	"github.com/google/uuid"
)

// DevModeService handles development mode operations
type DevModeService struct {
	repo          repository.DevModeRepository
	securityGroup string
	region        string
	pemKeyPath    string
	pemKeyContent string
}

// NewDevModeService creates a new DevModeService
func NewDevModeService(repo repository.DevModeRepository, sgID, region, pemKeyPath string) *DevModeService {
	svc := &DevModeService{
		repo:          repo,
		securityGroup: sgID,
		region:        region,
		pemKeyPath:    pemKeyPath,
	}

	// Try to load PEM key content from file or environment
	if pemKeyPath != "" {
		if content, err := os.ReadFile(pemKeyPath); err == nil {
			svc.pemKeyContent = string(content)
			log.Printf("Loaded PEM key from file: %s", pemKeyPath)
		}
	}

	// Check environment variable as fallback
	if svc.pemKeyContent == "" {
		if envKey := os.Getenv("SSH_PRIVATE_KEY"); envKey != "" {
			svc.pemKeyContent = envKey
			log.Println("Loaded PEM key from SSH_PRIVATE_KEY environment variable")
		}
	}

	if svc.pemKeyContent == "" {
		log.Println("Warning: No SSH private key configured for dev mode")
	}

	return svc
}

// GetStatus returns the current dev mode status
func (s *DevModeService) GetStatus(ctx context.Context) (*models.DevModeStatus, error) {
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}

	status := &models.DevModeStatus{
		IsEnabled: settings.IsEnabled,
	}

	if settings.AllowedIP.Valid {
		status.AllowedIP = settings.AllowedIP.String
	}

	if settings.EnabledBy != nil {
		status.EnabledByID = settings.EnabledBy.String()
		if name, err := s.repo.GetEnabledByUser(ctx, *settings.EnabledBy); err == nil {
			status.EnabledBy = name
		}
	}

	if settings.EnabledAt.Valid {
		status.EnabledAt = settings.EnabledAt.Time.Format("Jan 02, 2006 15:04 MST")
	}

	return status, nil
}

// EnableDevMode enables SSH access for a specific IP
func (s *DevModeService) EnableDevMode(ctx context.Context, allowedIP string, userID uuid.UUID) error {
	// Validate IP address
	if net.ParseIP(allowedIP) == nil {
		return fmt.Errorf("invalid IP address: %s", allowedIP)
	}

	ipCidr := allowedIP + "/32"
	description := fmt.Sprintf("Dev mode SSH access - %s", userID.String())

	// Add SSH rule to security group
	ruleID, err := s.addSSHRule(ipCidr, description)
	if err != nil {
		log.Printf("Failed to add SSH rule: %v", err)
		return fmt.Errorf("failed to add SSH rule: %w", err)
	}

	// Update database
	err = s.repo.SetEnabled(ctx, true, userID, allowedIP, ruleID)
	if err != nil {
		// Rollback the security group rule
		s.removeSSHRule(ruleID)
		return fmt.Errorf("failed to update database: %w", err)
	}

	log.Printf("Dev mode enabled for IP %s by user %s", allowedIP, userID)
	return nil
}

// DisableDevMode disables SSH access and kills all sessions
func (s *DevModeService) DisableDevMode(ctx context.Context, userID uuid.UUID) error {
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return err
	}

	// Remove security group rule
	if settings.SGRuleID.Valid && settings.SGRuleID.String != "" {
		if err := s.removeSSHRule(settings.SGRuleID.String); err != nil {
			log.Printf("Failed to remove SSH rule: %v", err)
		}
	}

	// Kill all SSH sessions on local server
	s.killAllSessions()

	// Update database
	err = s.repo.SetEnabled(ctx, false, uuid.Nil, "", "")
	if err != nil {
		return fmt.Errorf("failed to update database: %w", err)
	}

	log.Printf("Dev mode disabled by user %s", userID)
	return nil
}

// addSSHRule uses AWS CLI to add an SSH ingress rule
func (s *DevModeService) addSSHRule(ipCidr, description string) (string, error) {
	ipPermissions := fmt.Sprintf(`[{"IpProtocol":"tcp","FromPort":22,"ToPort":22,"IpRanges":[{"CidrIp":"%s","Description":"%s"}]}]`, ipCidr, description)

	cmd := exec.Command("aws", "ec2", "authorize-security-group-ingress",
		"--group-id", s.securityGroup,
		"--ip-permissions", ipPermissions,
		"--region", s.region,
		"--output", "json")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("aws cli error: %s - %w", string(output), err)
	}

	// Parse output to get rule ID
	var result struct {
		SecurityGroupRules []struct {
			SecurityGroupRuleId string `json:"SecurityGroupRuleId"`
		} `json:"SecurityGroupRules"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		log.Printf("Failed to parse AWS CLI output: %s", string(output))
		return "", nil
	}

	if len(result.SecurityGroupRules) > 0 {
		return result.SecurityGroupRules[0].SecurityGroupRuleId, nil
	}

	return "", nil
}

// removeSSHRule uses AWS CLI to remove a security group rule
func (s *DevModeService) removeSSHRule(ruleID string) error {
	cmd := exec.Command("aws", "ec2", "revoke-security-group-ingress",
		"--group-id", s.securityGroup,
		"--security-group-rule-ids", ruleID,
		"--region", s.region)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("aws cli error: %s - %w", string(output), err)
	}

	return nil
}

// ListSSHSessions lists active SSH sessions on the local server
func (s *DevModeService) ListSSHSessions(ctx context.Context) ([]models.SSHSession, error) {
	cmd := exec.Command("who", "-u")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run who command: %w", err)
	}

	return s.parseWhoOutput(string(output)), nil
}

// parseWhoOutput parses the output of the who -u command
func (s *DevModeService) parseWhoOutput(output string) []models.SSHSession {
	var sessions []models.SSHSession
	scanner := bufio.NewScanner(strings.NewReader(output))

	// who -u output format: username tty date time idle PID (host)
	// Example: ubuntu   pts/0        2024-01-15 10:30   .         12345 (192.168.1.100)
	re := regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2})\s+\S+\s+(\d+)\s*(?:\(([^)]+)\))?`)

	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)
		if len(matches) >= 5 {
			session := models.SSHSession{
				Username:  matches[1],
				TTY:       matches[2],
				LoginTime: matches[3],
				PID:       matches[4],
			}
			if len(matches) >= 6 && matches[5] != "" {
				session.SourceIP = matches[5]
			}
			sessions = append(sessions, session)
		}
	}

	return sessions
}

// KillSession kills a specific SSH session by TTY
func (s *DevModeService) KillSession(ctx context.Context, tty string) error {
	// Sanitize TTY to prevent command injection
	if !regexp.MustCompile(`^pts/\d+$`).MatchString(tty) && !regexp.MustCompile(`^tty\d+$`).MatchString(tty) {
		return fmt.Errorf("invalid TTY format: %s", tty)
	}

	cmd := exec.Command("pkill", "-9", "-t", tty)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// pkill returns exit code 1 if no processes matched, which is fine
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil
		}
		return fmt.Errorf("failed to kill session: %s - %w", string(output), err)
	}

	log.Printf("Killed SSH session on TTY %s", tty)
	return nil
}

// killAllSessions kills all SSH sessions on the local server
func (s *DevModeService) killAllSessions() {
	sessions, err := s.ListSSHSessions(context.Background())
	if err != nil {
		log.Printf("Failed to list sessions: %v", err)
		return
	}

	for _, session := range sessions {
		if err := s.KillSession(context.Background(), session.TTY); err != nil {
			log.Printf("Failed to kill session %s: %v", session.TTY, err)
		}
	}
}

// GetPEMKeyContent returns the SSH private key content
func (s *DevModeService) GetPEMKeyContent() (string, error) {
	if s.pemKeyContent == "" {
		return "", fmt.Errorf("SSH private key not configured")
	}
	return s.pemKeyContent, nil
}

// GetPEMKeyPath returns the path to the PEM key file (for display purposes)
func (s *DevModeService) GetPEMKeyPath() string {
	return s.pemKeyPath
}

// HasPEMKey returns true if a PEM key is configured
func (s *DevModeService) HasPEMKey() bool {
	return s.pemKeyContent != ""
}

// GetSecurityGroupID returns the configured security group ID
func (s *DevModeService) GetSecurityGroupID() string {
	return s.securityGroup
}

// GetCurrentInstanceIP returns the current EC2 instance's public IP
func (s *DevModeService) GetCurrentInstanceIP() string {
	// Try to get from instance metadata
	cmd := exec.Command("curl", "-s", "--connect-timeout", "2", "http://169.254.169.254/latest/meta-data/public-ipv4")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
