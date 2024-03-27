package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
)

type Config struct {
	Accounts         []ConfigAccount          `json:"accounts"`
	TransactionTypes []ConfigTransactionTypes `json:"transaction_types"`
}

type ConfigAccount struct {
	Name       string `json:"name"`
	Firefly3Id string `json:"firefly3_id,omitempty"`
	MonobankId string `json:"monobank_id,omitempty"`
}

type ConfigTransactionTypes struct {
	Names    []string                      `json:"names"`
	Firefly3 ConfigTransactionTypeFirefly3 `json:"firefly3,omitempty"`
	MccCodes []int                         `json:"mcc_codes,omitempty"`
}

type ConfigTransactionTypeFirefly3 struct {
	Description string `json:"description"`
	Destination string `json:"destination"`
	Category    string `json:"category"`
}

func ReadConfig(path string) (Config, error) {
	var config Config

	// open file
	file, err := os.Open(path)
	if err != nil {
		return config, err
	}
	defer file.Close()

	// read file
	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return config, err
	}

	// read file ot config struct
	if err := json.Unmarshal(bytes, &config); err != nil {
		return config, err
	}

	return config, nil
}

func ConfigGetAccountByName(config Config, q string) ConfigAccount {
	for _, row := range config.Accounts {
		if row.Name == q {
			return row
		}
	}

	return ConfigAccount{}
}

func ConfigGetAccountByMonobankId(config Config, q string) ConfigAccount {
	for _, row := range config.Accounts {
		if row.MonobankId == q {
			return row
		}
	}

	return ConfigAccount{}
}
