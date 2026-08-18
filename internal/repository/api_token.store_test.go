package repository

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

func TestFetchAPITokenGrantUsesPlaintextControlPlaneContract(t *testing.T) {
	token := "sk-rdp-v1-" + strings.Repeat("A", 43)
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db := bun.NewDB(sqlDB, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(regexp.QuoteMeta("SELECT ak.id AS api_key_id, ak.user_id AS user_id, akg.relay_target_ref AS relay_target_ref FROM api_keys AS ak") + ".*" + regexp.QuoteMeta(fmt.Sprintf("WHERE (ak.token = '%s')", token))).
		WillReturnRows(sqlmock.NewRows([]string{"api_key_id", "user_id", "relay_target_ref"}).
			AddRow("api-key-id", "user-id", "target-main"))
	grant, err := NewStore(db).FetchAPITokenGrant(context.Background(), APITokenLookup{
		Token: token, At: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if grant.APIKeyID != "api-key-id" || grant.UserID != "user-id" || grant.RelayTargetRef != "target-main" {
		t.Fatalf("unexpected grant: %#v", grant)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
