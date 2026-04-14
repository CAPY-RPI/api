package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"github.com/jackc/pgx/v5/pgconn"
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

func TestCreateOrganizationBotRequiresGuildID(t *testing.T) {
	mockQueries := mocks.NewQuerier(t)
	h := handler.New(mockQueries, &config.Config{})

	body, _ := json.Marshal(dto.CreateOrganizationRequest{
		Name: "Bot Org",
	})

	req := httptest.NewRequest(http.MethodPost, "/organizations", bytes.NewBuffer(body))
	req = req.WithContext(context.WithValue(context.Background(), middleware.AuthTypeKey, "bot"))
	rr := httptest.NewRecorder()

	http.HandlerFunc(h.CreateOrganization).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestCreateOrganizationBotCreatesDiscordLinkInTransaction(t *testing.T) {
	oid := uuid.New()
	guildID := int64(123456789)
	tx := &fakeTx{
		org: database.Organization{
			Oid:  oid,
			Name: "Bot Org",
		},
	}
	beginner := fakeTxBeginner{tx: tx}

	mockQueries := mocks.NewQuerier(t)
	h := handler.New(mockQueries, &config.Config{}, beginner)

	body, _ := json.Marshal(dto.CreateOrganizationRequest{
		Name:    "Bot Org",
		GuildID: &guildID,
	})

	req := httptest.NewRequest(http.MethodPost, "/organizations", bytes.NewBuffer(body))
	req = req.WithContext(context.WithValue(context.Background(), middleware.AuthTypeKey, "bot"))
	rr := httptest.NewRecorder()

	http.HandlerFunc(h.CreateOrganization).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	assert.True(t, tx.createOrganizationCalled)
	assert.True(t, tx.createOrgDiscordCalled)
	assert.True(t, tx.commitCalled)
}

func TestCreateOrganizationBotRollsBackWhenDiscordLinkFails(t *testing.T) {
	oid := uuid.New()
	guildID := int64(123456789)
	tx := &fakeTx{
		org: database.Organization{
			Oid:  oid,
			Name: "Bot Org",
		},
		createOrgDiscordErr: &pgconn.PgError{Code: "23505"},
	}
	beginner := fakeTxBeginner{tx: tx}

	mockQueries := mocks.NewQuerier(t)
	h := handler.New(mockQueries, &config.Config{}, beginner)

	body, _ := json.Marshal(dto.CreateOrganizationRequest{
		Name:    "Bot Org",
		GuildID: &guildID,
	})

	req := httptest.NewRequest(http.MethodPost, "/organizations", bytes.NewBuffer(body))
	req = req.WithContext(context.WithValue(context.Background(), middleware.AuthTypeKey, "bot"))
	rr := httptest.NewRecorder()

	http.HandlerFunc(h.CreateOrganization).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusConflict, rr.Code)
	assert.True(t, tx.createOrganizationCalled)
	assert.True(t, tx.createOrgDiscordCalled)
	assert.False(t, tx.commitCalled)
	assert.True(t, tx.rollbackCalled)
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

type fakeTxBeginner struct {
	tx  pgx.Tx
	err error
}

func (f fakeTxBeginner) Begin(ctx context.Context) (pgx.Tx, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tx, nil
}

type fakeTx struct {
	org                  database.Organization
	createOrganizationErr error
	createOrgDiscordErr   error
	commitErr             error
	createOrganizationCalled bool
	createOrgDiscordCalled   bool
	commitCalled             bool
	rollbackCalled           bool
}

func (f *fakeTx) Begin(context.Context) (pgx.Tx, error) { return nil, errors.New("not implemented") }

func (f *fakeTx) Commit(context.Context) error {
	f.commitCalled = true
	return f.commitErr
}

func (f *fakeTx) Rollback(context.Context) error {
	f.rollbackCalled = true
	return nil
}

func (f *fakeTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("not implemented")
}

func (f *fakeTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }

func (f *fakeTx) LargeObjects() pgx.LargeObjects { return pgx.LargeObjects{} }

func (f *fakeTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if sql == createOrgDiscordSQL {
		f.createOrgDiscordCalled = true
		return pgconn.CommandTag{}, f.createOrgDiscordErr
	}
	return pgconn.CommandTag{}, errors.New("unexpected exec")
}

func (f *fakeTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	if sql == createOrganizationSQL {
		f.createOrganizationCalled = true
		if f.createOrganizationErr != nil {
			return fakeRow{err: f.createOrganizationErr}
		}
		return fakeRow{values: []any{f.org.Oid, f.org.Name, f.org.DateCreated, f.org.DateModified}}
	}
	return fakeRow{err: errors.New("unexpected query")}
}

func (f *fakeTx) Conn() *pgx.Conn { return nil }

type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return errors.New("scan arity mismatch")
	}
	for i := range dest {
		switch d := dest[i].(type) {
		case *uuid.UUID:
			*d = r.values[i].(uuid.UUID)
		case *string:
			*d = r.values[i].(string)
		case *pgtype.Date:
			*d = r.values[i].(pgtype.Date)
		default:
			return errors.New("unexpected scan destination")
		}
	}
	return nil
}

const createOrganizationSQL = `-- name: CreateOrganization :one
INSERT INTO organizations (name)
VALUES ($1)
RETURNING oid, name, date_created, date_modified
`

const createOrgDiscordSQL = `-- name: CreateOrgDiscord :exec
INSERT INTO org_discords (oid, guild_id)
VALUES ($1, $2)
`
