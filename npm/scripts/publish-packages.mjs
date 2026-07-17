import { spawnSync } from "node:child_process"
import { join, resolve } from "node:path"

import { optionValue, readJson } from "./lib.mjs"

const args = process.argv.slice(2)
const directory = resolve(optionValue(args, "--directory", "dist/npm"))
const release = await readJson(join(directory, "release-manifest.json"))

const publish = packageDirectory => {
  const result = spawnSync(
    "npm",
    [
      "publish",
      packageDirectory,
      "--access",
      "public",
      "--tag",
      release.distTag,
      "--provenance",
    ],
    { stdio: "inherit" },
  )
  if (result.error) throw result.error
  if (result.status !== 0) throw new Error(`npm publish failed for ${packageDirectory}`)
}

for (const platform of release.platforms) {
  publish(join(directory, platform.directory))
}
publish(join(directory, release.root.directory))
