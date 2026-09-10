# Architecture

QuizDock is a local-first web application packaged as one Go executable. The application contains only the compiled frontend and database migrations; question content arrives through user-selected `.qbank` files. Official bank sources may live in this repository for coordinated releases, but they are not embedded in the application.

```text
Browser (React + TypeScript)
          │ JSON / multipart
          ▼
Go HTTP service
  ├── qbank validation and import
  ├── practice and review logic
  ├── embedded frontend
  └── SQLite store
```

## Boundaries

- `cmd/quizdock` contains CLI wiring and process lifecycle.
- `internal/api` owns HTTP transport and static frontend delivery.
- `internal/qbank` owns the public archive and Markdown formats.
- `internal/database` owns migrations, catalogue persistence and learning state.
- `web` owns browser interaction and refresh-safe session state.

## Persistent identity

A question is identified by `bank_id + question_id`. Publishers must keep both values stable when correcting or updating a question. Catalogue import runs in one SQLite transaction and never deletes attempts, progress, favourites or mastery rows.

## Navigation recovery

The active mode and question are represented by `/practice/:mode/:uid`. Filters are encoded in the query string, long-term progress is stored in SQLite and the current queue plus scroll offset are stored in `sessionStorage`. Reloading therefore restores the exact question, including a random session's order.

## Trust model

QuizDock is designed primarily for a trusted local machine. The server binds to `127.0.0.1` by default. Operators who expose it on a network should place it behind an authenticated reverse proxy.
