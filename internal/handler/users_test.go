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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGetUser(t *testing.T) {
	uid := uuid.New()

	tests := []struct {
		name           string
		uidParam       string
		mockSetup      func(*mocks.Querier)
		expectedStatus int
	}{
		{
			name:     "Success",
			uidParam: uid.String(),
			mockSetup: func(m *mocks.Querier) {
				m.On("GetUserByID", mock.Anything, uid).Return(database.User{
					Uid:           uid,
					FirstName:     "John",
					LastName:      "Doe",
					PersonalEmail: pgtype.Text{String: "john@example.com", Valid: true},
					Role:          database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
				}, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "UserNotFound",
			uidParam: uid.String(),
			mockSetup: func(m *mocks.Querier) {
				m.On("GetUserByID", mock.Anything, uid).Return(database.User{}, pgx.ErrNoRows)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:     "InvalidUUID",
			uidParam: "invalid-uuid",
			mockSetup: func(m *mocks.Querier) {
				// No DB call expected
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockQueries := mocks.NewQuerier(t)
			tt.mockSetup(mockQueries)

			h := handler.New(mockQueries, &config.Config{})

			// Router to handle URL params
			r := chi.NewRouter()
			r.Get("/users/{uid}", h.GetUser)

			// Request
			req, _ := http.NewRequest("GET", fmt.Sprintf("/users/%s", tt.uidParam), nil)
			rr := httptest.NewRecorder()

			// Execute
			r.ServeHTTP(rr, req)

			// Assert
			assert.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}

func TestUpdateUser(t *testing.T) {
	targetUID := uuid.New()
	authenticatedUID := uuid.New()
	firstName := "Updated"
	currentRole := "student"
	role := "faculty"

	tests := []struct {
		name           string
		requestBody    dto.UpdateUserRequest
		mockSetup      func(*mocks.Querier)
		setupContext   func() context.Context
		expectedStatus int
	}{
		{
			name: "NonDevCanUpdateWhenSubmittedRoleMatchesCurrentRole",
			requestBody: dto.UpdateUserRequest{
				FirstName: &firstName,
				Role:      &currentRole,
			},
			mockSetup: func(m *mocks.Querier) {
				m.On("GetUserByID", mock.Anything, authenticatedUID).Return(database.User{
					Uid:  authenticatedUID,
					Role: database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
				}, nil)
				m.On("GetUserByID", mock.Anything, targetUID).Return(database.User{
					Uid:  targetUID,
					Role: database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
				}, nil)
				m.On("UpdateUser", mock.Anything, mock.MatchedBy(func(arg database.UpdateUserParams) bool {
					return arg.Uid == targetUID &&
						arg.FirstName.Valid && arg.FirstName.String == firstName &&
						!arg.Role.Valid
				})).Return(database.User{
					Uid:       targetUID,
					FirstName: firstName,
					LastName:  "Doe",
					Role:      database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
				}, nil)
			},
			setupContext: func() context.Context {
				ctx := context.Background()
				claims := &middleware.UserClaims{UserID: authenticatedUID.String()}
				ctx = context.WithValue(ctx, middleware.UserClaimsKey, claims)
				return context.WithValue(ctx, middleware.AuthTypeKey, "human")
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "NonDevCannotUpdateRole",
			requestBody: dto.UpdateUserRequest{
				Role: &role,
			},
			mockSetup: func(m *mocks.Querier) {
				m.On("GetUserByID", mock.Anything, authenticatedUID).Return(database.User{
					Uid:  authenticatedUID,
					Role: database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
				}, nil)
				m.On("GetUserByID", mock.Anything, targetUID).Return(database.User{
					Uid:  targetUID,
					Role: database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
				}, nil)
			},
			setupContext: func() context.Context {
				ctx := context.Background()
				claims := &middleware.UserClaims{UserID: authenticatedUID.String()}
				ctx = context.WithValue(ctx, middleware.UserClaimsKey, claims)
				return context.WithValue(ctx, middleware.AuthTypeKey, "human")
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name: "DevCanUpdateRole",
			requestBody: dto.UpdateUserRequest{
				Role: &role,
			},
			mockSetup: func(m *mocks.Querier) {
				m.On("GetUserByID", mock.Anything, authenticatedUID).Return(database.User{
					Uid:  authenticatedUID,
					Role: database.NullUserRole{UserRole: database.UserRoleDev, Valid: true},
				}, nil)
				m.On("GetUserByID", mock.Anything, targetUID).Return(database.User{
					Uid:  targetUID,
					Role: database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
				}, nil)
				m.On("UpdateUser", mock.Anything, mock.MatchedBy(func(arg database.UpdateUserParams) bool {
					return arg.Uid == targetUID &&
						arg.Role.Valid &&
						arg.Role.UserRole == database.UserRoleFaculty
				})).Return(database.User{
					Uid:  targetUID,
					Role: database.NullUserRole{UserRole: database.UserRoleFaculty, Valid: true},
				}, nil)
			},
			setupContext: func() context.Context {
				ctx := context.Background()
				claims := &middleware.UserClaims{UserID: authenticatedUID.String()}
				ctx = context.WithValue(ctx, middleware.UserClaimsKey, claims)
				return context.WithValue(ctx, middleware.AuthTypeKey, "human")
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQueries := mocks.NewQuerier(t)
			tt.mockSetup(mockQueries)

			h := handler.New(mockQueries, &config.Config{})
			r := chi.NewRouter()
			r.Put("/users/{uid}", h.UpdateUser)

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/users/%s", targetUID), bytes.NewBuffer(body))
			req = req.WithContext(tt.setupContext())
			rr := httptest.NewRecorder()

			r.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}
