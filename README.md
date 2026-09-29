# Monobank - Firefly3 bot

This is a public GitHub mirror of the upstream project at
<https://gitea.stuzer.link/stuzer05/monobank-firefly3-bot>. The upstream project
is the source of truth for application code.

The GitHub Actions workflow tests changes and publishes multi-architecture
container images to `ghcr.io/sashasimkin/monobank-firefly3-bot` on pushes to
`master` and version tags. Images are tagged `latest`, `sha-<commit>`, and with
the version tag when one is pushed.

This bot is used to automatically log transactions from Monobank (via webhook) to [Firefly3](https://www.firefly-iii.org/).

Bot creates firefly3 transactions which are meant to be further processed by Firefly3 rules

## Installation

```sh
git clone https://gitea.stuzer.link/stuzer05/monobank-firefly3-bot.git

cd monobank-firefly3-bot

make
```

## Configuration

create `.env` file and `config.json`
```sh
cp .env.example .env
cp config.json.example config.json
```

set credential in `.env`

configure accounts in `config.json`

configure which transactions to match (by name or mcc codes) in `config.json`

## Run

you only need `.env`, `config.json` and build binary to run the bot

```sh
./monobank-firefly3-bot
```

bot will automatically register Monobank webhook url and start listening for incoming transactions

For incremental statement polling, run `./monobank-firefly3-bot --monobank-sync`.
Set `MONOBANK_SYNC_STATE_FILE` to a persistent writable file; the command saves
per-account cursors only after a successful import. It overlaps the saved cursor
to catch late rows and uses Monobank transaction IDs as Firefly `external_id`
values for repeat-safe imports. Monobank statement requests are limited to one
per minute and each request returns at most 500 rows.

To import older statements explicitly, run
`./monobank-firefly3-bot --monobank-import-history=YYYY-MM-DD`. Configure and
review `transaction_types` (including MCC rules) before using this command.
History is read in statement-window chunks and may take a long time because of
Monobank's rate limit. The command does not change the incremental poll cursor.

Set `import_unmatched_transactions` to `true` to import new rows that do not
match a configured rule under the `Uncategorized` category. This is useful for
new transactions while MCC rules are being prepared; historical imports should
wait until rules are ready.

## Usage

to get monobank account ids use `--monobank-list-accounts` command
```sh
./monobank-firefly3-bot --monobank-list-accounts
0xzGO4sgEGXXXXXXqqSTJQ  537541******3946
wp6M2Ln7nkXXXXXXVYCCpA  444111******7344
4723djMLsLXXXXXXYjxqRw  444111******3747
```

to get firefly3 account ids use `--firefly3-list-accounts` command
```sh
./monobank-firefly3-bot --firefly3-list-accounts
1     Mono black
2     Wallet cash (UAH)
3     Mono white
4     PrivatBank virtual
```

## API docs
- https://api-docs.firefly-iii.org
- https://api.monobank.ua/docs/index.html
