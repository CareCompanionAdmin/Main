package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"carecompanion/internal/repository"
)

var (
	ErrTicketNotFound   = errors.New("ticket not found")
	ErrEmptySubject     = errors.New("subject cannot be empty")
	ErrEmptyDescription = errors.New("description cannot be empty")
	ErrEmptyReply       = errors.New("reply message cannot be empty")
)

// UserSupportService handles user-facing support ticket operations
type UserSupportService struct {
	repo repository.UserSupportRepository
}

// NewUserSupportService creates a new user support service
func NewUserSupportService(repo repository.UserSupportRepository) *UserSupportService {
	return &UserSupportService{repo: repo}
}

// CreateTicketRequest represents a request to create a ticket
type CreateTicketRequest struct {
	Subject     string `json:"subject"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
}

// CreateTicket creates a new support ticket for a user
func (s *UserSupportService) CreateTicket(ctx context.Context, userID uuid.UUID, req *CreateTicketRequest) (*repository.SupportTicket, error) {
	if req.Subject == "" {
		return nil, ErrEmptySubject
	}
	if req.Description == "" {
		return nil, ErrEmptyDescription
	}

	return s.repo.CreateTicket(ctx, userID, req.Subject, req.Description, req.Priority)
}

// GetTickets returns all tickets for a user
func (s *UserSupportService) GetTickets(ctx context.Context, userID uuid.UUID) ([]repository.SupportTicket, error) {
	return s.repo.GetTickets(ctx, userID)
}

// GetTicketByID returns a specific ticket, validating user ownership
func (s *UserSupportService) GetTicketByID(ctx context.Context, ticketID, userID uuid.UUID) (*repository.SupportTicket, error) {
	ticket, err := s.repo.GetTicketByID(ctx, ticketID, userID)
	if err != nil {
		return nil, err
	}
	if ticket == nil {
		return nil, ErrTicketNotFound
	}
	return ticket, nil
}

// GetTicketWithMessages returns a ticket with its messages
func (s *UserSupportService) GetTicketWithMessages(ctx context.Context, ticketID, userID uuid.UUID) (*repository.SupportTicket, []repository.TicketMessage, error) {
	ticket, err := s.repo.GetTicketByID(ctx, ticketID, userID)
	if err != nil {
		return nil, nil, err
	}
	if ticket == nil {
		return nil, nil, ErrTicketNotFound
	}

	messages, err := s.repo.GetTicketMessages(ctx, ticketID, userID)
	if err != nil {
		return nil, nil, err
	}

	// Mark ticket as read when viewing
	s.repo.MarkTicketRead(ctx, ticketID, userID)

	return ticket, messages, nil
}

// AddReply adds a reply message to a ticket
func (s *UserSupportService) AddReply(ctx context.Context, ticketID, userID uuid.UUID, message string) error {
	if message == "" {
		return ErrEmptyReply
	}

	// Verify ticket ownership
	ticket, err := s.repo.GetTicketByID(ctx, ticketID, userID)
	if err != nil {
		return err
	}
	if ticket == nil {
		return ErrTicketNotFound
	}

	return s.repo.AddMessage(ctx, ticketID, userID, message)
}

// MarkTicketRead marks a ticket as read
func (s *UserSupportService) MarkTicketRead(ctx context.Context, ticketID, userID uuid.UUID) error {
	return s.repo.MarkTicketRead(ctx, ticketID, userID)
}

// HasUnreadSupportMessages checks if user has any tickets with unread support replies
func (s *UserSupportService) HasUnreadSupportMessages(ctx context.Context, userID uuid.UUID) (bool, error) {
	return s.repo.HasUnreadSupportMessages(ctx, userID)
}

// GetUnreadTicketCount returns count of tickets with unread messages
func (s *UserSupportService) GetUnreadTicketCount(ctx context.Context, userID uuid.UUID) (int, error) {
	return s.repo.GetUnreadTicketCount(ctx, userID)
}
