// Package accesscontrol contains persistence-neutral access-control errors
// shared by repositories and services without creating a package cycle.
package accesscontrol

import "errors"

var (
	ErrNotFound        = errors.New("access control record not found")
	ErrConflict        = errors.New("access control record changed or conflicts")
	ErrInvalidRelation = errors.New("access control relation is invalid")
	ErrState           = errors.New("access control record state prevents the operation")
	ErrImmutable       = errors.New("access control evidence is immutable")
)
