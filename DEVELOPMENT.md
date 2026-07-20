# CLI development

Nore CLI is a standalone Go application. Run commands from the repository
root.

## Requirements

- Go 1.25.8 or newer
- [Task](https://taskfile.dev/)
- GoReleaser 2.17.0 or newer for release checks
- Node.js 22.14.0 or newer for npm release tooling

## Quality checks

```sh
task
```

This runs goimports, Staticcheck, and all Go tests. Test the npm packaging
helpers separately:

```sh
task npm-test
```

## Running from source

```sh
go run ./cmd/nore --help
```

For local API development:

```sh
go run ./cmd/nore config set --registry http://127.0.0.1:3001
go run ./cmd/nore login
```

The registry is configuration, not a per-command override. Restore the
production API when local testing is complete:

```sh
go run ./cmd/nore config set --registry https://api.nore.sh
```

## Releases

Validate release configuration without publishing:

```sh
task release-check
```

Build a local release matrix without uploading it:

```sh
task release-snapshot
```

Pushing an annotated `v*` tag publishes the GitHub Release, six native npm
platform packages, and the `@norehq/cli` root package. npm Trusted Publishing
must be configured for the `norehq/cli` repository, `release.yml` workflow,
and `npm` GitHub environment.
