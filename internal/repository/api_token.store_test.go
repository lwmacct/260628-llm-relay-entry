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
	remoteSpec := `{"uuid":"01990f4a-9e4c-7c42-a7ec-5c3f37a6f6b2","http":{"url":"https://resolver.example/resolver"}}`
	mock.ExpectQuery(regexp.QuoteMeta("SELECT ak.id AS api_key_id, ak.user_id AS user_id, akg.remote_spec_json AS remote_spec_json FROM api_keys AS ak") + ".*" + regexp.QuoteMeta(fmt.Sprintf("WHERE (ak.token = '%s')", token))).
		WillReturnRows(sqlmock.NewRows([]string{"api_key_id", "user_id", "remote_spec_json"}).
			AddRow("api-key-id", "user-id", remoteSpec))
	grant, err := NewStore(db).FetchAPITokenGrant(context.Background(), APITokenLookup{
		Token: token, At: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if grant.APIKeyID != "api-key-id" || grant.UserID != "user-id" || grant.RemoteSpecJSON != remoteSpec {
		t.Fatalf("unexpected grant: %#v", grant)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
