package service

import (
	"context"

	"github.com/complianceforge/platform/internal/models"
)

// FrameworkRepository and ControlRepository are retained for the
// older reporting engine until that module is migrated to implementation-state
// scoring. The HTTP compliance slice does not construct or use these contracts.
type FrameworkRepository interface {
	Create(context.Context, *models.ComplianceFramework) error
	GetByID(context.Context, string) (*models.ComplianceFramework, error)
	GetWithControls(context.Context, string) (*models.ComplianceFramework, error)
	Update(context.Context, *models.ComplianceFramework) error
	Delete(context.Context, string) error
	List(context.Context, string, int, int) ([]models.ComplianceFramework, int, error)
}

type ControlRepository interface {
	Create(context.Context, *models.Control) error
	GetByID(context.Context, string) (*models.Control, error)
	Update(context.Context, *models.Control) error
	Delete(context.Context, string) error
	List(context.Context, string, int, int) ([]models.Control, int, error)
	ListByFrameworkID(context.Context, string) ([]models.Control, error)
	CountByStatus(context.Context, string) (map[models.ComplianceStatus]int, error)
}
