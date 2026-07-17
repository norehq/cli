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

The initial publication and Trusted Publishing setup for all seven packages was
completed on July 17, 2026. The temporary bootstrap workflow, GitHub environment
secret, and npm token were removed after an OIDC-only prerelease succeeded.

npm can only configure a trusted publisher after a package exists. If a new
package is added later, temporarily reintroduce a manual bootstrap workflow that
uses a short-lived granular access token restricted to the `@norehq` scope.
Publish the new package once, configure its trusted publisher, verify an
OIDC-only publish, and then delete the workflow, secret, and token again.

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

The repository variable `NPM_PUBLISH_ENABLED=true` enables the regular release
job. The publish script checks the registry before each package, so rerunning a
partially completed release skips versions that are already present.

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
