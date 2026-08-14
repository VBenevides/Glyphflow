import { readdirSync, readFileSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('../src', import.meta.url))
const violations = []

function visit(directory) {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) visit(path)
    else if (/\.(tsx?|jsx?)$/.test(entry.name)) check(path, readFileSync(path, 'utf8'))
  }
}

function check(path, source) {
  const name = relative(root, path)
  if (!/\.test\.[jt]sx?$/.test(name) && /\bconsole\.(debug|error|info|log|warn)\s*\(/.test(source)) {
    violations.push(`${name}: remove console output from production code`)
  }
  for (const match of source.matchAll(/<img\b[^>]*>/gi)) {
    if (!/\balt\s*=/.test(match[0])) violations.push(`${name}: every img element needs alt text`)
  }
  for (const match of source.matchAll(/<a\b[^>]*target\s*=\s*["']_blank["'][^>]*>/gi)) {
    if (!/\brel\s*=/.test(match[0])) violations.push(`${name}: target=_blank links need rel`)
  }
}

visit(root)
if (violations.length) {
  console.error(violations.join('\n'))
  process.exitCode = 1
} else {
  console.log(`quality checks passed (${relative(process.cwd(), root)})`)
}
