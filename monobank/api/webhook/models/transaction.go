package models

type Transaction struct {
	Type string `json:"type"`
	Data Data   `json:"data"`
}
