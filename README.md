# Nore CLI

[nore.sh](https://nore.sh)

Nore CLI connects your terminal or coding agent to Nore. Use it to sign in,
inspect sites and posts, create releases, follow publishing logs, and install
the bundled Nore skill for supported coding agents.

## Install

### npm (recommended)

```sh
npm install --global @norehq/cli
```

This installs the native binary for your platform. Node.js 22.14 or later is
required.

### macOS and Linux

```sh
curl -fsSL https://nore.sh/install.sh | sh
```

The installer downloads the latest GitHub Release, verifies its checksum, and
installs `nore` into `~/.local/bin`.

### Windows PowerShell

```powershell
irm https://nore.sh/install.ps1 | iex
```

The installer supports Windows PowerShell 5.1 and PowerShell 7, verifies the
download, and adds Nore CLI to your user `PATH`.

## Update

```sh
nore update
```

The command checks the latest stable GitHub release. npm installations receive
the matching npm, Yarn, pnpm, or Bun update command; native macOS and Linux
installations update in place; Windows installations receive the PowerShell
command to run.

## Get started

```sh
nore login
nore site list
nore post list --site <site>
nore release create --site <site>
```

`nore login` opens a browser-assisted sign-in flow. Replace `<site>` with a site
UUID or ident. When you create a release, the CLI follows its publishing logs
and reports the final result.

## Use with coding agents

Install or update the bundled Nore skill for detected coding agents:

```sh
nore skill update
```

Pass `--client` to target a specific agent. The installed skill teaches the
agent how to work with Nore through the CLI and its machine-readable JSON
output.

## Automation and CI

Provide a Nore Person Token through the environment when an interactive browser
login is not available:

```sh
export NORE_TOKEN="nore_pat_..."
nore site list --json
```

`NORE_TOKEN` takes precedence over locally saved credentials. Add `--json` when
another program or agent will consume the output.

## Help

```sh
nore --help
nore release --help
```
