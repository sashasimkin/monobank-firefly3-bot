package config

type Config struct {
	Accounts         []ConfigAccount          `json:"accounts"`
	TransactionTypes []ConfigTransactionTypes `json:"transaction_types"`
}

type ConfigAccount struct {
	Firefly3Name string `json:"firefly3_name,omitempty"`
	MonobankId   string `json:"monobank_id,omitempty"`
	Currency     string `json:"currency,omitempty"`
}

type ConfigTransactionTypes struct {
	Names       []string                      `json:"names,omitempty"`
	NamesRefund []string                      `json:"names_refund,omitempty"`
	MccCodes    []int                         `json:"mcc_codes,omitempty"`
	Firefly3    ConfigTransactionTypeFirefly3 `json:"firefly3,omitempty"`
	SumMax      int                           `json:"sum_max,omitempty"`
}

type ConfigTransactionTypeFirefly3 struct {
	Type                     string `json:"type,omitempty"`
	Destination              string `json:"destination,omitempty"`
	Description              string `json:"description,omitempty"`
	Category                 string `json:"category,omitempty"`
	IsUseDestinationAsSource bool   `json:"is_use_destination_as_source,omitempty"`
}
