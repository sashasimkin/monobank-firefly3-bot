package requests

import (
	"encoding/json"
	"main/monobank"
	models2 "main/monobank/api/statement/models"
	"strconv"
)

func Statement(request models2.StatementRequest) ([]models2.Statement, error) {
	data := []models2.Statement{}

	responseJson, err := monobank.Request("GET", "https://api.monobank.ua/personal/statement/"+request.Account+"/"+strconv.Itoa(request.From)+"/"+strconv.Itoa(request.To), struct{}{})
	if err != nil {
		return data, err
	}
	json.Unmarshal([]byte(responseJson), &data)

	return data, nil
}
