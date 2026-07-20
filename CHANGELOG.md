# Changelog

## Unreleased

## 0.3.0 - 2026-07-20

- Isolate saved OAuth and manual-token credentials by configured registry.
- Select configuration values through `--registry` and `--token` flags on
  `nore config get`, `set`, `unset`, and `reset`.
- Remove the temporary `login --registry` override.
- Keep request routing fixed to the configured registry during authentication,
  token refresh, retries, and logout.
- Avoid exposing the configured API endpoint in authentication and API command
  output.
- Migrate existing OAuth credentials into registry namespaces while requiring
  unscoped manual tokens to be configured again explicitly.
- Prompt for a site in interactive terminals when `--site` is omitted.
- Use a conventional, default-safe `[y/N]` confirmation for `nore logout`.
- Avoid changing persisted configuration and credential file modes during
  normal CLI access.

## 0.1.0 - 2026-07-17

- Create the standalone native Nore CLI.
