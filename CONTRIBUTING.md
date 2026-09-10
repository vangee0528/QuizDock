# Contributing

Thank you for contributing to QuizDock.

1. Create a focused branch from `main`.
2. Run `make setup` once and `make check` before submitting a change.
3. Add tests for parser, database or interaction changes.
4. Keep application changes independent from proprietary question-bank content.
5. Update `CHANGELOG.md` for user-visible changes.

Question-bank format changes require a new `schema_version` and a migration/compatibility note. Database schema changes must be added as a new numbered migration; existing migrations are immutable after release.
