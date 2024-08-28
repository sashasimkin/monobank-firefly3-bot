package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

func LogString(str string) {
	if len(os.Getenv("LOG_FILE")) == 0 {
		return
	}

	f, err := os.OpenFile(os.Getenv("LOG_FILE"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err)
	}
	defer f.Close()

	if _, err := f.WriteString(str + "\n"); err != nil {
		fmt.Println(err)
	}
}

func LogContainsTransactionID(transactionID string) (bool, error) {
	if len(os.Getenv("LOG_FILE")) == 0 {
		return false, nil
	}

	// open the log file for reading.
	logFile, err := os.Open(os.Getenv("LOG_FILE"))
	if err != nil {
		return false, fmt.Errorf("error opening log file: %w", err)
	}
	defer logFile.Close()

	// create a new scanner to read the log file line by line.
	scanner := bufio.NewScanner(logFile)

	// iterate over each line of the log file.
	for scanner.Scan() {
		// unmarshal the JSON data from the current line.
		var transactionData struct {
			Data struct {
				StatementItem struct {
					ID string `json:"id"`
				} `json:"statementItem"`
			} `json:"data"`
		}
		err := json.Unmarshal(scanner.Bytes(), &transactionData)
		if err != nil {
			// skip lines that are not valid JSON.
			continue
		}

		// check if the transaction ID matches the given ID.
		if transactionData.Data.StatementItem.ID == transactionID {
			return true, nil
		}
	}

	// check for any errors that occurred during scanning.
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("error scanning log file: %w", err)
	}

	// transaction ID not found in the log file.
	return false, nil
}
