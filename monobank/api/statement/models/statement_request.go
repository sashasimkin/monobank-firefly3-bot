package models

type StatementRequest struct {
	Account string `json:"account"`
	From    int    `json:"from"`
	To      int    `json:"to"`
}
