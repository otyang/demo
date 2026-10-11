package ids

import (
	"fmt"

	"github.com/gofrs/uuid/v5"
)

const (
	customerPrefix     = "cust_"
	transactionPrefix  = "txn_"
	organisationPrefix = "org_"
	userPrefix         = "user_"
	entityPrefix       = "entity_"
	beneficiaryPrefix  = "ben_"
	accountPrefix      = "account_"
	kycPrefix          = "kyc_"
)

func New(prefix string) string {
	id, err := uuid.NewV4()
	if err != nil {
		panic(fmt.Errorf("generate UUID: %w", err))
	}
	return prefix + id.String()
}

func NewCustomerID() string {
	return New(customerPrefix)
}

func NewTransactionID() string {
	return New(transactionPrefix)
}

func NewOrganisationID() string {
	return New(organisationPrefix)
}

func NewUserID() string {
	return New(userPrefix)
}

func NewEntityID() string {
	return New(entityPrefix)
}

func NewBeneficiaryID() string {
	return New(beneficiaryPrefix)
}

func NewAccountID() string {
	return New(accountPrefix)
}

func NewKYCID() string {
	return New(kycPrefix)
}
