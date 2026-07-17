import { join, resolve } from "node:path"

import { optionValue, readJson } from "./lib.mjs"
import { publishRelease } from "./publish.mjs"

const args = process.argv.slice(2)
const directory = resolve(optionValue(args, "--directory", "dist/npm"))
const release = await readJson(join(directory, "release-manifest.json"))

publishRelease(release, directory)
