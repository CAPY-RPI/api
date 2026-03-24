package handler

import (
	"context"
	"testing"

	"github.com/capyrpi/api/internal/config"
	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/database/mocks"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestUpsertUserCreatesStudentByDefault(t *testing.T) {
	mockQueries := mocks.NewQuerier(t)
	h := New(mockQueries, &config.Config{Env: "development"})

	mockQueries.On("GetUserByEmail", mock.Anything, pgtype.Text{
		String: "student@example.com",
		Valid:  true,
	}).Return(database.User{}, pgx.ErrNoRows).Once()
	mockQueries.On("CreateUser", mock.Anything, mock.MatchedBy(func(arg database.CreateUserParams) bool {
		return arg.FirstName == "Grace" &&
			arg.LastName == "Hopper" &&
			arg.PersonalEmail.Valid &&
			arg.PersonalEmail.String == "student@example.com" &&
			arg.Role.Valid &&
			arg.Role.UserRole == database.UserRoleStudent
	})).Return(database.User{
		Uid:           uuid.New(),
		PersonalEmail: pgtype.Text{String: "student@example.com", Valid: true},
		Role:          database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
	}, nil).Once()

	user, err := h.upsertUser(context.Background(), " Student@Example.com ", "Grace", "Hopper")

	assert.NoError(t, err)
	assert.Equal(t, database.UserRoleStudent, user.Role.UserRole)
}

func TestUpsertUserReturnsExistingUserWithoutRolePromotion(t *testing.T) {
	mockQueries := mocks.NewQuerier(t)
	h := New(mockQueries, &config.Config{Env: "development"})
	userID := uuid.New()

	mockQueries.On("GetUserByEmail", mock.Anything, pgtype.Text{
		String: "dev@example.com",
		Valid:  true,
	}).Return(database.User{
		Uid:           userID,
		PersonalEmail: pgtype.Text{String: "dev@example.com", Valid: true},
		Role:          database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
	}, nil).Once()

	user, err := h.upsertUser(context.Background(), " dev@example.com ", "Katherine", "Johnson")

	assert.NoError(t, err)
	assert.Equal(t, userID, user.Uid)
	assert.Equal(t, database.UserRoleStudent, user.Role.UserRole)
}
