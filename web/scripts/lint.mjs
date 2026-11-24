// Project rules the type checker can't express. Run with npm run lint.
import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'

const src = new URL('../src/', import.meta.url).pathname
const problems = []
for (const name of readdirSync(src)) {
  if (!/\.tsx?$/.test(name) || /\.test\.tsx?$/.test(name)) continue
  const text = readFileSync(join(src, name), 'utf8')
  text.split('\n').forEach((line, i) => {
    // One date formatter: dates go through date() in types.ts.
    if (name !== 'types.ts' && /\.toLocale(Date|Time)?String\(\)/.test(line)) problems.push(name + ':' + (i + 1) + ': use date() from types.ts')
    // No markup from strings (Trusted Types is enforced in the CSP).
    if (/dangerouslySetInnerHTML|\.innerHTML\s*=|insertAdjacentHTML/.test(line)) problems.push(name + ':' + (i + 1) + ': no HTML from strings')
  })
  // Clinical free text must not be offered to browser autofill.
  for (const m of text.matchAll(/<textarea\b[^>]*>/gs)) {
    if (!/autoComplete="off"/.test(m[0])) problems.push(name + ': a textarea without autoComplete="off"')
  }
}
if (problems.length) {
  console.error(problems.join('\n'))
  process.exit(1)
}
console.log('lint: project rules hold')
