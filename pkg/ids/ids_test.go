package ids

import (
	"strings"
	"testing"

	"github.com/gofrs/uuid/v5"
)

func TestNewAddsPrefixToUUIDv4(t *testing.T) {
	got := New("custom_")
	assertUUIDWithPrefix(t, got, "custom_")
}

func TestNewAllowsEmptyPrefix(t *testing.T) {
	got := New("")
	assertUUIDWithPrefix(t, got, "")
}

func TestConvenienceIDGenerators(t *testing.T) {
	tests := []struct {
		name string
		new  func() string
		want string
	}{
		{"customer", NewCustomerID, "cust_"},
		{"transaction", NewTransactionID, "txn_"},
		{"organisation", NewOrganisationID, "org_"},
		{"user", NewUserID, "user_"},
		{"entity", NewEntityID, "entity_"},
		{"beneficiary", NewBeneficiaryID, "ben_"},
		{"account", NewAccountID, "account_"},
		{"kyc", NewKYCID, "kyc_"},
	}

	generated := make(map[string]struct{}, len(tests))
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.new()
			assertUUIDWithPrefix(t, got, test.want)
			if _, exists := generated[got]; exists {
				t.Errorf("%s ID duplicated a generated ID: %q", test.name, got)
			}
			generated[got] = struct{}{}
		})
	}
}

func TestConvenienceIDGeneratorsProduceDistinctIDs(t *testing.T) {
	generators := []func() string{
		NewCustomerID,
		NewTransactionID,
		NewOrganisationID,
		NewUserID,
		NewEntityID,
		NewBeneficiaryID,
		NewAccountID,
		NewKYCID,
	}
	seen := make(map[string]struct{}, len(generators))

	for _, generate := range generators {
		id := generate()
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate ID generated: %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestGeneratedIDsAreUnique(t *testing.T) {
	const count = 1000
	seen := make(map[string]struct{}, count)
	for i := 0; i < count; i++ {
		id := NewUserID()
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate ID generated: %q", id)
		}
		seen[id] = struct{}{}
	}
}

func assertUUIDWithPrefix(t *testing.T, id, prefix string) {
	t.Helper()

	rawID := string(id)
	if !strings.HasPrefix(rawID, prefix) {
		t.Fatalf("ID = %q, want prefix %q", id, prefix)
	}
	parsed, err := uuid.FromString(strings.TrimPrefix(rawID, prefix))
	if err != nil {
		t.Fatalf("generated suffix is not a UUID: %v", err)
	}
	if parsed.Version() != 4 {
		t.Errorf("UUID version = %v, want version 4", parsed.Version())
	}
}
