package handler

import (
	"net/http"

	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) requireAuthenticatedUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, *middleware.UserClaims, bool) {
	claims, ok := middleware.GetUserClaims(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "Not authenticated")
		return uuid.Nil, nil, false
	}

	uid, err := uuid.Parse(claims.UserID)
	if err != nil {
		h.respondError(w, http.StatusUnauthorized, "Invalid user ID in token")
		return uuid.Nil, nil, false
	}

	return uid, claims, true
}

func (h *Handler) requireAuthenticatedUserRecord(w http.ResponseWriter, r *http.Request) (database.User, *middleware.UserClaims, bool) {
	uid, claims, ok := h.requireAuthenticatedUser(w, r)
	if !ok {
		return database.User{}, nil, false
	}

	user, err := h.queries.GetUserByID(r.Context(), uid)
	if err != nil {
		h.handleDBError(w, err)
		return database.User{}, nil, false
	}

	return user, claims, true
}

func (h *Handler) requireDev(w http.ResponseWriter, r *http.Request) bool {
	_, ok := h.requireDevClaims(w, r)
	return ok
}

func (h *Handler) requireDevClaims(w http.ResponseWriter, r *http.Request) (*middleware.UserClaims, bool) {
	user, claims, ok := h.requireAuthenticatedUserRecord(w, r)
	if !ok {
		return nil, false
	}

	if !user.Role.Valid || user.Role.UserRole != database.UserRoleDev {
		h.respondError(w, http.StatusForbidden, "Dev role required")
		return nil, false
	}

	return claims, true
}

func (h *Handler) requireSelfOrDev(w http.ResponseWriter, r *http.Request, targetUID uuid.UUID) (database.User, bool) {
	user, _, ok := h.requireAuthenticatedUserRecord(w, r)
	if !ok {
		return database.User{}, false
	}

	if user.Uid == targetUID {
		return user, true
	}

	if user.Role.Valid && user.Role.UserRole == database.UserRoleDev {
		return user, true
	}

	h.respondError(w, http.StatusForbidden, "Insufficient permissions")
	return database.User{}, false
}

func (h *Handler) requireOrgAdmin(w http.ResponseWriter, r *http.Request, oid uuid.UUID) (uuid.UUID, bool) {
	if middleware.GetAuthType(r.Context()) == "bot" {
		return uuid.Nil, true
	}

	uid, _, ok := h.requireAuthenticatedUser(w, r)
	if !ok {
		return uuid.Nil, false
	}

	isAdmin, err := h.queries.IsOrgAdmin(r.Context(), database.IsOrgAdminParams{
		Uid: uid,
		Oid: oid,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			h.respondError(w, http.StatusForbidden, "Organization admin required")
			return uuid.Nil, false
		}
		h.handleDBError(w, err)
		return uuid.Nil, false
	}

	if !isAdmin.Valid || !isAdmin.Bool {
		h.respondError(w, http.StatusForbidden, "Organization admin required")
		return uuid.Nil, false
	}

	return uid, true
}

func (h *Handler) requireEventAdmin(w http.ResponseWriter, r *http.Request, eid uuid.UUID) (uuid.UUID, bool) {
	if middleware.GetAuthType(r.Context()) == "bot" {
		return uuid.Nil, true
	}

	uid, _, ok := h.requireAuthenticatedUser(w, r)
	if !ok {
		return uuid.Nil, false
	}

	isAdmin, err := h.queries.IsEventAdmin(r.Context(), database.IsEventAdminParams{
		Uid: uid,
		Eid: eid,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			h.respondError(w, http.StatusForbidden, "Event admin required")
			return uuid.Nil, false
		}
		h.handleDBError(w, err)
		return uuid.Nil, false
	}

	if !isAdmin.Valid || !isAdmin.Bool {
		h.respondError(w, http.StatusForbidden, "Event admin required")
		return uuid.Nil, false
	}

	return uid, true
}
