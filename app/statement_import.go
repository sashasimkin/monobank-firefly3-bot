package app

import (
	"errors"
	"sort"
	"time"

	"gitea.stuzer.link/stuzer05/go-monobank"
	"stuzer.link/monobank-firefly3-bot/config"
)

func importStatementEntries(entries []statementEntry, cfg config.Config) (map[string]int, error) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Item.Time < entries[j].Item.Time })
	window, err := configuredTransferMatchWindow(cfg)
	if err != nil {
		return nil, err
	}
	pairs := matchStatementTransfers(entries, cfg.TransactionTypes, window)
	handled := make(map[string]bool, len(pairs)*2)
	importedByAccount := make(map[string]int)

	for _, pair := range pairs {
		outID := monobankExternalID(pair.Outgoing.Account.MonobankId, pair.Outgoing.Item.Id)
		inID := monobankExternalID(pair.Incoming.Account.MonobankId, pair.Incoming.Item.Id)
		outDate := statementDate(pair.Outgoing.Item)
		inDate := statementDate(pair.Incoming.Item)
		outExists, err := transactionExists(outID, outDate)
		if err != nil {
			return nil, err
		}
		inExists, err := transactionExists(inID, inDate)
		if err != nil {
			return nil, err
		}
		transfer := buildStatementTransfer(pair)
		transferExists, err := transactionExists(transfer.ExternalId, transfer.Date)
		if err != nil {
			return nil, err
		}
		handled[statementEntryKey(pair.Outgoing)] = true
		handled[statementEntryKey(pair.Incoming)] = true

		if !outExists && !inExists && !transferExists {
			if err := storeTransaction(transfer); err != nil {
				invalidateTransactionDate(transfer.Date)
				found, lookupErr := transactionExists(transfer.ExternalId, transfer.Date)
				if lookupErr != nil || !found {
					if lookupErr != nil {
						return nil, errors.Join(err, lookupErr)
					}
					return nil, err
				}
			} else {
				importedByAccount[pair.Outgoing.Account.MonobankId]++
				recordImportedID(transfer.ExternalId, transfer.Date)
			}
		} else if transferExists {
			recordImportedID(transfer.ExternalId, transfer.Date)
		}

		// Existing standalone entries are left untouched. Converting/deleting
		// user-visible Firefly records is a separate, explicit migration.
		if !outExists && !inExists {
			created, err := importStatementCommission(pair.Outgoing)
			if err != nil {
				return nil, err
			}
			if created {
				importedByAccount[pair.Outgoing.Account.MonobankId]++
			}
			created, err = importStatementCommission(pair.Incoming)
			if err != nil {
				return nil, err
			}
			if created {
				importedByAccount[pair.Incoming.Account.MonobankId]++
			}
		}
	}

	for _, entry := range entries {
		if handled[statementEntryKey(entry)] {
			continue
		}
		created, err := importTransactionWithTransferFallback(monobank.WebHookResponse{
			Type: "StatementItem",
			Data: monobank.WebHookResponseData{Account: entry.Account.MonobankId, StatementItem: entry.Item},
		}, true, true)
		if err != nil {
			return nil, err
		}
		if created {
			importedByAccount[entry.Account.MonobankId]++
		}
	}
	return importedByAccount, nil
}

func importStatementCommission(entry statementEntry) (bool, error) {
	if entry.Item.CommissionRate <= 0 {
		return false, nil
	}
	date := statementDate(entry.Item)
	externalID := monobankExternalID(entry.Account.MonobankId, entry.Item.Id) + ":commission"
	exists, err := transactionExists(externalID, date)
	if err != nil || exists {
		return false, err
	}
	fee := buildCommissionTransaction(entry.Item, entry.Account, externalID, date)
	if err := storeTransaction(fee); err != nil {
		invalidateTransactionDate(date)
		found, lookupErr := transactionExists(externalID, date)
		if lookupErr != nil || !found {
			if lookupErr != nil {
				return false, errors.Join(err, lookupErr)
			}
			return false, err
		}
	} else {
		recordImportedID(externalID, date)
		return true, nil
	}
	return false, nil
}

func statementDate(item monobank.StatementItemsInner) time.Time {
	return time.Unix(int64(item.Time), 0).Add(time.Hour * time.Duration(timezoneHoursDiff()))
}

func statementEntryKey(entry statementEntry) string {
	return entry.Account.MonobankId + ":" + entry.Item.Id
}
