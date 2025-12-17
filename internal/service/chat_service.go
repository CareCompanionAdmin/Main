package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

var (
	ErrThreadNotFound    = errors.New("thread not found")
	ErrNotParticipant    = errors.New("user is not a participant in this thread")
	ErrMessageNotFound   = errors.New("message not found")
	ErrNotMessageOwner   = errors.New("user is not the owner of this message")
	ErrEmptyMessage      = errors.New("message text cannot be empty")
)

type ChatService struct {
	chatRepo   repository.ChatRepository
	userRepo   repository.UserRepository
	familyRepo repository.FamilyRepository
	childRepo  repository.ChildRepository
}

func NewChatService(
	chatRepo repository.ChatRepository,
	userRepo repository.UserRepository,
	familyRepo repository.FamilyRepository,
	childRepo repository.ChildRepository,
) *ChatService {
	return &ChatService{
		chatRepo:   chatRepo,
		userRepo:   userRepo,
		familyRepo: familyRepo,
		childRepo:  childRepo,
	}
}

// CreateThread creates a new chat thread
func (s *ChatService) CreateThread(ctx context.Context, familyID, creatorID uuid.UUID, req *models.CreateThreadRequest) (*models.ChatThread, error) {
	// Get creator's role in family
	membership, err := s.familyRepo.GetMembership(ctx, familyID, creatorID)
	if err != nil || membership == nil {
		return nil, errors.New("user is not a member of this family")
	}

	thread := &models.ChatThread{
		FamilyID:       familyID,
		ChildID:        req.ChildID,
		ThreadType:     req.ThreadType,
		RelatedAlertID: req.RelatedAlertID,
		CreatedBy:      creatorID,
	}
	if req.Title != "" {
		thread.Title.String = req.Title
		thread.Title.Valid = true
	}

	if err := s.chatRepo.CreateThread(ctx, thread); err != nil {
		return nil, err
	}

	// Add creator as participant
	s.chatRepo.AddParticipant(ctx, thread.ID, creatorID, membership.Role)

	// Add other participants
	for _, userID := range req.Participants {
		if userID != creatorID {
			// Get their role
			m, _ := s.familyRepo.GetMembership(ctx, familyID, userID)
			if m != nil {
				s.chatRepo.AddParticipant(ctx, thread.ID, userID, m.Role)
			}
		}
	}

	// Send initial message if provided
	if req.InitialMessage != "" {
		s.SendMessage(ctx, thread.ID, creatorID, &models.SendMessageRequest{
			MessageText: req.InitialMessage,
		})
	}

	return s.chatRepo.GetThread(ctx, thread.ID)
}

// SendMessage sends a message to a thread
func (s *ChatService) SendMessage(ctx context.Context, threadID, senderID uuid.UUID, req *models.SendMessageRequest) (*models.ChatMessage, error) {
	// Require either message text or attachments
	if req.MessageText == "" && len(req.Attachments) == 0 {
		return nil, ErrEmptyMessage
	}

	// Verify sender is a participant
	isParticipant, err := s.chatRepo.IsParticipant(ctx, threadID, senderID)
	if err != nil {
		return nil, err
	}
	if !isParticipant {
		return nil, ErrNotParticipant
	}

	message := &models.ChatMessage{
		ThreadID:    threadID,
		SenderID:    senderID,
		MessageText: req.MessageText,
		Attachments: req.Attachments,
	}

	if err := s.chatRepo.CreateMessage(ctx, message); err != nil {
		return nil, err
	}

	// Load sender info
	message.Sender, _ = s.userRepo.GetByID(ctx, senderID)

	return message, nil
}

// EditMessage edits a message
func (s *ChatService) EditMessage(ctx context.Context, messageID, userID uuid.UUID, req *models.EditMessageRequest) (*models.ChatMessage, error) {
	if req.MessageText == "" {
		return nil, ErrEmptyMessage
	}

	message, err := s.chatRepo.GetMessage(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if message == nil {
		return nil, ErrMessageNotFound
	}

	// Verify user owns the message
	if message.SenderID != userID {
		return nil, ErrNotMessageOwner
	}

	message.MessageText = req.MessageText
	if err := s.chatRepo.UpdateMessage(ctx, message); err != nil {
		return nil, err
	}

	return s.chatRepo.GetMessage(ctx, messageID)
}

// DeleteMessage deletes a message
func (s *ChatService) DeleteMessage(ctx context.Context, messageID, userID uuid.UUID) error {
	message, err := s.chatRepo.GetMessage(ctx, messageID)
	if err != nil {
		return err
	}
	if message == nil {
		return ErrMessageNotFound
	}

	// Verify user owns the message
	if message.SenderID != userID {
		return ErrNotMessageOwner
	}

	return s.chatRepo.DeleteMessage(ctx, messageID)
}

// GetThreads gets all threads for a family that a user can see
func (s *ChatService) GetThreads(ctx context.Context, familyID, userID uuid.UUID) ([]models.ChatThread, error) {
	threads, err := s.chatRepo.GetThreadsByFamily(ctx, familyID)
	if err != nil {
		return nil, err
	}

	// Filter to threads where user is participant and add unread counts
	var result []models.ChatThread
	for _, thread := range threads {
		isParticipant, _ := s.chatRepo.IsParticipant(ctx, thread.ID, userID)
		if isParticipant {
			thread.UnreadCount, _ = s.chatRepo.GetUnreadCount(ctx, thread.ID, userID)

			// Populate participants
			participants, err := s.chatRepo.GetParticipants(ctx, thread.ID)
			if err == nil {
				thread.Participants = participants
			}

			// Populate child name if thread is about a child
			if thread.ChildID != nil {
				child, err := s.childRepo.GetByID(ctx, *thread.ChildID)
				if err == nil && child != nil {
					thread.ChildName = child.FirstName
					if child.LastName.Valid {
						thread.ChildName += " " + child.LastName.String
					}
				}
			}

			result = append(result, thread)
		}
	}

	return result, nil
}

// GetThread gets a thread with messages
func (s *ChatService) GetThread(ctx context.Context, threadID, userID uuid.UUID) (*models.ChatThread, error) {
	thread, err := s.chatRepo.GetThread(ctx, threadID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, ErrThreadNotFound
	}

	// Verify user is a participant
	isParticipant, _ := s.chatRepo.IsParticipant(ctx, threadID, userID)
	if !isParticipant {
		return nil, ErrNotParticipant
	}

	thread.UnreadCount, _ = s.chatRepo.GetUnreadCount(ctx, threadID, userID)

	// Populate participants
	participants, err := s.chatRepo.GetParticipants(ctx, threadID)
	if err == nil {
		thread.Participants = participants
	}

	// Populate child name if thread is about a child
	if thread.ChildID != nil {
		child, err := s.childRepo.GetByID(ctx, *thread.ChildID)
		if err == nil && child != nil {
			thread.ChildName = child.FirstName
			if child.LastName.Valid {
				thread.ChildName += " " + child.LastName.String
			}
		}
	}

	return thread, nil
}

// GetMessages gets messages for a thread with pagination
func (s *ChatService) GetMessages(ctx context.Context, threadID, userID uuid.UUID, limit, offset int) ([]models.ChatMessage, error) {
	// Verify user is a participant
	isParticipant, err := s.chatRepo.IsParticipant(ctx, threadID, userID)
	if err != nil {
		return nil, err
	}
	if !isParticipant {
		return nil, ErrNotParticipant
	}

	messages, err := s.chatRepo.GetMessages(ctx, threadID, limit, offset)
	if err != nil {
		return nil, err
	}

	// Mark as read
	s.chatRepo.UpdateLastRead(ctx, threadID, userID)

	return messages, nil
}

// MarkAsRead marks all messages in a thread as read
func (s *ChatService) MarkAsRead(ctx context.Context, threadID, userID uuid.UUID) error {
	return s.chatRepo.UpdateLastRead(ctx, threadID, userID)
}

// AddParticipant adds a user to a thread
func (s *ChatService) AddParticipant(ctx context.Context, threadID, userID, requesterID uuid.UUID) error {
	// Get thread to verify family membership
	thread, err := s.chatRepo.GetThread(ctx, threadID)
	if err != nil {
		return err
	}
	if thread == nil {
		return ErrThreadNotFound
	}

	// Verify requester is a participant
	isParticipant, _ := s.chatRepo.IsParticipant(ctx, threadID, requesterID)
	if !isParticipant {
		return ErrNotParticipant
	}

	// Get user's role in family
	membership, err := s.familyRepo.GetMembership(ctx, thread.FamilyID, userID)
	if err != nil || membership == nil {
		return errors.New("user is not a member of this family")
	}

	return s.chatRepo.AddParticipant(ctx, threadID, userID, membership.Role)
}

// RemoveParticipant removes a user from a thread
func (s *ChatService) RemoveParticipant(ctx context.Context, threadID, userID, requesterID uuid.UUID) error {
	// Verify requester is a participant
	isParticipant, _ := s.chatRepo.IsParticipant(ctx, threadID, requesterID)
	if !isParticipant {
		return ErrNotParticipant
	}

	return s.chatRepo.RemoveParticipant(ctx, threadID, userID)
}

// GetTotalUnreadCount gets total unread count for a user
func (s *ChatService) GetTotalUnreadCount(ctx context.Context, familyID, userID uuid.UUID) (int, error) {
	return s.chatRepo.GetTotalUnreadCount(ctx, familyID, userID)
}

// DeleteThread archives a thread
func (s *ChatService) DeleteThread(ctx context.Context, threadID, userID uuid.UUID) error {
	thread, err := s.chatRepo.GetThread(ctx, threadID)
	if err != nil {
		return err
	}
	if thread == nil {
		return ErrThreadNotFound
	}

	// Only creator can delete
	if thread.CreatedBy != userID {
		return errors.New("only the thread creator can delete the thread")
	}

	return s.chatRepo.DeleteThread(ctx, threadID)
}
