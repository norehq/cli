# npm publishing

Nore CLI publishes one public root package and six public platform packages:

- `@norehq/cli`
- `@norehq/cli-darwin-x64`
- `@norehq/cli-darwin-arm64`
- `@norehq/cli-linux-x64`
- `@norehq/cli-linux-arm64`
- `@norehq/cli-win32-x64`
- `@norehq/cli-win32-arm64`

Configure npm Trusted Publishing for every package with these values:

- Organization or user: `norehq`
- Repository: `cli`
- Workflow: `release.yml`
- Environment: `npm`

The GitHub `npm` environment should permit tag-triggered releases. The workflow
uses an OIDC identity token and does not require a long-lived npm publish token.

The release job publishes platform packages first and the root package last,
so `@norehq/cli` never references platform versions that have not been
published. Stable semantic versions use the `latest` dist-tag; prereleases use
`next`.

Before the first release, validate the release metadata and npm helper tests:

```sh
task release-check
```

To inspect packages without publishing, create a GoReleaser snapshot and then
build npm packages for an explicit version:

```sh
task release-snapshot
task npm-package-check VERSION=0.1.0
```
