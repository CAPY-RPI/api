package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/dto"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListOrganizations lists all organizations
// @Summary      List organizations
// @Description  Returns a paginated list of all organizations
// @Tags         organizations
// @Accept       json
// @Produce      json
// @Param        limit   query     int  false  "Limit (default 20, max 100)"
// @Param        offset  query     int  false  "Offset (default 0)"
// @Success      200     {array}   dto.OrganizationResponse
// @Router       /organizations [get]
func (h *Handler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)

	orgs, err := h.queries.ListOrganizations(r.Context(), database.ListOrganizationsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	response := make([]dto.OrganizationResponse, len(orgs))
	for i, org := range orgs {
		response[i] = toListOrgResponse(org)
	}

	h.respondJSON(w, http.StatusOK, response)
}

// CreateOrganization creates a new organization
// @Summary      Create organization
// @Description  Creates a new organization. The creator becomes an admin.
// @Tags         organizations
// @Accept       json
// @Produce      json
// @Param        body  body      dto.HumanCreateOrganizationRequest  true  "Organization data"
// @Success      201   {object}  dto.OrganizationResponse
// @Failure      400   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /organizations [post]
func (h *Handler) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		h.respondError(w, http.StatusBadRequest, "Name is required")
		return
	}

	if middleware.GetAuthType(r.Context()) == "bot" {
		if req.GuildID == nil {
			h.respondError(w, http.StatusBadRequest, "Guild ID is required")
			return
		}

		org, err := h.createBotOrganization(r.Context(), req.Name, *req.GuildID)
		if err != nil {
			h.handleDBError(w, err)
			return
		}

		h.respondJSON(w, http.StatusCreated, toOrgResponse(org))
		return
	}

	org, err := h.queries.CreateOrganization(r.Context(), req.Name)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	// Add creator as admin member
	claims, ok := middleware.GetUserClaims(r.Context())
	if ok {
		uid, err := uuid.Parse(claims.UserID)
		if err == nil {
			err = h.queries.AddOrgMember(r.Context(), database.AddOrgMemberParams{
				Oid:     org.Oid,
				Uid:     uid,
				IsAdmin: pgtype.Bool{Bool: true, Valid: true},
			})
			if err != nil {
				// Log error but don't fail the request as the org was created
				slog.Error("failed to add creator as org member", "error", err)
			}
		}
	}

	h.respondJSON(w, http.StatusCreated, toOrgResponse(org))
}

// CreateBotOrganization creates a new organization for a Discord guild.
// @Summary      Create bot organization
// @Description  Creates a new organization for the authenticated bot and links it to the provided Discord guild ID.
// @Tags         organizations
// @Accept       json
// @Produce      json
// @Param        body  body      dto.BotCreateOrganizationRequest  true  "Bot organization data"
// @Success      201   {object}  dto.OrganizationResponse
// @Failure      400   {object}  ErrorResponse
// @Failure      409   {object}  ErrorResponse
// @Security     BotToken
// @Router       /bot/organizations [post]
func (h *Handler) CreateBotOrganization(w http.ResponseWriter, r *http.Request) {
	h.CreateOrganization(w, r)
}

func (h *Handler) createBotOrganization(ctx context.Context, name string, guildID int64) (database.Organization, error) {
	if h.txBeginner == nil {
		org, err := h.queries.CreateOrganization(ctx, name)
		if err != nil {
			return database.Organization{}, err
		}

		if err := h.queries.CreateOrgDiscord(ctx, database.CreateOrgDiscordParams{
			Oid:     org.Oid,
			GuildID: guildID,
		}); err != nil {
			return database.Organization{}, err
		}

		return org, nil
	}

	tx, err := h.txBeginner.Begin(ctx)
	if err != nil {
		return database.Organization{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	q := database.New(tx)

	org, err := q.CreateOrganization(ctx, name)
	if err != nil {
		return database.Organization{}, err
	}

	if err := q.CreateOrgDiscord(ctx, database.CreateOrgDiscordParams{
		Oid:     org.Oid,
		GuildID: guildID,
	}); err != nil {
		return database.Organization{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return database.Organization{}, err
	}

	return org, nil
}

// GetOrganization gets an organization by ID
// @Summary      Get organization
// @Description  Returns organization details
// @Tags         organizations
// @Accept       json
// @Produce      json
// @Param        oid   path      string  true  "Organization UUID"
// @Success      200   {object}  dto.OrganizationResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /organizations/{oid} [get]
func (h *Handler) GetOrganization(w http.ResponseWriter, r *http.Request) {
	oidStr := chi.URLParam(r, "oid")
	oid, err := uuid.Parse(oidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid organization ID")
		return
	}

	org, err := h.queries.GetOrganizationByID(r.Context(), oid)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, toOrgResponse(org))
}

// GetOrganizationByGuildID gets a bot-managed organization by Discord guild ID.
// @Summary      Get organization by guild ID
// @Description  Returns organization details for the provided Discord guild ID.
// @Tags         organizations
// @Accept       json
// @Produce      json
// @Param        guild_id  path      int64  true  "Discord guild ID"
// @Success      200       {object}  dto.BotOrganizationResponse
// @Failure      400       {object}  ErrorResponse
// @Failure      404       {object}  ErrorResponse
// @Security     BotToken
// @Router       /bot/organizations/guilds/{guild_id} [get]
func (h *Handler) GetOrganizationByGuildID(w http.ResponseWriter, r *http.Request) {
	guildID, err := parseInt64PathParam(chi.URLParam(r, "guild_id"))
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid guild ID")
		return
	}

	org, err := h.queries.GetOrganizationByGuildID(r.Context(), guildID)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, toBotOrgResponse(org))
}

// UpdateOrganization updates an organization
// @Summary      Update organization
// @Description  Updates organization details. Requires org_admin role.
// @Tags         organizations
// @Accept       json
// @Produce      json
// @Param        oid   path      string                        true  "Organization UUID"
// @Param        body  body      dto.UpdateOrganizationRequest true  "Organization update data"
// @Success      200   {object}  dto.OrganizationResponse
// @Failure      400   {object}  ErrorResponse
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /organizations/{oid} [put]
func (h *Handler) UpdateOrganization(w http.ResponseWriter, r *http.Request) {
	oidStr := chi.URLParam(r, "oid")
	oid, err := uuid.Parse(oidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid organization ID")
		return
	}

	if _, ok := h.requireOrgAdmin(w, r, oid); !ok {
		return
	}

	var req dto.UpdateOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	org, err := h.queries.UpdateOrganization(r.Context(), database.UpdateOrganizationParams{
		Oid:  oid,
		Name: toPgText(req.Name),
	})
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, toOrgResponse(org))
}

// DeleteOrganization deletes an organization
// @Summary      Delete organization
// @Description  Deletes an organization. Requires org_admin role.
// @Tags         organizations
// @Accept       json
// @Produce      json
// @Param        oid   path      string  true  "Organization UUID"
// @Success      204
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /organizations/{oid} [delete]
func (h *Handler) DeleteOrganization(w http.ResponseWriter, r *http.Request) {
	oidStr := chi.URLParam(r, "oid")
	oid, err := uuid.Parse(oidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid organization ID")
		return
	}

	if _, ok := h.requireOrgAdmin(w, r, oid); !ok {
		return
	}

	if err := h.queries.DeleteOrganization(r.Context(), oid); err != nil {
		h.handleDBError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListOrgMembers lists members of an organization
// @Summary      List organization members
// @Description  Returns all members of an organization
// @Tags         organizations
// @Accept       json
// @Produce      json
// @Param        oid   path      string  true  "Organization UUID"
// @Success      200   {array}   dto.OrgMemberResponse
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /organizations/{oid}/members [get]
func (h *Handler) ListOrgMembers(w http.ResponseWriter, r *http.Request) {
	oidStr := chi.URLParam(r, "oid")
	oid, err := uuid.Parse(oidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid organization ID")
		return
	}

	members, err := h.queries.GetOrgMembers(r.Context(), oid)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	response := make([]dto.OrgMemberResponse, len(members))
	for i, m := range members {
		response[i] = dto.OrgMemberResponse{
			UID:       m.Uid,
			FirstName: m.FirstName,
			LastName:  m.LastName,
			Email:     fromPgText(m.PersonalEmail),
			IsAdmin:   m.IsAdmin.Bool,
		}
	}

	h.respondJSON(w, http.StatusOK, response)
}

// AddOrgMember adds a member to an organization
// @Summary      Add organization member
// @Description  Adds a user as a member of an organization
// @Tags         organizations
// @Accept       json
// @Produce      json
// @Param        oid   path      string              true  "Organization UUID"
// @Param        body  body      dto.AddMemberRequest true  "Member data"
// @Success      201
// @Failure      400   {object}  ErrorResponse
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /organizations/{oid}/members [post]
func (h *Handler) AddOrgMember(w http.ResponseWriter, r *http.Request) {
	oidStr := chi.URLParam(r, "oid")
	oid, err := uuid.Parse(oidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid organization ID")
		return
	}

	var req dto.AddMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	switch middleware.GetAuthType(r.Context()) {
	case "bot":
		// Bots retain full access to add members on behalf of users.
	default:
		authenticatedUID, _, ok := h.requireAuthenticatedUser(w, r)
		if !ok {
			return
		}

		// Allow self-join through the existing members endpoint, but never allow
		// a human user to self-promote to admin.
		if req.UID != authenticatedUID || req.IsAdmin {
			if _, ok := h.requireOrgAdmin(w, r, oid); !ok {
				return
			}
		}
	}

	if err := h.queries.AddOrgMember(r.Context(), database.AddOrgMemberParams{
		Uid:     req.UID,
		Oid:     oid,
		IsAdmin: pgtype.Bool{Bool: req.IsAdmin, Valid: true},
	}); err != nil {
		h.handleDBError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// RemoveOrgMember removes a member from an organization
// @Summary      Remove organization member
// @Description  Removes a user from an organization
// @Tags         organizations
// @Accept       json
// @Produce      json
// @Param        oid   path      string  true  "Organization UUID"
// @Param        uid   path      string  true  "User UUID"
// @Success      204
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /organizations/{oid}/members/{uid} [delete]
func (h *Handler) RemoveOrgMember(w http.ResponseWriter, r *http.Request) {
	oidStr := chi.URLParam(r, "oid")
	oid, err := uuid.Parse(oidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid organization ID")
		return
	}

	uidStr := chi.URLParam(r, "uid")
	uid, err := uuid.Parse(uidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	switch middleware.GetAuthType(r.Context()) {
	case "bot":
		// Bots retain full access to remove members on behalf of users.
	default:
		authenticatedUID, _, ok := h.requireAuthenticatedUser(w, r)
		if !ok {
			return
		}

		if uid != authenticatedUID {
			if _, ok := h.requireOrgAdmin(w, r, oid); !ok {
				return
			}
		}
	}

	if err := h.queries.RemoveOrgMember(r.Context(), database.RemoveOrgMemberParams{
		Uid: uid,
		Oid: oid,
	}); err != nil {
		h.handleDBError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListOrgEvents lists events hosted by an organization
// @Summary      List organization events
// @Description  Returns all events hosted by an organization
// @Tags         organizations
// @Accept       json
// @Produce      json
// @Param        oid     path      string  true   "Organization UUID"
// @Param        limit   query     int     false  "Limit (default 20)"
// @Param        offset  query     int     false  "Offset (default 0)"
// @Success      200     {array}   dto.EventResponse
// @Failure      404     {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /organizations/{oid}/events [get]
func (h *Handler) ListOrgEvents(w http.ResponseWriter, r *http.Request) {
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
func toOrgResponse(org database.Organization) dto.OrganizationResponse {
	return dto.OrganizationResponse{
		OID:          org.Oid,
		Name:         org.Name,
		DateCreated:  fromPgDate(org.DateCreated),
		DateModified: fromPgDate(org.DateModified),
	}
}

func toListOrgResponse(org database.ListOrganizationsRow) dto.OrganizationResponse {
	return dto.OrganizationResponse{
		OID:          org.Oid,
		Name:         org.Name,
		GuildID:      fromPgInt8(org.GuildID),
		DateCreated:  fromPgDate(org.DateCreated),
		DateModified: fromPgDate(org.DateModified),
	}
}

func toBotOrgResponse(org database.GetOrganizationByGuildIDRow) dto.BotOrganizationResponse {
	return dto.BotOrganizationResponse{
		OID:          org.Oid,
		Name:         org.Name,
		GuildID:      org.GuildID,
		DateCreated:  fromPgDate(org.DateCreated),
		DateModified: fromPgDate(org.DateModified),
	}
}
