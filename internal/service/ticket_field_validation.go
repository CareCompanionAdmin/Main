package service

import (
	"errors"
	"fmt"
)

// ErrInvalidTicketField is wrapped by every ticket field-validation failure so
// HTTP handlers can map it to 400 (vs. a 500 for genuine errors).
var ErrInvalidTicketField = errors.New("invalid ticket field")

// validTicketStatuses whitelists settable ticket status values.
var validTicketStatuses = map[string]bool{
	"open": true, "in_progress": true, "waiting_on_user": true,
	"resolved": true, "closed": true,
}

// ValidateTicketFields checks a requested type/priority/status change.
// An empty string for a field means "no change" and is always allowed.
// actorIsStaff gates the 'urgent' priority and any status change — app users
// may set priority only up to 'high' and may never set status directly.
func ValidateTicketFields(actorIsStaff bool, ticketType, priority, status string) error {
	if ticketType != "" && !validTicketTypes[ticketType] {
		return fmt.Errorf("%w: type %q", ErrInvalidTicketField, ticketType)
	}
	if priority != "" {
		switch priority {
		case "low", "normal", "high":
			// allowed for everyone
		case "urgent":
			if !actorIsStaff {
				return fmt.Errorf("%w: priority %q is reserved for support staff", ErrInvalidTicketField, priority)
			}
		default:
			return fmt.Errorf("%w: priority %q", ErrInvalidTicketField, priority)
		}
	}
	if status != "" {
		if !actorIsStaff {
			return fmt.Errorf("%w: users cannot change ticket status", ErrInvalidTicketField)
		}
		if !validTicketStatuses[status] {
			return fmt.Errorf("%w: status %q", ErrInvalidTicketField, status)
		}
	}
	return nil
}
