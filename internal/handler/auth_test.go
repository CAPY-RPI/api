package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/capyrpi/api/internal/config"
	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/database/mocks"
	"github.com/capyrpi/api/internal/handler"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGetMe(t *testing.T) {
	uid := uuid.New()

	tests := []struct {
		name           string
		claims         *middleware.UserClaims
		setupMock      func(*mocks.Querier)
		expectedStatus int
	}{
		{
			name: "Success",
			claims: &middleware.UserClaims{
				UserID: uid.String(),
				Role:   string(database.UserRoleStudent),
			},
			setupMock: func(m *mocks.Querier) {
				m.On("GetUserByID", mock.Anything, uid).Return(database.User{
					Uid:       uid,
					FirstName: "Test",
					LastName:  "User",
					Role:      database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
				}, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Unauthorized_NoClaims",
			claims:         nil,
			setupMock:      func(m *mocks.Querier) {},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQueries := mocks.NewQuerier(t)
			tt.setupMock(mockQueries)

			h := handler.New(mockQueries, &config.Config{})

			req := httptest.NewRequest("GET", "/auth/me", nil)
			if tt.claims != nil {
				ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, tt.claims)
				req = req.WithContext(ctx)
			}

			rr := httptest.NewRecorder()
			http.HandlerFunc(h.GetMe).ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}

func TestBotToken_RoleChecks(t *testing.T) {
	uid := uuid.New()

	tests := []struct {
		name           string
		handlerFunc    func(*handler.Handler) http.HandlerFunc
		method         string
		path           string
		role           database.UserRole
		setupMock      func(*mocks.Querier)
		expectedStatus int
	}{
		{
			name:        "ListBotTokens_Dev_Success",
			handlerFunc: func(h *handler.Handler) http.HandlerFunc { return h.ListBotTokens },
			method:      "GET",
			path:        "/bot/tokens",
			role:        database.UserRoleDev,
			setupMock: func(m *mocks.Querier) {
				m.On("ListBotTokens", mock.Anything).Return([]database.ListBotTokensRow{}, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "ListBotTokens_Faculty_Forbidden",
			handlerFunc:    func(h *handler.Handler) http.HandlerFunc { return h.ListBotTokens },
			method:         "GET",
			path:           "/bot/tokens",
			role:           database.UserRoleFaculty,
			setupMock:      func(m *mocks.Querier) {},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "ListBotTokens_Student_Forbidden",
			handlerFunc:    func(h *handler.Handler) http.HandlerFunc { return h.ListBotTokens },
			method:         "GET",
			path:           "/bot/tokens",
			role:           database.UserRoleStudent,
			setupMock:      func(m *mocks.Querier) {},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:        "CreateBotToken_Dev_Success",
			handlerFunc: func(h *handler.Handler) http.HandlerFunc { return h.CreateBotToken },
			method:      "POST",
			path:        "/bot/tokens",
			role:        database.UserRoleDev,
			setupMock: func(m *mocks.Querier) {
				m.On("CreateBotToken", mock.Anything, mock.Anything).Return(database.BotToken{
					TokenID:   uuid.New(),
					Name:      "Bot",
					CreatedAt: pgtype.Timestamp{Time: time.Now(), Valid: true},
					IsActive:  pgtype.Bool{Bool: true, Valid: true},
				}, nil)
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "CreateBotToken_Faculty_Forbidden",
			handlerFunc:    func(h *handler.Handler) http.HandlerFunc { return h.CreateBotToken },
			method:         "POST",
			path:           "/bot/tokens",
			role:           database.UserRoleFaculty,
			setupMock:      func(m *mocks.Querier) {},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "CreateBotToken_Student_Forbidden",
			handlerFunc:    func(h *handler.Handler) http.HandlerFunc { return h.CreateBotToken },
			method:         "POST",
			path:           "/bot/tokens",
			role:           database.UserRoleStudent,
			setupMock:      func(m *mocks.Querier) {},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:        "RevokeBotToken_Dev_Success",
			handlerFunc: func(h *handler.Handler) http.HandlerFunc { return h.RevokeBotToken },
			method:      "DELETE",
			path:        "/bot/tokens/" + uuid.New().String(),
			role:        database.UserRoleDev,
			setupMock: func(m *mocks.Querier) {
				m.On("RevokeBotToken", mock.Anything, mock.Anything).Return(nil)
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:           "RevokeBotToken_Faculty_Forbidden",
			handlerFunc:    func(h *handler.Handler) http.HandlerFunc { return h.RevokeBotToken },
			method:         "DELETE",
			path:           "/bot/tokens/" + uuid.New().String(),
			role:           database.UserRoleFaculty,
			setupMock:      func(m *mocks.Querier) {},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "RevokeBotToken_Student_Forbidden",
			handlerFunc:    func(h *handler.Handler) http.HandlerFunc { return h.RevokeBotToken },
			method:         "DELETE",
			path:           "/bot/tokens/" + uuid.New().String(),
			role:           database.UserRoleStudent,
			setupMock:      func(m *mocks.Querier) {},
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQueries := mocks.NewQuerier(t)
			if tt.expectedStatus != http.StatusForbidden {
				tt.setupMock(mockQueries)
			}

			h := handler.New(mockQueries, &config.Config{})

			var body *strings.Reader
			if tt.method == "POST" {
				jsonBody, _ := json.Marshal(map[string]interface{}{
					"name": "Test Bot",
				})
				body = strings.NewReader(string(jsonBody))
			} else {
				body = strings.NewReader("")
			}

			req := httptest.NewRequest(tt.method, tt.path, body)

			claims := &middleware.UserClaims{
				UserID: uid.String(),
				Role:   string(tt.role),
			}
			ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)

			if tt.method == "DELETE" {
				rctx := chi.NewRouteContext()
				parts := strings.Split(tt.path, "/")
				if len(parts) > 0 {
					rctx.URLParams.Add("token_id", parts[len(parts)-1])
				}
				ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
			}
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			tt.handlerFunc(h).ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}
