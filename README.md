# albert

`albert` is a CLI/TUI for finding worthwhile technical articles by topic. It
uses quality signals provided by communities instead of inventing its own
heuristic score:

- **Hacker News** — community upvotes from the Algolia Search API.
- **lobste.rs** — scores from its tag-based JSON endpoints.
- **GitHub** — related repositories to answer the question “has anyone built
  this before?”.

## Requirements

- Go 1.25+
- Internet access for searches
- A terminal that supports interactive applications when using the TUI

No API key is required for the basic Hacker News, lobste.rs, or GitHub search
requests.

## Installation

Clone the repository and build the binary:

```sh
git clone <repository-url>
cd albert
make build
```

The binary is created at `bin/albert`. To install it into `$GOPATH/bin`:

```sh
make install
```

## Usage

Open the TUI:

```sh
./bin/albert
./bin/albert "go scheduler"
```

Run a one-shot search and print the results to stdout:

```sh
./bin/albert search "go scheduler"
./bin/albert search -n 20 "distributed consensus"
./bin/albert search -sort relevance "claude code memory"
./bin/albert search -json "rust borrow checker"
```

Main options:

| Option | Default | Description |
| --- | ---: | --- |
| `-n` | TUI: 15, search: 10 | Number of results to display |
| `-timeout` | `10s` | Timeout for each HTTP request |
| `-sort` | `score` | `score` or `relevance` |
| `-json` | off | Print JSON; only available with `search` |

Use `-sort relevance` for new topics whose community scores have not had time
to form.

## TUI shortcuts

- `j` / `k`, `g` / `G`: move through the list
- `enter`: open the article
- `c`: open the discussion page
- `y`: copy the link
- `tab`: switch between articles and repositories
- `s`: change the sort mode
- `/`: enter a new topic
- `i`: edit the current topic
- `r`: search again
- `q`: quit

The TUI requires stdout to be a real terminal. When redirecting output or
running in a script, use `albert search` instead.

## Development

Run the tests and standard checks:

```sh
make test
make check
```

Run the TUI in a pseudo-terminal:

```sh
make build
SCENARIO=scripts/scenarios/basic.py \
  python3 scripts/tui-pty.py bin/albert 30 100
```

## Cache and local data

The application only caches lobste.rs tag metadata in the operating system's
standard cache directory, usually `~/Library/Caches/albert` on macOS or
`~/.cache/albert` on Linux. No database or repository configuration is
required.

## Known limitations

- Results depend on the coverage and ranking behavior of each source.
- Very narrow topics may return no results.
- GitHub search has a rate limit for unauthenticated requests.
- Current and future design notes are documented in [`plan.md`](plan.md).

## License

No license has been declared yet.
