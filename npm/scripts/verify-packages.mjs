import { access, readFile } from "node:fs/promises"
import { join, resolve } from "node:path"

import { optionValue, readJson, supportedPlatforms } from "./lib.mjs"

const args = process.argv.slice(2)
const directory = resolve(optionValue(args, "--directory", "dist/npm"))
const release = await readJson(join(directory, "release-manifest.json"))
const rootDirectory = join(directory, release.root.directory)
const root = await readJson(join(rootDirectory, "package.json"))
const platforms = await readJson(join(rootDirectory, "platforms.json"))

if (root.name !== "@norehq/cli") throw new Error(`Unexpected root package: ${root.name}`)
if (root.version !== release.version) throw new Error("Root package version mismatch")
if (release.platforms.length !== supportedPlatforms.length) {
  throw new Error(`Expected ${supportedPlatforms.length} platform packages`)
}

for (const platform of release.platforms) {
  const packageDirectory = join(directory, platform.directory)
  const manifest = await readJson(join(packageDirectory, "package.json"))
  if (manifest.name !== platform.name || manifest.version !== release.version) {
    throw new Error(`Invalid platform manifest for ${platform.key}`)
  }
  if (platforms[platform.key] !== platform.name) {
    throw new Error(`Root platform mapping mismatch for ${platform.key}`)
  }
  if (root.optionalDependencies[platform.name] !== release.version) {
    throw new Error(`Missing optional dependency for ${platform.name}`)
  }
  await access(join(packageDirectory, "bin", platform.binary))
}

const wrapper = await readFile(join(rootDirectory, "bin", "nore.cjs"), "utf8")
if (!wrapper.startsWith("#!/usr/bin/env node")) throw new Error("Invalid npm wrapper")

console.log(`Verified ${release.platforms.length + 1} npm packages for ${release.version}`)
