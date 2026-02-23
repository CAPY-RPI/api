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

func TestCreateLink(t *testing.T) {
	uid := uuid.New()
	oid := uuid.New()
	lid := uuid.New()

	tests := []struct {
		name           string
		requestBody    interface{}
		setupMock      func(*mocks.Querier)
		setupContext   func() context.Context
		expectedStatus int
	}{
		{
			name: "Success",
			requestBody: dto.CreateLinkRequest{
				EndpointURL: "promo-2024",
				DestURL:     "https://capyrpi.org/promo",
				OrgID:       oid,
			},
			setupMock: func(m *mocks.Querier) {
				m.On("IsOrgAdmin", mock.Anything, database.IsOrgAdminParams{
					Uid: uid,
					Oid: oid,
				}).Return(pgtype.Bool{Bool: true, Valid: true}, nil)

				m.On("CreateLink", mock.Anything, database.CreateLinkParams{
					EndpointUrl: "promo-2024",
					DestUrl:     "https://capyrpi.org/promo",
					Oid:         oid,
				}).Return(database.Link{
					Lid:         lid,
					EndpointUrl: "promo-2024",
					DestUrl:     "https://capyrpi.org/promo",
					Oid:         oid,
				}, nil)
			},
			setupContext: func() context.Context {
				ctx := context.Background()
				claims := &middleware.UserClaims{UserID: uid.String()}
				return context.WithValue(ctx, middleware.UserClaimsKey, claims)
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "Forbidden - Not Admin",
			requestBody: dto.CreateLinkRequest{
				EndpointURL: "promo-2024",
				DestURL:     "https://capyrpi.org/promo",
				OrgID:       oid,
			},
			setupMock: func(m *mocks.Querier) {
				m.On("IsOrgAdmin", mock.Anything, database.IsOrgAdminParams{
					Uid: uid,
					Oid: oid,
				}).Return(pgtype.Bool{Bool: false, Valid: true}, nil)
			},
			setupContext: func() context.Context {
				ctx := context.Background()
				claims := &middleware.UserClaims{UserID: uid.String()}
				return context.WithValue(ctx, middleware.UserClaimsKey, claims)
			},
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQueries := mocks.NewQuerier(t)
			if tt.setupMock != nil {
				tt.setupMock(mockQueries)
			}

			h := handler.New(mockQueries, &config.Config{})

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/links", bytes.NewBuffer(body))
			req = req.WithContext(tt.setupContext())
			rr := httptest.NewRecorder()

			http.HandlerFunc(h.CreateLink).ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			if tt.expectedStatus == http.StatusCreated {
				var res dto.LinkResponse
				err := json.Unmarshal(rr.Body.Bytes(), &res)
				assert.NoError(t, err)
				assert.Equal(t, lid, res.LID)
				assert.Equal(t, "promo-2024", res.EndpointURL)
			}
		})
	}
}

func TestResolveLink(t *testing.T) {
	lid := uuid.New()
	endpoint := "my-link"
	dest := "https://example.com"

	tests := []struct {
		name           string
		endpointParam  string
		setupMock      func(*mocks.Querier)
		setupContext   func() context.Context
		expectedStatus int
		expectedLoc    string
	}{
		{
			name:          "Success Redirect",
			endpointParam: endpoint,
			setupMock: func(m *mocks.Querier) {
				m.On("GetLinkByEndpointURL", mock.Anything, endpoint).Return(database.Link{
					Lid:         lid,
					EndpointUrl: endpoint,
					DestUrl:     dest,
				}, nil)

				// We don't strictly mock the background Context visit log here
				// since it happens in a goroutine and is hard to sync without sleep or waitgroups,
				// but let's mock it to avoid panic if the mock is strict.
				m.On("LogLinkVisit", mock.Anything, mock.MatchedBy(func(p database.LogLinkVisitParams) bool {
					return p.Lid == lid
				})).Return(database.LinkVisit{}, nil).Maybe()
			},
			setupContext: func() context.Context {
				return context.Background()
			},
			expectedStatus: http.StatusFound, // 302
			expectedLoc:    dest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQueries := mocks.NewQuerier(t)
			tt.setupMock(mockQueries)

			h := handler.New(mockQueries, &config.Config{})

			req := httptest.NewRequest("GET", "/r/"+tt.endpointParam, nil)
			req = req.WithContext(tt.setupContext())

			// Setup chi router context so chi.URLParam works
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("endpoint_url", tt.endpointParam)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			rr := httptest.NewRecorder()

			http.HandlerFunc(h.ResolveLink).ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			if tt.expectedStatus == http.StatusFound {
				assert.Equal(t, tt.expectedLoc, rr.Header().Get("Location"))
			}
		})
	}
}
