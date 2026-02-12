package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/capyrpi/api/internal/auth/ports"
	"github.com/capyrpi/api/internal/auth/service"
	"github.com/capyrpi/api/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mocks
type MockUserRepo struct {
	mock.Mock
}

func (m *MockUserRepo) GetUserByEmail(ctx context.Context, email pgtype.Text) (database.User, error) {
	args := m.Called(ctx, email)
	return args.Get(0).(database.User), args.Error(1)
}

func (m *MockUserRepo) CreateUser(ctx context.Context, arg database.CreateUserParams) (database.User, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(database.User), args.Error(1)
}

func (m *MockUserRepo) GetUserByID(ctx context.Context, uid uuid.UUID) (database.User, error) {
	args := m.Called(ctx, uid)
	return args.Get(0).(database.User), args.Error(1)
}

type MockBotRepo struct {
	mock.Mock
}

func (m *MockBotRepo) CreateBotToken(ctx context.Context, arg database.CreateBotTokenParams) (database.BotToken, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(database.BotToken), args.Error(1)
}

type MockTokenProvider struct {
	mock.Mock
}

func (m *MockTokenProvider) GenerateToken(user database.User) (string, error) {
	args := m.Called(user)
	return args.String(0), args.Error(1)
}

type MockOAuthProvider struct {
	mock.Mock
}

func (m *MockOAuthProvider) GetAuthURL(state string) string {
	args := m.Called(state)
	return args.String(0)
}

func (m *MockOAuthProvider) ExchangeCode(ctx context.Context, code string) (*ports.UserInfo, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ports.UserInfo), args.Error(1)
}

func TestAuthService_HandleOAuthCallback(t *testing.T) {
	// Setup
	mockUserRepo := new(MockUserRepo)
	mockBotRepo := new(MockBotRepo)
	mockTokenProvider := new(MockTokenProvider)
	mockGoogle := new(MockOAuthProvider)
	mockMicrosoft := new(MockOAuthProvider)

	svc := service.NewAuthService(mockUserRepo, mockBotRepo, mockTokenProvider, mockGoogle, mockMicrosoft)
	ctx := context.Background()

	t.Run("Success existing user", func(t *testing.T) {
		email := "test@example.com"
		userInfo := &ports.UserInfo{Email: email, FirstName: "John", LastName: "Doe"}
		user := database.User{
			Uid:           uuid.New(),
			FirstName:     "John",
			LastName:      "Doe",
			PersonalEmail: pgtype.Text{String: email, Valid: true},
		}
		token := "jwt-token"

		mockGoogle.On("ExchangeCode", ctx, "auth-code").Return(userInfo, nil)
		mockUserRepo.On("GetUserByEmail", ctx, pgtype.Text{String: email, Valid: true}).Return(user, nil)
		mockTokenProvider.On("GenerateToken", user).Return(token, nil)

		res, err := svc.HandleOAuthCallback(ctx, "google", "auth-code")

		assert.NoError(t, err)
		assert.Equal(t, user, res.User)
		assert.Equal(t, token, res.Token)
		mockGoogle.AssertExpectations(t)
		mockUserRepo.AssertExpectations(t)
		mockTokenProvider.AssertExpectations(t)
	})

	t.Run("Success new user", func(t *testing.T) {
		email := "new@example.com"
		userInfo := &ports.UserInfo{Email: email, FirstName: "Jane", LastName: "Doe"}
		user := database.User{
			Uid:           uuid.New(),
			FirstName:     "Jane",
			LastName:      "Doe",
			PersonalEmail: pgtype.Text{String: email, Valid: true},
		}
		token := "jwt-token"

		mockGoogle.On("ExchangeCode", ctx, "new-code").Return(userInfo, nil)
		mockUserRepo.On("GetUserByEmail", ctx, pgtype.Text{String: email, Valid: true}).Return(database.User{}, pgx.ErrNoRows)
		mockUserRepo.On("CreateUser", ctx, mock.AnythingOfType("database.CreateUserParams")).Return(user, nil)
		mockTokenProvider.On("GenerateToken", user).Return(token, nil)

		res, err := svc.HandleOAuthCallback(ctx, "google", "new-code")

		assert.NoError(t, err)
		assert.Equal(t, user, res.User)
		assert.Equal(t, token, res.Token)
	})

	t.Run("Invalid provider", func(t *testing.T) {
		res, err := svc.HandleOAuthCallback(ctx, "invalid", "code")
		assert.Error(t, err)
		assert.Nil(t, res)
	})

	t.Run("Exchange code error", func(t *testing.T) {
		mockGoogle.On("ExchangeCode", ctx, "bad-code").Return(nil, errors.New("exchange error"))
		res, err := svc.HandleOAuthCallback(ctx, "google", "bad-code")
		assert.Error(t, err)
		assert.Nil(t, res)
	})
}

func TestAuthService_GenerateBotToken(t *testing.T) {
	// Setup
	mockUserRepo := new(MockUserRepo)
	mockBotRepo := new(MockBotRepo)
	mockTokenProvider := new(MockTokenProvider)
	mockGoogle := new(MockOAuthProvider)
	mockMicrosoft := new(MockOAuthProvider)

	svc := service.NewAuthService(mockUserRepo, mockBotRepo, mockTokenProvider, mockGoogle, mockMicrosoft)
	ctx := context.Background()
	uid := uuid.New()

	t.Run("Success", func(t *testing.T) {
		name := "My Bot"
		botToken := database.BotToken{
			TokenID:   uuid.New(),
			Name:      name,
			CreatedBy: uid,
			CreatedAt: pgtype.Timestamp{Time: time.Now(), Valid: true},
		}

		// Use mock.MatchedBy to check arguments
		mockBotRepo.On("CreateBotToken", ctx, mock.MatchedBy(func(arg database.CreateBotTokenParams) bool {
			return arg.Name == name && arg.CreatedBy == uid && len(arg.TokenHash) > 0
		})).Return(botToken, nil)

		res, err := svc.GenerateBotToken(ctx, name, uid, nil)

		assert.NoError(t, err)
		assert.Equal(t, botToken, res.Token)
		assert.NotEmpty(t, res.RawToken)
		mockBotRepo.AssertExpectations(t)
	})
}
