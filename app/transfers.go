package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"gitea.stuzer.link/stuzer05/go-firefly3/v2"
	"gitea.stuzer.link/stuzer05/go-monobank"
	"stuzer.link/monobank-firefly3-bot/config"
)

const defaultTransferMatchWindowSeconds = 120
const maximumTransferMatchWindowSeconds = 600

func configuredTransferMatchWindow(cfg config.Config) (time.Duration, error) {
	seconds := cfg.TransferMatchWindowSeconds
	if seconds == 0 {
		seconds = defaultTransferMatchWindowSeconds
	}
	if seconds < 0 || seconds > maximumTransferMatchWindowSeconds {
		return 0, errors.New("transfer match window must be zero (default) or between 1 and 600 seconds")
	}
	return time.Duration(seconds) * time.Second, nil
}

func requiresStatementTransferPair(rule *config.TransactionTypes) bool {
	return rule != nil && rule.Firefly3.Type == "transfer" && slices.Contains(rule.MccCodes, 4829) && rule.Firefly3.Source == "" && rule.Firefly3.Destination == ""
}

func importTransferWebhook(ctx context.Context, event monobank.WebHookResponse) error {
	window, err := configuredTransferMatchWindow(App().Config)
	if err != nil {
		return err
	}
	source := App().Config.GetAccountByMonobankId(event.Data.Account)
	if source.MonobankId == "" || source.Firefly3Name == "" {
		return errors.New("cannot find Firefly or Monobank account mapping")
	}
	eventTime := int64(event.Data.StatementItem.Time)
	from := eventTime - int64(window/time.Second)
	if from < 0 {
		from = 0
	}
	to := eventTime + int64(window/time.Second)
	now := time.Now().Unix()
	if to > now {
		to = now
	}
	entries := []statementEntry{{Account: source, Item: event.Data.StatementItem}}
	for _, account := range App().Config.Accounts {
		if account.MonobankId == source.MonobankId || account.Currency != source.Currency {
			continue
		}
		items, err := fetchStatementItems(ctx, account.MonobankId, from, to)
		if err != nil {
			return err
		}
		for _, item := range items {
			entries = append(entries, statementEntry{Account: account, Item: item})
		}
	}

	pairs := matchStatementTransfers(entries, App().Config.TransactionTypes, window)
	eventKey := statementEntryKey(entries[0])
	selected := make([]statementEntry, 0, 2)
	for _, pair := range pairs {
		outKey := statementEntryKey(pair.Outgoing)
		inKey := statementEntryKey(pair.Incoming)
		if outKey == eventKey || inKey == eventKey {
			selected = append(selected, pair.Outgoing, pair.Incoming)
		}
	}
	if len(selected) == 0 {
		// Do not guess the other account. The incremental poller will retry
		// this row and preserve its signed direction if it remains unmatched.
		return nil
	}
	_, err = importStatementEntries(selected, App().Config)
	return err
}

type statementEntry struct {
	Account config.Account
	Item    monobank.StatementItemsInner
}

type statementTransferPair struct {
	Outgoing statementEntry
	Incoming statementEntry
}

// matchStatementTransfers only joins mutually unique MCC 4829 statement rows
// from different mapped accounts with the same currency and amount inside the
// configured time window. Ambiguous rows are left for ordinary rule handling.
func matchStatementTransfers(entries []statementEntry, rules []config.TransactionTypes, window time.Duration) []statementTransferPair {
	if window <= 0 {
		window = defaultTransferMatchWindowSeconds * time.Second
	}
	var outgoing, incoming []statementEntry
	for _, entry := range entries {
		if entry.Item.Hold || int(entry.Item.Mcc) != 4829 || mainAmountMinor(entry.Item) <= 0 {
			continue
		}
		rule, refund := matchTransactionRuleFrom(rules, entry.Item)
		if refund || !requiresStatementTransferPair(rule) {
			continue
		}
		if entry.Item.Amount < 0 {
			outgoing = append(outgoing, entry)
		} else if entry.Item.Amount > 0 {
			incoming = append(incoming, entry)
		}
	}

	forward := make([][]int, len(outgoing))
	reverse := make([][]int, len(incoming))
	for oi, out := range outgoing {
		for ii, in := range incoming {
			if out.Account.MonobankId == in.Account.MonobankId || out.Account.Currency != in.Account.Currency || mainAmountMinor(out.Item) != mainAmountMinor(in.Item) {
				continue
			}
			delta := time.Duration(math.Abs(float64(int64(out.Item.Time)-int64(in.Item.Time)))) * time.Second
			if delta > window {
				continue
			}
			forward[oi] = append(forward[oi], ii)
			reverse[ii] = append(reverse[ii], oi)
		}
	}

	var pairs []statementTransferPair
	for oi, candidates := range forward {
		if len(candidates) != 1 {
			continue
		}
		ii := candidates[0]
		if len(reverse[ii]) == 1 {
			pairs = append(pairs, statementTransferPair{Outgoing: outgoing[oi], Incoming: incoming[ii]})
		}
	}
	return pairs
}

func buildStatementTransfer(pair statementTransferPair) firefly3.TransactionSplitStore {
	typeTransfer := firefly3.TRANSFER_TransactionTypeProperty
	out := pair.Outgoing
	in := pair.Incoming
	date := time.Unix(int64(out.Item.Time), 0).Add(time.Hour * time.Duration(timezoneHoursDiff()))
	return firefly3.TransactionSplitStore{
		Type_:           &typeTransfer,
		Date:            date,
		Amount:          formatMinorAmount(mainAmountMinor(out.Item)),
		Description:     statementDescription(out.Item.Description),
		SourceName:      out.Account.Firefly3Name,
		DestinationName: in.Account.Firefly3Name,
		ExternalId:      statementTransferExternalID(out, in),
		Notes:           "Matched unique Monobank MCC 4829 statement pair",
	}
}

func statementDescription(description string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		return "Monobank account transfer"
	}
	return description
}

func statementTransferExternalID(out, in statementEntry) string {
	ids := []string{monobankExternalID(out.Account.MonobankId, out.Item.Id), monobankExternalID(in.Account.MonobankId, in.Item.Id)}
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(ids[0] + "\x00" + ids[1]))
	return fmt.Sprintf("monobank:transfer:%s", hex.EncodeToString(sum[:]))
}
