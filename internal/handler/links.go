package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/dto"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/yeqown/go-qrcode/v2"
	"github.com/yeqown/go-qrcode/writer/standard"
)

// CreateLink creates a new dynamic link
// @Summary      Create link
// @Description  Creates a new dynamic link for an organization. Requires org_admin role.
// @Tags         links
// @Accept       json
// @Produce      json
// @Param        body  body      dto.CreateLinkRequest  true  "Link data"
// @Success      201   {object}  dto.LinkResponse
// @Failure      400   {object}  ErrorResponse
// @Failure      403   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /links [post]
func (h *Handler) CreateLink(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.OrgID == uuid.Nil {
		h.respondError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	// Check if user is admin of the org
	claims, ok := middleware.GetUserClaims(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	uid, _ := uuid.Parse(claims.UserID)

	isAdmin, err := h.queries.IsOrgAdmin(r.Context(), database.IsOrgAdminParams{
		Uid: uid,
		Oid: req.OrgID,
	})
	if err != nil {
		h.handleDBError(w, err)
		return
	}
	if !isAdmin.Bool {
		h.respondError(w, http.StatusForbidden, "Only org admins can create links")
		return
	}

	link, err := h.queries.CreateLink(r.Context(), database.CreateLinkParams{
		EndpointUrl: req.EndpointURL,
		DestUrl:     req.DestURL,
		Oid:         req.OrgID,
	})
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	h.respondJSON(w, http.StatusCreated, toLinkResponse(link))
}

// UpdateLink updates an existing dynamic link
// @Summary      Update link
// @Description  Updates a dynamic link's destination or endpoint URL. Requires org_admin role.
// @Tags         links
// @Accept       json
// @Produce      json
// @Param        lid   path      string                 true  "Link UUID"
// @Param        body  body      dto.UpdateLinkRequest  true  "Update data"
// @Success      200   {object}  dto.LinkResponse
// @Failure      400   {object}  ErrorResponse
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /links/{lid} [put]
func (h *Handler) UpdateLink(w http.ResponseWriter, r *http.Request) {
	lidStr := chi.URLParam(r, "lid")
	lid, err := uuid.Parse(lidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid link ID")
		return
	}

	var req dto.UpdateLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	link, err := h.queries.GetLinkByLID(r.Context(), lid)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	// Permission check
	claims, ok := middleware.GetUserClaims(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	uid, _ := uuid.Parse(claims.UserID)

	isAdmin, err := h.queries.IsOrgAdmin(r.Context(), database.IsOrgAdminParams{
		Uid: uid,
		Oid: link.Oid,
	})
	if err != nil {
		h.handleDBError(w, err)
		return
	}
	if !isAdmin.Bool {
		h.respondError(w, http.StatusForbidden, "Only org admins can update links")
		return
	}

	updatedLink, err := h.queries.UpdateLink(r.Context(), database.UpdateLinkParams{
		Lid:         lid,
		EndpointUrl: toPgText(req.EndpointURL),
		DestUrl:     toPgText(req.DestURL),
	})
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, toLinkResponse(updatedLink))
}

// ResolveLink resolves a dynamic link, logs a visit, and redirects
// @Summary      Resolve link
// @Description  Redirects to the destination URL and logs a visit
// @Tags         links
// @Param        endpoint_url  path  string  true  "Dynamic link endpoint URL"
// @Success      302
// @Failure      404  {object}  ErrorResponse
// @Router       /r/{endpoint_url} [get]
func (h *Handler) ResolveLink(w http.ResponseWriter, r *http.Request) {
	endpointURL := chi.URLParam(r, "endpoint_url")

	link, err := h.queries.GetLinkByEndpointURL(r.Context(), endpointURL)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	// Log visit asynchronously to not block the redirect
	go func() {
		// Create a detached context so the DB query isn't cancelled when the HTTP request ends
		ctx := context.Background()

		var uid pgtype.UUID
		claims, ok := middleware.GetUserClaims(r.Context())
		if ok {
			parsedUID, err := uuid.Parse(claims.UserID)
			if err == nil {
				uid = pgtype.UUID{Bytes: parsedUID, Valid: true}
			}
		}

		_, err := h.queries.LogLinkVisit(ctx, database.LogLinkVisitParams{
			Lid: link.Lid,
			Uid: uid,
		})
		if err != nil {
			slog.Error("failed to log link visit", "lid", link.Lid, "error", err)
		}
	}()

	http.Redirect(w, r, link.DestUrl, http.StatusFound)
}

// ListOrgLinks lists all links for an organization
// @Summary      List org links
// @Description  Returns all dynamic links owned by an organization
// @Tags         links
// @Accept       json
// @Produce      json
// @Param        oid  path      string  true  "Organization UUID"
// @Success      200  {array}   dto.LinkResponse
// @Failure      404  {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /organizations/{oid}/links [get]
func (h *Handler) ListOrgLinks(w http.ResponseWriter, r *http.Request) {
	oidStr := chi.URLParam(r, "oid")
	oid, err := uuid.Parse(oidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid organization ID")
		return
	}

	links, err := h.queries.ListLinksByOrg(r.Context(), oid)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	response := make([]dto.LinkResponse, len(links))
	for i, l := range links {
		response[i] = toLinkResponse(l)
	}

	h.respondJSON(w, http.StatusOK, response)
}

// GetTotalVisits returns the total number of visits for a link
// @Summary      Get visit count
// @Description  Returns the total number of visits logged for a link
// @Tags         links
// @Produce      json
// @Param        lid  path      string  true  "Link UUID"
// @Success      200  {object}  dto.VisitCountResponse
// @Failure      404  {object}  ErrorResponse
// @Security     CookieAuth
// @Router       /links/{lid}/visits [get]
func (h *Handler) GetTotalVisits(w http.ResponseWriter, r *http.Request) {
	lidStr := chi.URLParam(r, "lid")
	lid, err := uuid.Parse(lidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid link ID")
		return
	}

	count, err := h.queries.GetTotalVisits(r.Context(), lid)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, dto.VisitCountResponse{
		LID:   lid,
		Count: count,
	})
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

// GetQRCode generates a QR code for a link's destination URL
// @Summary      Get QR code
// @Description  Generates and returns a QR code image for the link's destination URL
// @Tags         links
// @Produce      image/png
// @Param        lid  path  string  true  "Link UUID"
// @Success      200  {file}  image/png
// @Failure      404  {object}  ErrorResponse
// @Router       /links/{lid}/qrcode [get]
func (h *Handler) GetQRCode(w http.ResponseWriter, r *http.Request) {
	lidStr := chi.URLParam(r, "lid")
	lid, err := uuid.Parse(lidStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid link ID")
		return
	}

	link, err := h.queries.GetLinkByLID(r.Context(), lid)
	if err != nil {
		h.handleDBError(w, err)
		return
	}

	qrc, err := qrcode.New(link.EndpointUrl)
	if err != nil {
		slog.Error("failed to generate QR code", "error", err)
		h.respondError(w, http.StatusInternalServerError, "Failed to generate QR code")
		return
	}

	w.Header().Set("Content-Type", "image/png")
	wr := standard.NewWithWriter(nopWriteCloser{w})
	if err != nil {
		slog.Error("failed to create standard writer for QR code", "error", err)
		h.respondError(w, http.StatusInternalServerError, "Failed to create QR code writer")
		return
	}

	if err = qrc.Save(wr); err != nil {
		slog.Error("failed to write QR code to response", "error", err)
		// Header already set, but we can't do much now if it partially wrote
	}
}

// Helper functions for link conversion
func toLinkResponse(link database.Link) dto.LinkResponse {
	return dto.LinkResponse{
		LID:         link.Lid,
		EndpointURL: link.EndpointUrl,
		DestURL:     link.DestUrl,
		OrgID:       link.Oid,
		CreatedAt:   fromPgTimestamp(link.CreatedAt),
	}
}
