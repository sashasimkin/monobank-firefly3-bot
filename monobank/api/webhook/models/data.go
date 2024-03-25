package models

type Data struct {
	Account       string        `json:"account"`
	StatementItem StatementItem `json:"statementItem"`
}
