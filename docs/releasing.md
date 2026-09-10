# Releasing QuizDock

QuizDock uses Semantic Versioning. The application and the bundled optional question-bank artifact use the same version in v0.1.x; the `.qbank` manifest remains the authority for its own version.

## Prepare

1. Update `VERSION`, `web/package.json`, the bank `manifest.json` and `CHANGELOG.md`.
2. Run `make check`.
3. Build and validate the bank:

   ```bash
   ./dist/quizdock bank pack banks/software-designer release/software-designer-0.1.0.qbank
   ./dist/quizdock bank validate release/software-designer-0.1.0.qbank
   ```

4. Test a clean import and verify that the application starts with an empty database.

## Publish

Create and push an annotated tag:

```bash
git tag -a v0.1.0 -m "QuizDock v0.1.0"
git push origin main v0.1.0
```

The Release workflow performs GoReleaser cross-compilation, publishes the `linux/amd64` and `linux/arm64` container image, builds the `.qbank`, attaches it to the GitHub Release and emits SHA-256 checksums for executable archives.

Do not publish the optional question bank while its manifest licence is `Unspecified`; application-only releases can temporarily disable the two question-bank steps in the workflow.
