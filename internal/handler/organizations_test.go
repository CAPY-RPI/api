package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/capyrpi/api/internal/config"
	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/database/mocks"
	"github.com/capyrpi/api/internal/dto"
	"github.com/capyrpi/api/internal/handler"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCreateOrganization(t *testing.T) {
	uid := uuid.New()
	oid := uuid.New()

	tests := []struct {
		name           string
		requestBody    interface{}
		setupMock      func(*mocks.Querier)
		setupContext   func() context.Context
		expectedStatus int
	}{
		{
			name: "Success",
			requestBody: dto.CreateOrganizationRequest{
				Name: "Test Org",
			},
			setupMock: func(m *mocks.Querier) {
				// Expect CreateOrganization
				m.On("CreateOrganization", mock.Anything, "Test Org").Return(database.Organization{
					Oid:  oid,
					Name: "Test Org",
				}, nil)

				// Expect AddOrgMember (admin)
				m.On("AddOrgMember", mock.Anything, mock.MatchedBy(func(arg database.AddOrgMemberParams) bool {
					return arg.Oid == oid && arg.Uid == uid && arg.IsAdmin.Bool
				})).Return(nil) // AddOrgMember returns error only (exec)
			},
			setupContext: func() context.Context {
				// Mock authenticated user
				ctx := context.Background()
				claims := &middleware.UserClaims{UserID: uid.String()}
				return context.WithValue(ctx, middleware.UserClaimsKey, claims)
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:        "InvalidBody",
			requestBody: "invalid-json",
			setupMock: func(m *mocks.Querier) {
			},
			setupContext: func() context.Context {
				return context.Background()
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQueries := mocks.NewQuerier(t)
			tt.setupMock(mockQueries)

			h := handler.New(mockQueries, &config.Config{})

			var body []byte
			if s, ok := tt.requestBody.(string); ok && s == "invalid-json" {
				body = []byte(s)
			} else {
				body, _ = json.Marshal(tt.requestBody)
			}

			req := httptest.NewRequest("POST", "/organizations", bytes.NewBuffer(body))
			req = req.WithContext(tt.setupContext())
			rr := httptest.NewRecorder()

			http.HandlerFunc(h.CreateOrganization).ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}

func TestListOrganizations(t *testing.T) {
	tests := []struct {
		name           string
		setupMock      func(*mocks.Querier)
		expectedStatus int
	}{
		{
			name: "Success",
			setupMock: func(m *mocks.Querier) {
				m.On("ListOrganizations", mock.Anything, mock.MatchedBy(func(arg database.ListOrganizationsParams) bool {
					return arg.Limit == 20 && arg.Offset == 0
				})).Return([]database.Organization{
					{Name: "Org 1"},
					{Name: "Org 2"},
				}, nil)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQueries := mocks.NewQuerier(t)
			tt.setupMock(mockQueries)

			h := handler.New(mockQueries, &config.Config{})

			req := httptest.NewRequest("GET", "/organizations", nil) // Defaults to limit 20 offset 0
			rr := httptest.NewRecorder()

			http.HandlerFunc(h.ListOrganizations).ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}

func TestAddOrgMemberAllowsSelfJoin(t *testing.T) {
	uid := uuid.New()
	oid := uuid.New()

	mockQueries := mocks.NewQuerier(t)
	mockQueries.On("AddOrgMember", mock.Anything, mock.MatchedBy(func(arg database.AddOrgMemberParams) bool {
		return arg.Oid == oid && arg.Uid == uid && arg.IsAdmin.Valid && !arg.IsAdmin.Bool
	})).Return(nil)

	h := handler.New(mockQueries, &config.Config{})

	body, _ := json.Marshal(dto.AddMemberRequest{
		UID:     uid,
		IsAdmin: false,
	})

	req := httptest.NewRequest("POST", "/organizations/"+oid.String()+"/members", bytes.NewBuffer(body))
	req = req.WithContext(context.WithValue(context.Background(), middleware.UserClaimsKey, &middleware.UserClaims{UserID: uid.String()}))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("oid", oid.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	http.HandlerFunc(h.AddOrgMember).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
}

func TestRemoveOrgMemberAuthorization(t *testing.T) {
	oid := uuid.New()
	selfUID := uuid.New()
	otherUID := uuid.New()

	tests := []struct {
		name           string
		authUID        uuid.UUID
		targetUID      uuid.UUID
		setupMock      func(*mocks.Querier)
		expectedStatus int
	}{
		{
			name:      "MemberCanRemoveSelf",
			authUID:   selfUID,
			targetUID: selfUID,
			setupMock: func(m *mocks.Querier) {
				m.On("RemoveOrgMember", mock.Anything, database.RemoveOrgMemberParams{
					Uid: selfUID,
					Oid: oid,
				}).Return(nil)
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:      "AdminCanRemoveOtherMember",
			authUID:   selfUID,
			targetUID: otherUID,
			setupMock: func(m *mocks.Querier) {
				m.On("IsOrgAdmin", mock.Anything, database.IsOrgAdminParams{
					Uid: selfUID,
					Oid: oid,
				}).Return(pgtype.Bool{Bool: true, Valid: true}, nil)
				m.On("RemoveOrgMember", mock.Anything, database.RemoveOrgMemberParams{
					Uid: otherUID,
					Oid: oid,
				}).Return(nil)
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:      "NonAdminCannotRemoveOtherMember",
			authUID:   selfUID,
			targetUID: otherUID,
			setupMock: func(m *mocks.Querier) {
				m.On("IsOrgAdmin", mock.Anything, database.IsOrgAdminParams{
					Uid: selfUID,
					Oid: oid,
				}).Return(pgtype.Bool{Bool: false, Valid: true}, nil)
			},
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQueries := mocks.NewQuerier(t)
			tt.setupMock(mockQueries)

			h := handler.New(mockQueries, &config.Config{})

			req := httptest.NewRequest(http.MethodDelete, "/organizations/"+oid.String()+"/members/"+tt.targetUID.String(), nil)
			req = req.WithContext(context.WithValue(context.Background(), middleware.UserClaimsKey, &middleware.UserClaims{UserID: tt.authUID.String()}))

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("oid", oid.String())
			rctx.URLParams.Add("uid", tt.targetUID.String())
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			rr := httptest.NewRecorder()
			http.HandlerFunc(h.RemoveOrgMember).ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}
