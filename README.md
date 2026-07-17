# Nore CLI

Nore CLI manages Nore sites, posts, and releases from a terminal or AI agent.
It is a standalone Go application distributed as native binaries and through
the `@norehq/cli` npm package.

## Install

```sh
npm install --global @norehq/cli
```

## Quick start

```sh
nore login
nore skill update --client codex
nore whoami
nore site list
nore post list --site <site>
nore release create --site <site>
```

`<site>` can be a site UUID or ident. Release creation follows publishing logs
for up to three minutes unless `--non-interactive` is passed.

## Agent skill

Install the skill bundled with the CLI for every detected supported coding
agent:

```sh
nore skill update
```

Pass `--client codex`, `--client claude`, `--client cursor`, or `--client all`
to select targets explicitly. Updating replaces the installed user-level Nore
skill with the version bundled in the current CLI. Inspect that source without
installing it with `nore skill show`; both commands support `--json`.

## Authentication

`nore login` opens a browser-assisted PKCE flow and saves a 30-day grant. Use
`nore logout` to revoke that grant and remove it locally.

For CI or an agent, provide a Person Token without writing it to disk:

```sh
export NORE_TOKEN="nore_pat_..."
nore site list --json
```

To save a Person Token locally:

```sh
nore config set token "nore_pat_..."
nore config unset token
```

Token precedence is `NORE_TOKEN`, saved Person Token, then browser login.

## Commands

| Command                                                | Purpose                                       |
| ------------------------------------------------------ | --------------------------------------------- |
| `nore login`                                           | Sign in through the browser                   |
| `nore logout`                                          | Revoke the browser grant and clear it locally |
| `nore whoami`                                          | Show the authenticated identity               |
| `nore config show`                                     | Show paths and effective registry             |
| `nore config set token <token>`                        | Save a Person Token                           |
| `nore config unset token`                              | Remove the saved Person Token                 |
| `nore skill update [--client <client>]`                | Install or update the bundled agent skill     |
| `nore skill show`                                      | Print the bundled agent skill                 |
| `nore site list`                                       | List authorized sites                         |
| `nore site get --site <site>`                          | Inspect a site                                |
| `nore post list --site <site>`                         | Browse and search posts                       |
| `nore post get --site <site> --post <id>`              | Inspect a post                                |
| `nore release create --site <site>`                    | Create and follow a release                   |
| `nore release list --site <site>`                      | List release history                          |
| `nore release get --site <site> --release <id> --logs` | Inspect a release and its logs                |

Add `--json` for machine-readable output. Release failures exit with status 1;
a follow timeout exits with status 2.
