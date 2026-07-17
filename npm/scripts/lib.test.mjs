import assert from "node:assert/strict"
import test from "node:test"

import {
  defaultDistTag,
  normalizeVersion,
  platformKey,
  platformPackageName,
  supportedPlatforms,
} from "./lib.mjs"

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
