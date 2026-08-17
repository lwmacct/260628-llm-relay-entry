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
	mock.ExpectQuery(regexp.QuoteMeta("SELECT ak.id AS api_key_id, ak.user_id AS user_id, ak.binding_id AS binding_id, rb.vendor_credential_id AS vendor_credential_id FROM api_keys AS ak") + ".*" + regexp.QuoteMeta(fmt.Sprintf("WHERE (ak.token = '%s')", token))).
		WillReturnRows(sqlmock.NewRows([]string{"api_key_id", "user_id", "binding_id", "vendor_credential_id"}).
			AddRow(42, 7, "binding-id", "credential-id"))
	grant, err := NewStore(db).FetchAPITokenGrant(context.Background(), APITokenLookup{
		Token: token, At: time.Now().UTC(),
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
