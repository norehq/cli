import { createHash } from "node:crypto"
import { spawnSync } from "node:child_process"
import {
  access,
  chmod,
  copyFile,
  mkdir,
  readFile,
  readdir,
  rm,
  writeFile,
} from "node:fs/promises"
import { dirname, join, resolve } from "node:path"
import { fileURLToPath } from "node:url"

import {
  defaultDistTag,
  normalizeVersion,
  optionValue,
  platformKey,
  platformPackageName,
  readJson,
  repositoryFields,
  supportedPlatforms,
  writeJson,
} from "./lib.mjs"

const scriptDirectory = dirname(fileURLToPath(import.meta.url))
const repositoryRoot = resolve(scriptDirectory, "../..")
const args = process.argv.slice(2)
const versionInput = optionValue(args, "--version")

if (!versionInput) throw new Error("Usage: build-packages.mjs --version <version-or-tag>")

const version = normalizeVersion(versionInput)
const assetsDirectory = resolve(repositoryRoot, optionValue(args, "--assets", "dist"))
const outputDirectory = resolve(repositoryRoot, optionValue(args, "--output", "dist/npm"))
const config = await readJson(
  resolve(repositoryRoot, optionValue(args, "--config", "npm/release.json")),
)

const sha256 = async path =>
  createHash("sha256").update(await readFile(path)).digest("hex")

const readChecksums = async () => {
  const contents = await readFile(join(assetsDirectory, "checksums.txt"), "utf8")
  return new Map(
    contents
      .trim()
      .split("\n")
      .map(line => {
        const match = line.match(/^([a-f0-9]{64})\s+\*?(.+)$/i)
        if (!match) throw new Error(`Invalid checksum line: ${line}`)
        return [match[2], match[1].toLowerCase()]
      }),
  )
}

const findArchive = (platform, files) => {
  const suffix = `_${platform.goos}_${platform.goarch}.${platform.archiveExtension}`
  const matches = files.filter(file => file.endsWith(suffix))
  if (matches.length !== 1) {
    throw new Error(`Expected one archive ending in ${suffix}, found ${matches.length}`)
  }
  return matches[0]
}

const extractBinary = (archivePath, platform) => {
  const [command, commandArgs] =
    platform.archiveExtension === "zip"
      ? ["unzip", ["-p", archivePath, platform.binary]]
      : ["tar", ["-xOzf", archivePath, platform.binary]]
  const result = spawnSync(command, commandArgs, {
    encoding: null,
    maxBuffer: 64 * 1024 * 1024,
  })
  if (result.error) throw result.error
  if (result.status !== 0) {
    throw new Error(
      `Unable to extract ${platform.binary}: ${result.stderr?.toString() ?? ""}`,
    )
  }
  return result.stdout
}

const packageDirectory = name => join(outputDirectory, "packages", name)

await access(join(assetsDirectory, "checksums.txt"))
await rm(outputDirectory, { recursive: true, force: true })
await mkdir(join(outputDirectory, "packages"), { recursive: true })

const files = await readdir(assetsDirectory)
const checksums = await readChecksums()
const platforms = []

for (const platform of supportedPlatforms) {
  const archiveName = findArchive(platform, files)
  const archivePath = join(assetsDirectory, archiveName)
  const expectedChecksum = checksums.get(archiveName)
  if (!expectedChecksum) throw new Error(`Missing checksum for ${archiveName}`)
  const actualChecksum = await sha256(archivePath)
  if (actualChecksum !== expectedChecksum) {
    throw new Error(`Checksum mismatch for ${archiveName}`)
  }

  const key = platformKey(platform)
  const name = platformPackageName(config, platform)
  const directory = packageDirectory(key)
  const binaryDirectory = join(directory, "bin")
  const binaryPath = join(binaryDirectory, platform.binary)
  const manifest = {
    name,
    version,
    description: `${config.description} (${platform.os}/${platform.cpu} binary)`,
    license: config.license,
    ...repositoryFields(config),
    os: [platform.os],
    cpu: [platform.cpu],
    files: ["bin", "LICENSE"],
    publishConfig: { access: "public", registry: config.registry },
  }

  await mkdir(binaryDirectory, { recursive: true })
  await writeFile(binaryPath, extractBinary(archivePath, platform))
  await chmod(binaryPath, 0o755)
  await copyFile(join(repositoryRoot, "LICENSE"), join(directory, "LICENSE"))
  await writeJson(join(directory, "package.json"), manifest)
  await writeFile(
    join(directory, "README.md"),
    `# ${name}\n\nThis package contains one platform binary for Nore CLI. Install \`${config.rootPackage}\` instead.\n`,
  )
  platforms.push({ ...platform, key, name, directory: `packages/${key}` })
}

const rootDirectory = packageDirectory("root")
const rootBinaryDirectory = join(rootDirectory, "bin")
const optionalDependencies = Object.fromEntries(
  platforms.map(platform => [platform.name, version]),
)
const platformPackages = Object.fromEntries(
  platforms.map(platform => [platform.key, platform.name]),
)
const rootManifest = {
  name: config.rootPackage,
  version,
  description: config.description,
  license: config.license,
  ...repositoryFields(config),
  keywords: ["cli", "nore", "notion", "publishing"],
  bin: { [config.command]: "bin/nore.cjs" },
  files: ["bin", "platforms.json", "LICENSE"],
  engines: { node: config.node },
  optionalDependencies,
  publishConfig: { access: "public", registry: config.registry },
}

await mkdir(rootBinaryDirectory, { recursive: true })
await copyFile(join(repositoryRoot, "npm/bin/nore.cjs"), join(rootBinaryDirectory, "nore.cjs"))
await chmod(join(rootBinaryDirectory, "nore.cjs"), 0o755)
await copyFile(join(repositoryRoot, "LICENSE"), join(rootDirectory, "LICENSE"))
await writeFile(
  join(rootDirectory, "README.md"),
  `# Nore CLI\n\nInstall globally with \`npm install --global ${config.rootPackage}\`.\n\nAfter installation, install the bundled coding-agent skill with \`nore skill update\`.\n`,
)
await writeJson(join(rootDirectory, "package.json"), rootManifest)
await writeJson(join(rootDirectory, "platforms.json"), platformPackages)
await writeJson(join(outputDirectory, "release-manifest.json"), {
  version,
  distTag: defaultDistTag(version),
  registry: config.registry,
  root: { name: config.rootPackage, directory: "packages/root" },
  platforms,
})

console.log(`Prepared 7 npm packages for ${version} in ${outputDirectory}`)
