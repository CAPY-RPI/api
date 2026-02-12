package adapters

import (
	"context"

	"github.com/capyrpi/api/internal/auth/ports"
	"github.com/capyrpi/api/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type UserRepoAdapter struct {
	queries database.Querier
}

func NewUserRepoAdapter(queries database.Querier) ports.UserRepo {
	return &UserRepoAdapter{
		queries: queries,
	}
}

func (r *UserRepoAdapter) GetUserByEmail(ctx context.Context, email pgtype.Text) (database.User, error) {
	return r.queries.GetUserByEmail(ctx, email)
}

func (r *UserRepoAdapter) CreateUser(ctx context.Context, arg database.CreateUserParams) (database.User, error) {
	return r.queries.CreateUser(ctx, arg)
}

func (r *UserRepoAdapter) GetUserByID(ctx context.Context, uid uuid.UUID) (database.User, error) {
	return r.queries.GetUserByID(ctx, uid)
}
