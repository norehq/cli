import { spawnSync } from "node:child_process"
import { join } from "node:path"

const commandOutput = result =>
  [result.stdout, result.stderr].filter(Boolean).join("\n").trim()

export const versionPublished = (name, version, run = spawnSync) => {
  const result = run(
    "npm",
    ["view", `${name}@${version}`, "version", "--json"],
    { encoding: "utf8" },
  )
  if (result.error) throw result.error
  if (result.status === 0) {
    const published = JSON.parse(result.stdout)
    return published === version ||
      (Array.isArray(published) && published.includes(version))
  }

  const output = commandOutput(result)
  if (/\bE404\b|404 Not Found/.test(output)) return false

  throw new Error(`npm view failed for ${name}@${version}: ${output}`)
}

const publishPackage = (
  packageDirectory,
  name,
  release,
  run,
  log,
) => {
  if (versionPublished(name, release.version, run)) {
    log(`Skipping ${name}@${release.version}; it is already published`)
    return
  }

  const result = run(
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
  if (result.status !== 0) throw new Error(`npm publish failed for ${name}`)
}

export const publishRelease = (
  release,
  directory,
  { run = spawnSync, log = console.log } = {},
) => {
  for (const platform of release.platforms) {
    publishPackage(
      join(directory, platform.directory),
      platform.name,
      release,
      run,
      log,
    )
  }
  publishPackage(
    join(directory, release.root.directory),
    release.root.name,
    release,
    run,
    log,
  )
}
