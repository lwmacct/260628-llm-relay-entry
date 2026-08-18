package repository

import (
	"context"
	"database/sql"
	"errors"
)

func (s *Store) FetchAPITokenGrant(ctx context.Context, lookup APITokenLookup) (*APITokenGrant, error) {
	row := new(APITokenGrant)
	err := s.db.NewSelect().
		TableExpr("api_keys AS ak").
		ColumnExpr("ak.id AS api_key_id").
		ColumnExpr("ak.user_id AS user_id").
		ColumnExpr("akg.remote_spec_json AS remote_spec_json").
		Join("JOIN users AS u ON u.id = ak.user_id").
		Join("JOIN api_key_groups AS akg ON akg.id = ak.group_id").
		Where("ak.token = ?", lookup.Token).
		Where("ak.status = 'active'").
		Where("ak.deleted_at IS NULL").
		Where("ak.expires_at IS NULL OR ak.expires_at > ?", lookup.At).
		Where("ak.limit_microusd IS NULL OR ak.used_microusd < ak.limit_microusd").
		Where("u.status = 'active'").
		Where("akg.status = 'active'").
		Limit(1).
		Scan(ctx, row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAPITokenNotFound
	}
	if err != nil {
		return nil, err
	}
	return row, nil
}
