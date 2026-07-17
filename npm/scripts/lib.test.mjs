import assert from "node:assert/strict"
import test from "node:test"

import {
  defaultDistTag,
  normalizeVersion,
  platformKey,
  platformPackageName,
  supportedPlatforms,
} from "./lib.mjs"
import { publishRelease, versionPublished } from "./publish.mjs"

test("normalizes release versions and dist tags", () => {
  assert.equal(normalizeVersion("v1.2.3"), "1.2.3")
  assert.equal(defaultDistTag("1.2.3"), "latest")
  assert.equal(defaultDistTag("1.2.3-beta.1"), "next")
  assert.throws(() => normalizeVersion("latest"))
})

test("uses @norehq platform package names for all six targets", () => {
  const config = { platformPackagePrefix: "@norehq/cli" }
  assert.equal(supportedPlatforms.length, 6)
  assert.deepEqual(
    supportedPlatforms.map(platform => platformPackageName(config, platform)),
    supportedPlatforms.map(platform => `@norehq/cli-${platformKey(platform)}`),
  )
})

test("publishes missing packages and skips versions already on the registry", () => {
  const commands = []
  const messages = []
  const release = {
    version: "1.2.3",
    distTag: "latest",
    platforms: [
      {
        name: "@norehq/cli-darwin-arm64",
        directory: "packages/darwin-arm64",
      },
      {
        name: "@norehq/cli-linux-arm64",
        directory: "packages/linux-arm64",
      },
    ],
    root: { name: "@norehq/cli", directory: "packages/root" },
  }
  const run = (_command, args) => {
    commands.push(args)
    if (args[0] !== "view") return { status: 0 }
    if (args[1] === "@norehq/cli-darwin-arm64@1.2.3") {
      return { status: 0, stdout: '"1.2.3"', stderr: "" }
    }
    return { status: 1, stdout: "", stderr: "npm error code E404" }
  }

  publishRelease(release, "/release", {
    run,
    log: message => messages.push(message),
  })

  assert.deepEqual(
    commands.filter(args => args[0] === "publish").map(args => args[1]),
    ["/release/packages/linux-arm64", "/release/packages/root"],
  )
  assert.deepEqual(messages, [
    "Skipping @norehq/cli-darwin-arm64@1.2.3; it is already published",
  ])
})

test("does not treat registry failures as unpublished versions", () => {
  const run = () => ({
    status: 1,
    stdout: "",
    stderr: "npm error code E500",
  })

  assert.throws(
    () => versionPublished("@norehq/cli", "1.2.3", run),
    /npm view failed/,
  )
})
