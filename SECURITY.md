# Security policy

Security fixes are supported for the latest QuizDock release.

Do not include private question banks, tokens, browsing profiles or production databases in bug reports. Report vulnerabilities privately to the repository maintainer before public disclosure.

The `.qbank` importer limits archive size, expanded size, entry count and individual files; rejects traversal paths, symlinks, encrypted entries, duplicate paths and unsupported assets; verifies SHA-256 checksums; and imports in one SQLite transaction.
