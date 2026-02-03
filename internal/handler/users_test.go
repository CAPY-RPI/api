package handler_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/capyrpi/api/internal/config"
	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/database/mocks"
	"github.com/capyrpi/api/internal/handler"
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
