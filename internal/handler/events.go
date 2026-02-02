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

// ListEvents lists all events
// @Summary      List events
// @Description  Returns a paginated list of all public events
// @Tags         events
// @Accept       json
// @Produce      json
// @Param        limit   query     int  false  "Limit (default 20, max 100)"
// @Param        offset  query     int  false  "Offset (default 0)"
// @Success      200     {array}   dto.EventResponse
// @Security     CookieAuth
// @Router       /events [get]
func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)

	events, err := h.queries.ListEvents(r.Context(), database.ListEventsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	response := make([]dto.EventResponse, len(events))
	for i, e := range events {
		response[i] = toEventResponse(e)
	}

	h.respondJSON(w, http.StatusOK, response)
}

// CreateEvent creates a new event
// @Summary      Create event
// @Description  Creates a new event for an organization
// @Tags         events
// @Accept       json
// @Produce      json
// @Param        body  body      dto.CreateEventRequest  true  "Event data"
// @Success      201   {object}  dto.EventResponse
// @Failure      400   {object}  ErrorResponse
// @Failure      403   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /events [post]
func (h *Handler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.OrgID == uuid.Nil {
		h.respondError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	event, err := h.queries.CreateEvent(r.Context(), database.CreateEventParams{
		Location:    toPgTextFromString(req.Location),
		EventTime:   toPgTimestamp(req.EventTime),
		Description: toPgTextFromString(req.Description),
	})
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	// Add the organization as host
	if err := h.queries.AddEventHost(r.Context(), database.AddEventHostParams{
		Eid: event.Eid,
		Oid: req.OrgID,
	}); err != nil {
		h.handleDBError(w, err)
		return
	}

	h.respondJSON(w, http.StatusCreated, toEventResponse(event))
}

// GetEvent gets an event by ID
// @Summary      Get event
// @Description  Returns event details
// @Tags         events
// @Accept       json
// @Produce      json
// @Param        eid   path      string  true  "Event UUID"
// @Success      200   {object}  dto.EventResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /events/{eid} [get]
func (h *Handler) GetEvent(w http.ResponseWriter, r *http.Request) {
	eidStr := chi.URLParam(r, "eid")
	eid, err := uuid.Parse(eidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid event ID")
		return
	}

	event, err := h.queries.GetEventByID(r.Context(), eid)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, toEventResponse(event))
}

// UpdateEvent updates an event
// @Summary      Update event
// @Description  Updates event details. Requires event_admin role.
// @Tags         events
// @Accept       json
// @Produce      json
// @Param        eid   path      string                  true  "Event UUID"
// @Param        body  body      dto.UpdateEventRequest  true  "Event update data"
// @Success      200   {object}  dto.EventResponse
// @Failure      400   {object}  ErrorResponse
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /events/{eid} [put]
func (h *Handler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	eidStr := chi.URLParam(r, "eid")
	eid, err := uuid.Parse(eidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid event ID")
		return
	}

	var req dto.UpdateEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	event, err := h.queries.UpdateEvent(r.Context(), database.UpdateEventParams{
		Eid:         eid,
		Location:    toPgText(req.Location),
		EventTime:   toPgTimestamp(req.EventTime),
		Description: toPgText(req.Description),
	})
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, toEventResponse(event))
}

// DeleteEvent deletes an event
// @Summary      Delete event
// @Description  Deletes an event. Requires event_admin role.
// @Tags         events
// @Accept       json
// @Produce      json
// @Param        eid   path      string  true  "Event UUID"
// @Success      204
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /events/{eid} [delete]
func (h *Handler) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	eidStr := chi.URLParam(r, "eid")
	eid, err := uuid.Parse(eidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid event ID")
		return
	}

	if err := h.queries.DeleteEvent(r.Context(), eid); err != nil {
		h.handleDBError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListEventRegistrations lists registrations for an event
// @Summary      List event registrations
// @Description  Returns all users registered for an event
// @Tags         events
// @Accept       json
// @Produce      json
// @Param        eid   path      string  true  "Event UUID"
// @Success      200   {array}   dto.EventRegistrationResponse
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /events/{eid}/registrations [get]
func (h *Handler) ListEventRegistrations(w http.ResponseWriter, r *http.Request) {
	eidStr := chi.URLParam(r, "eid")
	eid, err := uuid.Parse(eidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid event ID")
		return
	}

	registrations, err := h.queries.GetEventRegistrations(r.Context(), eid)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	response := make([]dto.EventRegistrationResponse, len(registrations))
	for i, reg := range registrations {
		response[i] = dto.EventRegistrationResponse{
			UID:         reg.Uid,
			FirstName:   reg.FirstName,
			LastName:    reg.LastName,
			IsAttending: reg.IsAttending.Bool,
			IsAdmin:     reg.IsAdmin.Bool,
		}
	}

	h.respondJSON(w, http.StatusOK, response)
}

// RegisterForEvent registers a user for an event
// @Summary      Register for event
// @Description  Registers the current user or a specified user for an event
// @Tags         events
// @Accept       json
// @Produce      json
// @Param        eid   path      string                    true  "Event UUID"
// @Param        body  body      dto.RegisterEventRequest  true  "Registration data"
// @Success      201
// @Failure      400   {object}  ErrorResponse
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /events/{eid}/register [post]
func (h *Handler) RegisterForEvent(w http.ResponseWriter, r *http.Request) {
	eidStr := chi.URLParam(r, "eid")
	eid, err := uuid.Parse(eidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid event ID")
		return
	}

	var req dto.RegisterEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// TODO: Get user ID from context if not provided (for human auth)
	if req.UID == nil {
		h.respondError(w, http.StatusBadRequest, "uid is required")
		return
	}

	if err := h.queries.RegisterForEvent(r.Context(), database.RegisterForEventParams{
		Uid:         *req.UID,
		Eid:         eid,
		IsAttending: req.IsAttending,
	}); err != nil {
		h.handleDBError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// UnregisterFromEvent unregisters a user from an event
// @Summary      Unregister from event
// @Description  Removes registration for the current user or a specified user
// @Tags         events
// @Accept       json
// @Produce      json
// @Param        eid   path      string  true  "Event UUID"
// @Param        uid   query     string  false "User UUID (for bot auth)"
// @Success      204
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /events/{eid}/register [delete]
func (h *Handler) UnregisterFromEvent(w http.ResponseWriter, r *http.Request) {
	eidStr := chi.URLParam(r, "eid")
	eid, err := uuid.Parse(eidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid event ID")
		return
	}

	// Get UID from query param or context
	uidStr := r.URL.Query().Get("uid")
	if uidStr == "" {
		// TODO: Get from auth context
		h.respondError(w, http.StatusBadRequest, "uid is required")
		return
	}

	uid, err := uuid.Parse(uidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	if err := h.queries.UnregisterFromEvent(r.Context(), database.UnregisterFromEventParams{
		Uid: uid,
		Eid: eid,
	}); err != nil {
		h.handleDBError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListEventsByOrg lists events by organization
// @Summary      List events by organization
// @Description  Returns all events hosted by a specific organization
// @Tags         events
// @Accept       json
// @Produce      json
// @Param        oid     path      string  true   "Organization UUID"
// @Param        limit   query     int     false  "Limit (default 20)"
// @Param        offset  query     int     false  "Offset (default 0)"
// @Success      200     {array}   dto.EventResponse
// @Failure      404     {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /events/org/{oid} [get]
func (h *Handler) ListEventsByOrg(w http.ResponseWriter, r *http.Request) {
	oidStr := chi.URLParam(r, "oid")
	oid, err := uuid.Parse(oidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid organization ID")
		return
	}

	limit, offset := parsePagination(r)

	events, err := h.queries.ListEventsByOrg(r.Context(), database.ListEventsByOrgParams{
		Oid:    oid,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	response := make([]dto.EventResponse, len(events))
	for i, e := range events {
		response[i] = toEventResponse(e)
	}

	h.respondJSON(w, http.StatusOK, response)
}

// Helper functions
func toEventResponse(event database.Event) dto.EventResponse {
	return dto.EventResponse{
		EID:         event.Eid,
		Location:    fromPgText(event.Location),
		EventTime:   fromPgTimestamp(event.EventTime),
		Description: fromPgText(event.Description),
	}
}

func toPgTextFromString(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: s, Valid: true}
}

func toPgTimestamp(t *time.Time) pgtype.Timestamp {
	if t == nil {
		return pgtype.Timestamp{Valid: false}
	}
	return pgtype.Timestamp{Time: *t, Valid: true}
}
