package config

type Config struct {
	Accounts                    []Account          `json:"accounts"`
	TransactionTypes            []TransactionTypes `json:"transaction_types"`
	ImportUnmatchedTransactions bool               `json:"import_unmatched_transactions,omitempty"`
}

type Account struct {
	Firefly3Name string `json:"firefly3_name,omitempty"`
	MonobankId   string `json:"monobank_id,omitempty"`
	Currency     string `json:"currency,omitempty"`
}

type TransactionTypes struct {
	Names           []string                `json:"names,omitempty"`
	NamesRefund     []string                `json:"names_refund,omitempty"`
	NamesLooseMatch bool                    `json:"names_loose_match,omitempty"` // "name%" match
	MccCodes        []int                   `json:"mcc_codes,omitempty"`
	Firefly3        TransactionTypeFirefly3 `json:"firefly3,omitempty"`
	SumMax          int                     `json:"sum_max,omitempty"`
}

type TransactionTypeFirefly3 struct {
	Type        string `json:"type,omitempty"`
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination,omitempty"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
}
