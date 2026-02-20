//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/testutils"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserQueries(t *testing.T) {
	// Spin up container
	pool := testutils.SetupTestDB(t)
	defer pool.Close()

	q := database.New(pool)
	ctx := context.Background()

	// 1. Create User
	newUser, err := q.CreateUser(ctx, database.CreateUserParams{
		FirstName:     "Test",
		LastName:      "User",
		PersonalEmail: pgtype.Text{String: "test@example.com", Valid: true},
		SchoolEmail:   pgtype.Text{Valid: false},
		Role:          database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, newUser.Uid)
	assert.Equal(t, "Test", newUser.FirstName)
	assert.Equal(t, database.UserRoleStudent, newUser.Role.UserRole)

	// 2. Get User
	fetchedUser, err := q.GetUserByID(ctx, newUser.Uid)
	require.NoError(t, err)
	assert.Equal(t, newUser.Uid, fetchedUser.Uid)

	// 3. Update User
	updatedUser, err := q.UpdateUser(ctx, database.UpdateUserParams{
		Uid:           newUser.Uid,
		FirstName:     pgtype.Text{String: "Updated", Valid: true},
		LastName:      pgtype.Text{Valid: false}, // Should keep original
		PersonalEmail: pgtype.Text{Valid: false},
		SchoolEmail:   pgtype.Text{Valid: false},
		Phone:         pgtype.Text{Valid: false},
		GradYear:      pgtype.Int4{Valid: false},
		Role:          database.NullUserRole{Valid: false},
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated", updatedUser.FirstName)
	assert.Equal(t, "User", updatedUser.LastName) // Kept original

	// 4. Delete User
	err = q.DeleteUser(ctx, newUser.Uid)
	require.NoError(t, err)

	// 5. Verify Deletion
	_, err = q.GetUserByID(ctx, newUser.Uid)
	assert.Error(t, err)
}

var testUserParams = database.CreateUserParams{
	FirstName:     "Test",
	LastName:      "User",
	PersonalEmail: pgtype.Text{String: "testuser@gmail.com", Valid: true},
	SchoolEmail:   pgtype.Text{String: "testuser@rpi.edu", Valid: true},
	Phone:         pgtype.Text{String: "555-555-5555", Valid: true},
	GradYear:      pgtype.Int4{Int32: 2027, Valid: true},
	Role:          database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
}

func TestAddUser(t *testing.T) {
	// Spin up container
	pool := testutils.SetupTestDB(t)
	defer pool.Close()

	q := database.New(pool)
	ctx := context.Background()

	q.CreateUser(ctx, testUserParams)
	user, err := q.GetUserByEmail(ctx, pgtype.Text{String: "testuser@gmail.com", Valid: true})
	require.NoError(t, err)
	assert.Equal(t, user.FirstName, "Test")
	assert.Equal(t, user.LastName, "User")
	assert.Equal(t, user.Phone.String, "555-555-5555")
	assert.Equal(t, user.GradYear.Int32, int32(2027))
	assert.Equal(t, user.Role.UserRole, database.UserRoleStudent)
}

func TestAddDuplicateUser(t *testing.T) {
	// Spin up container
	pool := testutils.SetupTestDB(t)
	defer pool.Close()
	q := database.New(pool)
	ctx := context.Background()

	addedUser, err := q.CreateUser(ctx, testUserParams)
	require.NoError(t, err)

	personalUser, err := q.GetUserByEmail(ctx, pgtype.Text{String: "testuser@gmail.com", Valid: true})
	require.NoError(t, err)

	assert.Equal(t, addedUser.Uid, personalUser.Uid)

	_, err = q.CreateUser(ctx, testUserParams)
	require.Error(t, err)

	schoolUser, err := q.GetUserByEmail(ctx, pgtype.Text{String: "testuser@rpi.edu", Valid: true})
	assert.Equal(t, addedUser.Uid, schoolUser.Uid)
}
