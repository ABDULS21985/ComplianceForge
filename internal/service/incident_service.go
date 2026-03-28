package service

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

const (
	// gdprBreachNotificationHours is the GDPR Article 33 deadline for notifying
	// the supervisory authority of a personal data breach.
	gdprBreachNotificationHours = 72
)

var (
	ErrIncidentNotFound          = errors.New("incident not found")
	ErrIncidentInvalidTransition = errors.New("invalid incident status transition")
)

// IncidentRepository defines the data access interface for incidents.
type IncidentRepository interface {
	Create(ctx context.Context, incident *models.Incident) error
	GetByID(ctx context.Context, id string) (*models.Incident, error)
	Update(ctx context.Context, incident *models.Incident) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, orgID string, page, pageSize int) ([]models.Incident, int, error)
	ListByStatus(ctx context.Context, orgID string, status models.IncidentStatus) ([]models.Incident, error)
	ListBreachNotifiable(ctx context.Context, orgID string) ([]models.Incident, error)
}

// IncidentService handles business logic for security incident management,
// including GDPR 72-hour breach notification tracking.
type IncidentService struct {
	incidentRepo IncidentRepository
	logger       zerolog.Logger
}

// NewIncidentService constructs a new IncidentService.
func NewIncidentService(incidentRepo IncidentRepository, logger zerolog.Logger) *IncidentService {
	return &IncidentService{
		incidentRepo: incidentRepo,
		logger:       logger.With().Str("service", "incident").Logger(),
	}
}

// Create persists a new incident and sets the GDPR notification deadline if
// the incident is flagged as breach-notifiable.
func (s *IncidentService) Create(ctx context.Context, incident *models.Incident) error {
	incident.Status = models.IncidentStatusOpen

	// Set detection time if not provided.
	if incident.DetectedAt == nil {
		now := time.Now()
		incident.DetectedAt = &now
	}

	// Calculate GDPR 72-hour notification deadline for breach-notifiable incidents.
	if incident.IsBreachNotifiable {
		deadline := incident.DetectedAt.Add(gdprBreachNotificationHours * time.Hour)
		incident.NotificationDeadline = &deadline
		s.logger.Warn().
			Str("title", incident.Title).
			Time("deadline", deadline).
			Msg("breach-notifiable incident created - GDPR 72h deadline set")
	}

	if err := s.incidentRepo.Create(ctx, incident); err != nil {
		s.logger.Error().Err(err).Str("title", incident.Title).Msg("failed to create incident")
		return err
	}

	s.logger.Info().
		Str("incident_id", incident.ID).
		Str("severity", string(incident.Severity)).
		Msg("incident created")
	return nil
}

// GetByID retrieves an incident by its unique identifier.
func (s *IncidentService) GetByID(ctx context.Context, id string) (*models.Incident, error) {
	incident, err := s.incidentRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrIncidentNotFound
	}
	return incident, nil
}

// Update modifies an existing incident.
func (s *IncidentService) Update(ctx context.Context, incident *models.Incident) error {
	if _, err := s.incidentRepo.GetByID(ctx, incident.ID); err != nil {
		return ErrIncidentNotFound
	}

	// Recalculate GDPR deadline if breach-notifiable flag changes.
	if incident.IsBreachNotifiable && incident.NotificationDeadline == nil && incident.DetectedAt != nil {
		deadline := incident.DetectedAt.Add(gdprBreachNotificationHours * time.Hour)
		incident.NotificationDeadline = &deadline
	}

	if err := s.incidentRepo.Update(ctx, incident); err != nil {
		s.logger.Error().Err(err).Str("incident_id", incident.ID).Msg("failed to update incident")
		return err
	}

	s.logger.Info().Str("incident_id", incident.ID).Msg("incident updated")
	return nil
}

// Delete soft-deletes an incident by ID.
func (s *IncidentService) Delete(ctx context.Context, id string) error {
	if _, err := s.incidentRepo.GetByID(ctx, id); err != nil {
		return ErrIncidentNotFound
	}

	if err := s.incidentRepo.Delete(ctx, id); err != nil {
		s.logger.Error().Err(err).Str("incident_id", id).Msg("failed to delete incident")
		return err
	}

	s.logger.Info().Str("incident_id", id).Msg("incident deleted")
	return nil
}

// List returns a paginated list of incidents for an organization.
func (s *IncidentService) List(ctx context.Context, orgID string, page, pageSize int) ([]models.Incident, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	incidents, total, err := s.incidentRepo.List(ctx, orgID, page, pageSize)
	if err != nil {
		s.logger.Error().Err(err).Str("org_id", orgID).Msg("failed to list incidents")
		return nil, 0, err
	}
	return incidents, total, nil
}

// UpdateStatus transitions an incident to a new status with validation.
// Valid transitions: Open -> Investigating -> Contained -> Resolved -> Closed.
func (s *IncidentService) UpdateStatus(ctx context.Context, id string, newStatus models.IncidentStatus) error {
	incident, err := s.incidentRepo.GetByID(ctx, id)
	if err != nil {
		return ErrIncidentNotFound
	}

	if !isValidIncidentTransition(incident.Status, newStatus) {
		s.logger.Warn().
			Str("incident_id", id).
			Str("current", string(incident.Status)).
			Str("requested", string(newStatus)).
			Msg("invalid incident status transition")
		return ErrIncidentInvalidTransition
	}

	now := time.Now()
	incident.Status = newStatus

	switch newStatus {
	case models.IncidentStatusContained:
		incident.ContainedAt = &now
	case models.IncidentStatusResolved:
		incident.ResolvedAt = &now
	}

	if err := s.incidentRepo.Update(ctx, incident); err != nil {
		s.logger.Error().Err(err).Str("incident_id", id).Msg("failed to update incident status")
		return err
	}

	s.logger.Info().
		Str("incident_id", id).
		Str("new_status", string(newStatus)).
		Msg("incident status updated")
	return nil
}

// GetBreachNotifiable returns all breach-notifiable incidents for an organization,
// particularly those approaching or past the GDPR 72-hour notification deadline.
func (s *IncidentService) GetBreachNotifiable(ctx context.Context, orgID string) ([]models.Incident, error) {
	incidents, err := s.incidentRepo.ListBreachNotifiable(ctx, orgID)
	if err != nil {
		s.logger.Error().Err(err).Str("org_id", orgID).Msg("failed to get breach-notifiable incidents")
		return nil, err
	}

	// Log warnings for incidents approaching or past their notification deadline.
	now := time.Now()
	for _, inc := range incidents {
		if inc.NotificationDeadline != nil {
			remaining := inc.NotificationDeadline.Sub(now)
			if remaining < 0 {
				s.logger.Error().
					Str("incident_id", inc.ID).
					Str("title", inc.Title).
					Dur("overdue_by", -remaining).
					Msg("GDPR notification deadline EXCEEDED")
			} else if remaining < 24*time.Hour {
				s.logger.Warn().
					Str("incident_id", inc.ID).
					Str("title", inc.Title).
					Dur("remaining", remaining).
					Msg("GDPR notification deadline approaching")
			}
		}
	}

	return incidents, nil
}

// EscalateIncident raises the severity of an incident and logs the escalation.
func (s *IncidentService) EscalateIncident(ctx context.Context, id string, newSeverity models.IncidentSeverity, reason string) error {
	incident, err := s.incidentRepo.GetByID(ctx, id)
	if err != nil {
		return ErrIncidentNotFound
	}

	previousSeverity := incident.Severity
	incident.Severity = newSeverity

	// If escalating to Critical, automatically flag as breach-notifiable if not already.
	if newSeverity == models.IncidentSeverityCritical && !incident.IsBreachNotifiable {
		incident.IsBreachNotifiable = true
		if incident.DetectedAt != nil {
			deadline := incident.DetectedAt.Add(gdprBreachNotificationHours * time.Hour)
			incident.NotificationDeadline = &deadline
		}
		s.logger.Warn().
			Str("incident_id", id).
			Msg("critical escalation - auto-flagged as breach-notifiable")
	}

	if err := s.incidentRepo.Update(ctx, incident); err != nil {
		s.logger.Error().Err(err).Str("incident_id", id).Msg("failed to escalate incident")
		return err
	}

	s.logger.Info().
		Str("incident_id", id).
		Str("from_severity", string(previousSeverity)).
		Str("to_severity", string(newSeverity)).
		Str("reason", reason).
		Msg("incident escalated")
	return nil
}

// isValidIncidentTransition checks whether a status transition is allowed.
func isValidIncidentTransition(current, next models.IncidentStatus) bool {
	transitions := map[models.IncidentStatus][]models.IncidentStatus{
		models.IncidentStatusOpen:          {models.IncidentStatusInvestigating},
		models.IncidentStatusInvestigating: {models.IncidentStatusContained, models.IncidentStatusResolved},
		models.IncidentStatusContained:     {models.IncidentStatusResolved},
		models.IncidentStatusResolved:      {models.IncidentStatusClosed},
	}

	allowed, ok := transitions[current]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == next {
			return true
		}
	}
	return false
}
