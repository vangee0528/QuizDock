# QuizDock

QuizDock is a local-first, self-hosted practice application for importable question banks. The application is exam-agnostic: users install `.qbank` packages and can switch between, combine, update or remove them without replacing the application.

The v0.1.0 interface is Chinese. The project uses a Go backend, a React/TypeScript frontend and SQLite for portable persistence.

This repository also maintains the optional software-designer bank under `banks/software-designer`. It is packaged as a separate `.qbank` asset in each release and is never embedded in the executable or Docker image.

## Features

- Transactional `.qbank` import from the browser or CLI.
- Multiple installed and independently enabled question banks.
- Sequence, chapter, random, past-exam and wrong-answer review modes.
- Persistent attempts, favourites, progress and wrong-answer memory.
- Stable practice URLs and scroll restoration after refresh.
- Markdown questions with GFM tables, code blocks and local images.
- One-click copy-and-open for asking an AI website after a wrong answer.
- Docker, source and single-executable distributions.

## Run from source

Requirements: Go 1.27+, Node.js 22+ and npm 10+.

```bash
make setup
make dev
```

For a production build:

```bash
make build
./dist/quizdock serve
```

The default address is `http://127.0.0.1:8765`. Runtime data is stored under the operating system's user data directory unless `--data-dir` is supplied.

## Docker

```bash
docker compose up --build
```

Or run the published image:

```bash
docker run --rm -p 8765:8765 -v quizdock-data:/data ghcr.io/quizdock/quizdock:0.1.0
```

## Releases

Every `v*` Git tag creates three distribution paths from the same codebase:

1. Cross-platform single-executable archives for Linux, macOS and Windows.
2. A multi-architecture container image published to GitHub Container Registry.
3. The tagged source archive, buildable with `make build` or runnable with `make dev`.

The release also attaches `software-designer-<version>.qbank`. It is optional content selected by the user after QuizDock starts, not part of the executable or container image.

## Question-bank tools

```bash
quizdock bank validate software-designer.qbank
quizdock bank inspect software-designer.qbank
quizdock bank import software-designer.qbank
quizdock bank pack ./my-bank ./my-bank.qbank
```

See [docs/qbank-format.md](docs/qbank-format.md) for the package specification.

## Repository layout

```text
cmd/quizdock/                 CLI and server entry point
internal/api/                 HTTP API and embedded frontend delivery
internal/database/            SQLite migrations and persistence
internal/qbank/               qbank validation, parsing and packing
web/                          React and TypeScript frontend
banks/software-designer/      optional bank source, packaged separately
```

## Data safety

Imported content and learning state share one SQLite database, but content updates only upsert catalogue tables. Attempts, progress, favourites and wrong-answer memory are preserved across bank upgrades. Removing a bank preserves learning records by default unless explicitly requested otherwise.

## Development checks

```bash
make check
```

## Licence

QuizDock's application source is available under the MIT licence. Question-bank packages are released independently and must declare their own content licence in `manifest.json`.
