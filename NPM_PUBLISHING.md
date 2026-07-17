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
- Allowed action: `npm publish`

The GitHub `npm` environment should permit tag-triggered releases. The workflow
uses an OIDC identity token and does not require a long-lived npm publish token.

## First publication

npm can only configure a trusted publisher after a package exists. Bootstrap all
seven packages once before enabling the regular release job:

1. Sign in to npm with an account that has publish access to the `norehq`
   organization and has two-factor authentication enabled.
2. Create a short-lived granular access token with read/write access to the
   `@norehq` scope and bypass 2FA enabled.
3. Save it temporarily as the `NPM_BOOTSTRAP_TOKEN` secret in the GitHub `npm`
   environment.
4. Run the `Bootstrap npm Packages` workflow for an existing GitHub Release tag.
5. Configure the GitHub Actions trusted publisher shown above for every package.
6. Delete the bootstrap secret and revoke its npm token after a trusted publish
   succeeds.

With npm 11.15 or newer, the trusted publishers can be configured in one login
session:

```sh
packages=(
  @norehq/cli
  @norehq/cli-darwin-x64
  @norehq/cli-darwin-arm64
  @norehq/cli-linux-x64
  @norehq/cli-linux-arm64
  @norehq/cli-win32-x64
  @norehq/cli-win32-arm64
)

for package in "${packages[@]}"; do
  npm trust github "$package" \
    --repo norehq/cli \
    --file release.yml \
    --env npm \
    --allow-publish \
    --yes
  sleep 2
done
```

Set the repository variable `NPM_PUBLISH_ENABLED=true` only after all seven
trusted publishers are configured. The publish script checks the registry before
each package, so rerunning a partially completed release skips versions that are
already present.

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
