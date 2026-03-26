package handler

import (
	"encoding/json"
	"net/http"

	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/dto"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
// @Description  Updates a user's profile. Only role changes require the caller to have the dev role.
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

	authenticatedUser, ok := h.requireSelfOrDev(w, r, uid)
	if !ok {
		return
	}

	var req dto.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	targetUser, err := h.queries.GetUserByID(r.Context(), uid)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	var role database.NullUserRole
	if req.Role != nil {
		requestedRole := database.UserRole(*req.Role)
		roleChanged := !targetUser.Role.Valid || targetUser.Role.UserRole != requestedRole
		if roleChanged && (!authenticatedUser.Role.Valid || authenticatedUser.Role.UserRole != database.UserRoleDev) {
			h.respondError(w, http.StatusForbidden, "Only dev may update user roles")
			return
		}
		if roleChanged {
			role = database.NullUserRole{UserRole: requestedRole, Valid: true}
		}
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

	if _, ok := h.requireSelfOrDev(w, r, uid); !ok {
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
		DateCreated:   fromPgDate(user.DateCreated),
		DateModified:  fromPgDate(user.DateModified),
	}
}
