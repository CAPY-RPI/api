package dto

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// User DTOs
// ============================================================================

type CreateUserRequest struct {
	FirstName     string `json:"first_name" validate:"required,min=1,max=100"`
	LastName      string `json:"last_name" validate:"required,min=1,max=100"`
	PersonalEmail string `json:"personal_email,omitempty" validate:"omitempty,email"`
	SchoolEmail   string `json:"school_email,omitempty" validate:"omitempty,email"`
	Phone         string `json:"phone,omitempty"`
	GradYear      int    `json:"grad_year,omitempty" validate:"omitempty,gte=2000,lte=2100"`
	Role          string `json:"role,omitempty" validate:"omitempty,oneof=student alumni faculty external dev"`
}

type UpdateUserRequest struct {
	FirstName     *string `json:"first_name,omitempty" validate:"omitempty,min=1,max=100"`
	LastName      *string `json:"last_name,omitempty" validate:"omitempty,min=1,max=100"`
	PersonalEmail *string `json:"personal_email,omitempty" validate:"omitempty,email"`
	SchoolEmail   *string `json:"school_email,omitempty" validate:"omitempty,email"`
	Phone         *string `json:"phone,omitempty"`
	GradYear      *int    `json:"grad_year,omitempty" validate:"omitempty,gte=2000,lte=2100"`
	Role          *string `json:"role,omitempty" validate:"omitempty,oneof=student alumni faculty external dev"`
}

type UserResponse struct {
	UID           uuid.UUID  `json:"uid"`
	FirstName     string     `json:"first_name"`
	LastName      string     `json:"last_name"`
	PersonalEmail *string    `json:"personal_email,omitempty"`
	SchoolEmail   *string    `json:"school_email,omitempty"`
	Phone         *string    `json:"phone,omitempty"`
	GradYear      *int       `json:"grad_year,omitempty"`
	Role          string     `json:"role"`
	DateCreated   *time.Time `json:"date_created,omitempty"`
	DateModified  *time.Time `json:"date_modified,omitempty"`
}

// ============================================================================
// Organization DTOs
// ============================================================================

type CreateOrganizationRequest struct {
	Name       string     `json:"name" validate:"required,min=1,max=200"`
	CreatorUID *uuid.UUID `json:"creator_uid,omitempty"` // Required for bot-created orgs
}

type UpdateOrganizationRequest struct {
	Name *string `json:"name,omitempty" validate:"omitempty,min=1,max=200"`
}

type OrganizationResponse struct {
	OID          uuid.UUID  `json:"oid"`
	Name         string     `json:"name"`
	DateCreated  *time.Time `json:"date_created,omitempty"`
	DateModified *time.Time `json:"date_modified,omitempty"`
}

type OrgMemberResponse struct {
	UID        uuid.UUID  `json:"uid"`
	FirstName  string     `json:"first_name"`
	LastName   string     `json:"last_name"`
	Email      *string    `json:"email,omitempty"`
	IsAdmin    bool       `json:"is_admin"`
	DateJoined *time.Time `json:"date_joined,omitempty"`
	LastActive *time.Time `json:"last_active,omitempty"`
}

type AddMemberRequest struct {
	UID     uuid.UUID `json:"uid" validate:"required"`
	IsAdmin bool      `json:"is_admin"`
}

// ============================================================================
// Event DTOs
// ============================================================================

type CreateEventRequest struct {
	Title       string     `json:"title,omitempty"`
	Location    string     `json:"location,omitempty"`
	EventTime   *time.Time `json:"event_time,omitempty"`
	Description string     `json:"description,omitempty"`
	OrgID       uuid.UUID  `json:"org_id" validate:"required"` // Which org is hosting
}

type UpdateEventRequest struct {
	Title       *string    `json:"title,omitempty"`
	Location    *string    `json:"location,omitempty"`
	EventTime   *time.Time `json:"event_time,omitempty"`
	Description *string    `json:"description,omitempty"`
}

type EventResponse struct {
	EID          uuid.UUID  `json:"eid"`
	Title        *string    `json:"title,omitempty"`
	Location     *string    `json:"location,omitempty"`
	EventTime    *time.Time `json:"event_time,omitempty"`
	Description  *string    `json:"description,omitempty"`
	DateCreated  *time.Time `json:"date_created,omitempty"`
	DateModified *time.Time `json:"date_modified,omitempty"`
}

type EventRegistrationResponse struct {
	UID            uuid.UUID  `json:"uid"`
	FirstName      string     `json:"first_name"`
	LastName       string     `json:"last_name"`
	IsAttending    bool       `json:"is_attending"`
	IsAdmin        bool       `json:"is_admin"`
	DateRegistered *time.Time `json:"date_registered,omitempty"`
}

type RegisterEventRequest struct {
	UID         *uuid.UUID `json:"uid,omitempty"` // For bot to register on behalf of user
	IsAttending bool       `json:"is_attending"`
}

// ============================================================================
// Pagination
// ============================================================================

type PaginationParams struct {
	Limit  int `json:"limit" validate:"gte=1,lte=100"`
	Offset int `json:"offset" validate:"gte=0"`
}

type PaginatedResponse[T any] struct {
	Data   []T `json:"data"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total,omitempty"`
}
