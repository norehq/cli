# Changelog

## Unreleased

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

## 0.1.0 - 2026-07-17

- Create the standalone native Nore CLI.
