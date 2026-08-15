package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

func TestFetchAPITokenGrantByDigestUsesSharedControlPlaneContract(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db := bun.NewDB(sqlDB, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(regexp.QuoteMeta("SELECT ak.id AS api_key_id, ak.user_id AS user_id, ak.binding_id AS binding_id, rb.vendor_credential_id AS vendor_credential_id FROM api_keys AS ak")).
		WillReturnRows(sqlmock.NewRows([]string{"api_key_id", "user_id", "binding_id", "vendor_credential_id"}).
			AddRow(42, 7, "binding-id", "credential-id"))
	grant, err := NewStore(db).FetchAPITokenGrantByDigest(context.Background(), APITokenDigest{
		DigestKeyID: "v1", TokenDigest: "digest", At: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if grant.APIKeyID != 42 || grant.UserID != 7 || grant.BindingID != "binding-id" || grant.VendorCredentialID != "credential-id" {
		t.Fatalf("unexpected grant: %#v", grant)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
