package models

type ClientInfo struct {
	ClientID    string    `json:"clientId"`
	Name        string    `json:"name"`
	WebHookURL  string    `json:"webHookUrl"`
	Permissions string    `json:"permissions"`
	Accounts    []Account `json:"accounts_list"`
	Jars        []Jar     `json:"jars"`
}

type Account struct {
	ID           string   `json:"id"`
	SendID       string   `json:"sendId"`
	Balance      int      `json:"balance"`
	CreditLimit  int      `json:"creditLimit"`
	Type         string   `json:"type"`
	CurrencyCode int      `json:"currencyCode"`
	CashbackType string   `json:"cashbackType"`
	MaskedPan    []string `json:"maskedPan"`
	Iban         string   `json:"iban"`
}

type Jar struct {
	ID           string `json:"id"`
	SendID       string `json:"sendId"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	CurrencyCode int    `json:"currencyCode"`
	Balance      int    `json:"balance"`
	Goal         int    `json:"goal"`
}
