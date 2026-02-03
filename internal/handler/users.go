package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/dto"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// GetUser retrieves a user by ID
// @Summary      Get user by ID
// @Description  Retrieves a user's profile
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        uid   path      string  true  "User UUID"
// @Success      200   {object}  dto.UserResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /users/{uid} [get]
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	uidStr := chi.URLParam(r, "uid")
	uid, err := uuid.Parse(uidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	user, err := h.queries.GetUserByID(r.Context(), uid)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, toUserResponse(user))
}

// UpdateUser updates a user's profile
// @Summary      Update user
// @Description  Updates a user's profile. Users can only update their own profile.
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        uid   path      string                  true  "User UUID"
// @Param        body  body      dto.UpdateUserRequest   true  "User update data"
// @Success      200   {object}  dto.UserResponse
// @Failure      400   {object}  ErrorResponse
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /users/{uid} [put]
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	uidStr := chi.URLParam(r, "uid")
	uid, err := uuid.Parse(uidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	var req dto.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	var role database.NullUserRole
	if req.Role != nil {
		role = database.NullUserRole{UserRole: database.UserRole(*req.Role), Valid: true}
	}

	user, err := h.queries.UpdateUser(r.Context(), database.UpdateUserParams{
		Uid:           uid,
		FirstName:     toPgText(req.FirstName),
		LastName:      toPgText(req.LastName),
		PersonalEmail: toPgText(req.PersonalEmail),
		SchoolEmail:   toPgText(req.SchoolEmail),
		Phone:         toPgText(req.Phone),
		GradYear:      toPgInt4(req.GradYear),
		Role:          role,
	})
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, toUserResponse(user))
}

// DeleteUser deletes a user
// @Summary      Delete user
// @Description  Deletes a user account. Users can only delete their own account.
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        uid   path      string  true  "User UUID"
// @Success      204
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /users/{uid} [delete]
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	uidStr := chi.URLParam(r, "uid")
	uid, err := uuid.Parse(uidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	if err := h.queries.DeleteUser(r.Context(), uid); err != nil {
		h.handleDBError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetUserOrganizations lists organizations a user belongs to
// @Summary      List user's organizations
// @Description  Returns all organizations the user is a member of
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        uid   path      string  true  "User UUID"
// @Success      200   {array}   dto.OrganizationResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /users/{uid}/organizations [get]
func (h *Handler) GetUserOrganizations(w http.ResponseWriter, r *http.Request) {
	uidStr := chi.URLParam(r, "uid")
	uid, err := uuid.Parse(uidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	orgs, err := h.queries.GetUserOrganizations(r.Context(), uid)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	response := make([]dto.OrganizationResponse, len(orgs))
	for i, org := range orgs {
		response[i] = dto.OrganizationResponse{
			OID:  org.Oid,
			Name: org.Name,
		}
	}

	h.respondJSON(w, http.StatusOK, response)
}

// GetUserEvents lists events a user is registered for
// @Summary      List user's events
// @Description  Returns all events the user is registered for
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        uid   path      string  true  "User UUID"
// @Success      200   {array}   dto.EventResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /users/{uid}/events [get]
func (h *Handler) GetUserEvents(w http.ResponseWriter, r *http.Request) {
	uidStr := chi.URLParam(r, "uid")
	uid, err := uuid.Parse(uidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	events, err := h.queries.GetUserEvents(r.Context(), uid)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	response := make([]dto.EventResponse, len(events))
	for i, event := range events {
		response[i] = dto.EventResponse{
			EID:         event.Eid,
			Location:    fromPgText(event.Location),
			EventTime:   fromPgTimestamp(event.EventTime),
			Description: fromPgText(event.Description),
		}
	}

	h.respondJSON(w, http.StatusOK, response)
}

// Helper functions for converting between DTO and database types
func toUserResponse(user database.User) dto.UserResponse {
	return dto.UserResponse{
		UID:           user.Uid,
		FirstName:     user.FirstName,
		LastName:      user.LastName,
		PersonalEmail: fromPgText(user.PersonalEmail),
		SchoolEmail:   fromPgText(user.SchoolEmail),
		Phone:         fromPgText(user.Phone),
		GradYear:      fromPgInt4(user.GradYear),
		Role:          string(user.Role.UserRole),
	}
}

func toPgText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func toPgInt4(i *int) pgtype.Int4 {
	if i == nil {
		return pgtype.Int4{Valid: false}
	}
	return pgtype.Int4{Int32: int32(*i), Valid: true}
}

func fromPgText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func fromPgInt4(i pgtype.Int4) *int {
	if !i.Valid {
		return nil
	}
	v := int(i.Int32)
	return &v
}

func fromPgTimestamp(t pgtype.Timestamp) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
