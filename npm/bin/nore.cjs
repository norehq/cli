#!/usr/bin/env node

const { spawnSync } = require("node:child_process")
const { dirname, join } = require("node:path")

const platforms = require("../platforms.json")
const platformKey = `${process.platform}-${process.arch}`
const packageName = platforms[platformKey]

if (!packageName) {
  console.error(
    `Nore CLI does not provide an npm binary for ${process.platform}/${process.arch}.`,
  )
  process.exit(1)
}

let packageManifest

try {
  packageManifest = require.resolve(`${packageName}/package.json`)
} catch {
  console.error(
    [
      `The optional package ${packageName} is required for this platform but was not installed.`,
      "Reinstall without --omit=optional and verify that your package manager retained optional dependencies.",
    ].join("\n"),
  )
  process.exit(1)
}

const binaryName = process.platform === "win32" ? "nore.exe" : "nore"
const binaryPath = join(dirname(packageManifest), "bin", binaryName)
const result = spawnSync(binaryPath, process.argv.slice(2), {
  env: process.env,
  stdio: "inherit",
  windowsHide: false,
})

if (result.error) {
  console.error(`Unable to start Nore CLI: ${result.error.message}`)
  process.exit(1)
}

if (result.signal) process.kill(process.pid, result.signal)
process.exit(result.status ?? 1)
