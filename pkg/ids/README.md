# IDs

`github.com/otyang/demo/pkg/ids` creates UUID v4 identifiers with an optional prefix. The
prefixes distinguish ID types in logs and serialized values; the UUID suffix
provides the random identifier.

## Typed ID shortcuts

```go
import "github.com/otyang/demo/pkg/ids"

customerID := ids.NewCustomerID()
transactionID := ids.NewTransactionID()
```

The convenience generators return a single `string` and include these
prefixes:

| Function | Prefix |
|---|---|
| `NewCustomerID()` | `cust_` |
| `NewTransactionID()` | `txn_` |
| `NewOrganisationID()` | `org_` |
| `NewUserID()` | `user_` |
| `NewEntityID()` | `entity_` |
| `NewBeneficiaryID()` | `ben_` |
| `NewAccountID()` | `account_` |
| `NewKYCID()` | `kyc_` |

## Custom prefix

Use `New` to create IDs for other entity types:

```go
import "github.com/otyang/demo/pkg/ids"

invoiceID := ids.New("invoice_")
```

Generated IDs are strings such as `cust_550e8400-e29b-41d4-a716-446655440000`.
The prefix is descriptive, not an access-control or secrecy mechanism.

These functions panic if UUID generation fails. Use them when generation
failure should stop the current operation rather than being returned as an
error.
