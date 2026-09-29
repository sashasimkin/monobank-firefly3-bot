package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gitea.stuzer.link/stuzer05/go-monobank"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

const (
	statementMaxRange = 31*24*time.Hour + time.Hour
	statementMaxItems = 500
	monobankMinDelay  = 61 * time.Second
)

type SyncState struct {
	LastSync map[string]int64 `json:"last_sync"`
	StartAt  map[string]int64 `json:"start_at,omitempty"`
}

// SyncMonobankTransactions imports recent statement rows and saves a per-account
// cursor only after that account has been processed successfully.
func SyncMonobankTransactions(ctx context.Context) error {
	statePath := os.Getenv("MONOBANK_SYNC_STATE_FILE")
	if statePath == "" {
		return errors.New("MONOBANK_SYNC_STATE_FILE is required for incremental sync")
	}
	state, err := readSyncState(statePath)
	if err != nil {
		return err
	}
	if state.StartAt == nil {
		state.StartAt = map[string]int64{}
	}
	now := time.Now().Unix()
	overlapHours := envInt64("MONOBANK_SYNC_OVERLAP_HOURS", 48)
	if overlapHours < 0 || overlapHours > 24*31 {
		return errors.New("invalid Monobank sync lookback configuration")
	}

	entries := make([]statementEntry, 0)
	accountItems := make(map[string]int)
	accountsToAdvance := make([]string, 0, len(App().Config.Accounts))
	for _, account := range App().Config.Accounts {
		if account.MonobankId == "" || account.Firefly3Name == "" {
			return errors.New("all configured accounts need Monobank and Firefly names")
		}
		from, initialized := state.LastSync[account.MonobankId]
		if !initialized {
			// First run establishes a cursor only. This avoids importing recent
			// transactions before the user has reviewed classification rules.
			state.LastSync[account.MonobankId] = now
			state.StartAt[account.MonobankId] = now
			if err := writeSyncState(statePath, state); err != nil {
				return err
			}
			log.Printf("Initialized Monobank sync cursor for %s; existing transactions were left untouched", account.Firefly3Name)
			continue
		}
		startAt, hasStartAt := state.StartAt[account.MonobankId]
		if !hasStartAt {
			// Older state files only carried LastSync; treat that checkpoint as
			// the earliest safe start instead of retroactively importing history.
			startAt = from
			state.StartAt[account.MonobankId] = startAt
		}
		from = syncWindowStart(from, startAt, overlapHours*int64(time.Hour/time.Second))
		minimumStart := now - int64(statementMaxRange/time.Second)
		if from < minimumStart {
			return fmt.Errorf("sync cursor for account %s is older than Monobank's statement window; run historical import before advancing it", account.Firefly3Name)
		}

		items, err := fetchStatementItems(ctx, account.MonobankId, from, now)
		if err != nil {
			return fmt.Errorf("fetch statement for %s: %w", account.Firefly3Name, err)
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Time < items[j].Time })
		for _, item := range items {
			entries = append(entries, statementEntry{Account: account, Item: item})
		}
		accountItems[account.MonobankId] = len(items)
		accountsToAdvance = append(accountsToAdvance, account.MonobankId)
	}

	importedByAccount, err := importStatementEntries(entries, App().Config)
	if err != nil {
		return fmt.Errorf("import Monobank statement batch: %w", err)
	}
	for _, account := range App().Config.Accounts {
		if _, ok := accountItems[account.MonobankId]; !ok {
			continue
		}
		log.Printf("Synced Monobank account %s: statement_items=%d, imported_transactions=%d", account.Firefly3Name, accountItems[account.MonobankId], importedByAccount[account.MonobankId])
	}
	for _, accountID := range accountsToAdvance {
		state.LastSync[accountID] = now
	}
	if err := writeSyncState(statePath, state); err != nil {
		return err
	}
	return nil
}

// ImportMonobankHistory imports statements from the requested date through now.
// Monobank limits each statement request to 31 days plus one hour.
func ImportMonobankHistory(ctx context.Context, from time.Time) error {
	if len(App().Config.TransactionTypes) == 0 {
		return errors.New("historical import requires configured transaction_types; add and verify MCC/merchant rules first")
	}
	if from.IsZero() || from.After(time.Now()) {
		return errors.New("historical import start date must be in the past")
	}
	end := time.Now().Unix()
	window, err := configuredTransferMatchWindow(App().Config)
	if err != nil {
		return err
	}
	chunkSize := int64(statementMaxRange/time.Second) - 1 - int64(window/time.Second)
	importedTotals := make(map[string]int)
	start := from.Unix()
	for start < end {
		chunkEnd := start + chunkSize
		if chunkEnd > end {
			chunkEnd = end
		}
		entries := make([]statementEntry, 0)
		isFinalChunk := chunkEnd >= end
		safeBatchEnd := chunkEnd
		if !isFinalChunk {
			safeBatchEnd -= int64(window / time.Second)
		}
		for _, account := range App().Config.Accounts {
			if account.MonobankId == "" || account.Firefly3Name == "" {
				return errors.New("all configured accounts need Monobank and Firefly names")
			}
			queryStart := start
			if queryStart > from.Unix() {
				queryStart -= int64(window / time.Second)
			}
			items, err := fetchStatementItems(ctx, account.MonobankId, queryStart, chunkEnd)
			if err != nil {
				return fmt.Errorf("fetch history for %s: %w", account.Firefly3Name, err)
			}
			for _, item := range items {
				if int64(item.Time) <= safeBatchEnd || isFinalChunk {
					entries = append(entries, statementEntry{Account: account, Item: item})
				}
			}
		}
		imported, err := importStatementEntries(entries, App().Config)
		if err != nil {
			return fmt.Errorf("import Monobank history batch: %w", err)
		}
		for accountID, count := range imported {
			importedTotals[accountID] += count
		}
		if chunkEnd >= end {
			break
		}
		start = chunkEnd - int64(window/time.Second) + 1
	}
	for _, account := range App().Config.Accounts {
		log.Printf("Imported Monobank history for %s: new_transactions=%d", account.Firefly3Name, importedTotals[account.MonobankId])
	}
	return nil
}

func fetchStatementItems(ctx context.Context, accountID string, from, to int64) ([]monobank.StatementItemsInner, error) {
	var all []monobank.StatementItemsInner
	seen := map[string]struct{}{}
	queryTo := to
	for {
		waitForMonobankRateLimit()
		items, _, err := App().MonobankClient.Api.PersonalStatementAccountFromToGet(ctx, os.Getenv("MONOBANK_TOKEN"), accountID, strconv.FormatInt(from, 10), strconv.FormatInt(queryTo, 10))
		if err != nil {
			return nil, err
		}
		oldest := int64(queryTo)
		for _, item := range items {
			if item.Id == "" {
				return nil, errors.New("Monobank statement item is missing its transaction ID")
			}
			if _, ok := seen[item.Id]; !ok {
				seen[item.Id] = struct{}{}
				all = append(all, item)
			}
			if int64(item.Time) < oldest {
				oldest = int64(item.Time)
			}
		}
		if len(items) < statementMaxItems {
			return all, nil
		}
		if oldest <= from {
			return nil, errors.New("Monobank returned 500 statement items at the requested time boundary")
		}
		queryTo = oldest - 1
	}
}

var lastStatementRequest time.Time
var statementRequestLock sync.Mutex

func waitForMonobankRateLimit() {
	statementRequestLock.Lock()
	defer statementRequestLock.Unlock()
	if !lastStatementRequest.IsZero() {
		wait := time.Until(lastStatementRequest.Add(monobankMinDelay))
		if wait > 0 {
			time.Sleep(wait)
		}
	}
	lastStatementRequest = time.Now()
}

func readSyncState(path string) (SyncState, error) {
	state := SyncState{LastSync: map[string]int64{}, StartAt: map[string]int64{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("read Monobank sync state: %w", err)
	}
	if state.LastSync == nil {
		state.LastSync = map[string]int64{}
	}
	if state.StartAt == nil {
		state.StartAt = map[string]int64{}
	}
	return state, nil
}

func syncWindowStart(lastSync, startAt, overlapSeconds int64) int64 {
	from := lastSync - overlapSeconds
	if from < startAt {
		return startAt
	}
	return from
}

func writeSyncState(path string, state SyncState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sync-state-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func envInt64(name string, fallback int64) int64 {
	value, err := strconv.ParseInt(os.Getenv(name), 10, 64)
	if err != nil {
		return fallback
	}
	return value
}
