package validator

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

// Validator wraps go-playground/validator with user-friendly error formatting.
type Validator struct {
	validate *validator.Validate
}

// New creates a new Validator instance with default settings.
func New() *Validator {
	return &Validator{
		validate: validator.New(validator.WithRequiredStructEnabled()),
	}
}

// ValidateStruct validates the given struct and returns a user-friendly error
// message if validation fails, or nil if the struct is valid.
func (v *Validator) ValidateStruct(s interface{}) error {
	err := v.validate.Struct(s)
	if err == nil {
		return nil
	}

	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return err
	}

	var messages []string
	for _, fe := range validationErrors {
		messages = append(messages, formatFieldError(fe))
	}

	return fmt.Errorf("%s", strings.Join(messages, "; "))
}

// formatFieldError converts a single validator.FieldError into a human-readable string.
func formatFieldError(fe validator.FieldError) string {
	field := fe.Field()

	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", field)
	case "email":
		return fmt.Sprintf("%s must be a valid email address", field)
	case "min":
		return fmt.Sprintf("%s must be at least %s characters", field, fe.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s characters", field, fe.Param())
	case "gte":
		return fmt.Sprintf("%s must be greater than or equal to %s", field, fe.Param())
	case "lte":
		return fmt.Sprintf("%s must be less than or equal to %s", field, fe.Param())
	case "oneof":
		return fmt.Sprintf("%s must be one of: %s", field, fe.Param())
	case "uuid":
		return fmt.Sprintf("%s must be a valid UUID", field)
	case "url":
		return fmt.Sprintf("%s must be a valid URL", field)
	case "len":
		return fmt.Sprintf("%s must be exactly %s characters", field, fe.Param())
	default:
		return fmt.Sprintf("%s failed validation: %s", field, fe.Tag())
	}
}
