package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRegisterForEvent(t *testing.T) {
	eid := uuid.New()
	userUID := uuid.New()
	otherUID := uuid.New()

	tests := []struct {
		name           string
		authType       string
		claims         *middleware.UserClaims
		requestBody    dto.RegisterEventRequest
		mockSetup      func(*mocks.Querier)
		expectedStatus int
	}{
		{
			name:     "Human_Success_ExplicitUID",
			authType: "human",
			claims: &middleware.UserClaims{
				UserID: userUID.String(),
			},
			requestBody: dto.RegisterEventRequest{
				UID:         &userUID,
				IsAttending: true,
			},
			mockSetup: func(m *mocks.Querier) {
				m.On("RegisterForEvent", mock.Anything, mock.MatchedBy(func(arg database.RegisterForEventParams) bool {
					return arg.Uid == userUID && arg.Eid == eid
				})).Return(nil)
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:     "Human_Success_ImplicitUID",
			authType: "human",
			claims: &middleware.UserClaims{
				UserID: userUID.String(),
			},
			requestBody: dto.RegisterEventRequest{
				UID:         nil,
				IsAttending: true,
			},
			mockSetup: func(m *mocks.Querier) {
				m.On("RegisterForEvent", mock.Anything, mock.MatchedBy(func(arg database.RegisterForEventParams) bool {
					return arg.Uid == userUID && arg.Eid == eid
				})).Return(nil)
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:     "Human_Forbidden_MismatchUID",
			authType: "human",
			claims: &middleware.UserClaims{
				UserID: userUID.String(),
			},
			requestBody: dto.RegisterEventRequest{
				UID:         &otherUID,
				IsAttending: true,
			},
			mockSetup: func(m *mocks.Querier) {
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:     "Bot_Success",
			authType: "bot",
			claims:   nil,
			requestBody: dto.RegisterEventRequest{
				UID:         &otherUID,
				IsAttending: true,
			},
			mockSetup: func(m *mocks.Querier) {
				m.On("RegisterForEvent", mock.Anything, mock.MatchedBy(func(arg database.RegisterForEventParams) bool {
					return arg.Uid == otherUID && arg.Eid == eid
				})).Return(nil)
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:     "Bot_MissingUID",
			authType: "bot",
			claims:   nil,
			requestBody: dto.RegisterEventRequest{
				UID:         nil,
				IsAttending: true,
			},
			mockSetup: func(m *mocks.Querier) {
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQueries := mocks.NewQuerier(t)
			if tt.mockSetup != nil {
				tt.mockSetup(mockQueries)
			}

			h := handler.New(mockQueries, &config.Config{})
			r := chi.NewRouter()
			r.Post("/events/{eid}/register", h.RegisterForEvent)

			body, _ := json.Marshal(tt.requestBody)
			req, _ := http.NewRequest("POST", fmt.Sprintf("/events/%s/register", eid), bytes.NewBuffer(body))

			ctx := req.Context()
			if tt.authType != "" {
				ctx = context.WithValue(ctx, middleware.AuthTypeKey, tt.authType)
			}
			if tt.claims != nil {
				ctx = context.WithValue(ctx, middleware.UserClaimsKey, tt.claims)
			}
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}

func TestUnregisterFromEvent(t *testing.T) {
	eid := uuid.New()
	userUID := uuid.New()
	otherUID := uuid.New()

	tests := []struct {
		name           string
		authType       string
		claims         *middleware.UserClaims
		uidParam       string
		mockSetup      func(*mocks.Querier)
		expectedStatus int
	}{
		{
			name:     "Human_Success_ExplicitUID",
			authType: "human",
			claims: &middleware.UserClaims{
				UserID: userUID.String(),
			},
			uidParam: userUID.String(),
			mockSetup: func(m *mocks.Querier) {
				m.On("UnregisterFromEvent", mock.Anything, mock.MatchedBy(func(arg database.UnregisterFromEventParams) bool {
					return arg.Uid == userUID && arg.Eid == eid
				})).Return(nil)
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:     "Human_Success_ImplicitUID",
			authType: "human",
			claims: &middleware.UserClaims{
				UserID: userUID.String(),
			},
			uidParam: "",
			mockSetup: func(m *mocks.Querier) {
				m.On("UnregisterFromEvent", mock.Anything, mock.MatchedBy(func(arg database.UnregisterFromEventParams) bool {
					return arg.Uid == userUID && arg.Eid == eid
				})).Return(nil)
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:     "Human_Forbidden_MismatchUID",
			authType: "human",
			claims: &middleware.UserClaims{
				UserID: userUID.String(),
			},
			uidParam: otherUID.String(),
			mockSetup: func(m *mocks.Querier) {
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:     "Bot_Success",
			authType: "bot",
			claims:   nil,
			uidParam: otherUID.String(),
			mockSetup: func(m *mocks.Querier) {
				m.On("UnregisterFromEvent", mock.Anything, mock.MatchedBy(func(arg database.UnregisterFromEventParams) bool {
					return arg.Uid == otherUID && arg.Eid == eid
				})).Return(nil)
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:     "Bot_MissingUID",
			authType: "bot",
			claims:   nil,
			uidParam: "",
			mockSetup: func(m *mocks.Querier) {
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQueries := mocks.NewQuerier(t)
			if tt.mockSetup != nil {
				tt.mockSetup(mockQueries)
			}

			h := handler.New(mockQueries, &config.Config{})
			r := chi.NewRouter()
			r.Delete("/events/{eid}/register", h.UnregisterFromEvent)

			url := fmt.Sprintf("/events/%s/register", eid)
			if tt.uidParam != "" {
				url += "?uid=" + tt.uidParam
			}
			req, _ := http.NewRequest("DELETE", url, nil)

			ctx := req.Context()
			if tt.authType != "" {
				ctx = context.WithValue(ctx, middleware.AuthTypeKey, tt.authType)
			}
			if tt.claims != nil {
				ctx = context.WithValue(ctx, middleware.UserClaimsKey, tt.claims)
			}
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}
