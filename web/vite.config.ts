import { readFileSync } from 'node:fs'
import { defineConfig } from 'vite'

const read = (path: string) => readFileSync(new URL(path, import.meta.url), 'utf8').trim()
const guidance = read('../internal/clinical/model-guidance.txt')
const preview = read('../internal/clinical/preview.txt')
const escape = (s: string) => s.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;')

// The privacy notice is readable offline, before sign-in, without scripts.
const sections = read('../PRIVACY.md').split(/\n\s*\n/).map((block) => {
  const heading = /^(#{1,2}) (.*)$/.exec(block)
  if (!heading) return `<p>${escape(block)}</p>`
  const tag = `h${heading[1].length}`
  return `<${tag}>${escape(heading[2])}</${tag}>`
})
const privacy = `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><link rel="stylesheet" href="/privacy.css"><title>Privacy · healthctl</title><main><p>${escape(preview)}</p>${sections.join('\n')}</main></html>`

export default defineConfig({
  plugins: [{
    name: 'healthctl-notices',
    enforce: 'pre',
    transform(code, id) {
      if (!id.endsWith('/src/styles.css')) return
      return {code: code.replace(/(['"])__MODEL_GUIDANCE_PRINT__\1/, JSON.stringify(guidance)), map: null}
    },
    transformIndexHtml(html) {
      return html.replace('<!-- model-guidance -->', escape(guidance)).replace('<!-- preview-notice -->', escape(preview))
    },
    generateBundle() {
      this.emitFile({type: 'asset', fileName: 'privacy.html', source: privacy})
    },
    configureServer(server) {
      server.middlewares.use('/privacy.html', (_req, res) => {
        res.setHeader('Content-Type', 'text/html; charset=utf-8')
        res.end(privacy)
      })
    },
  }],
})
