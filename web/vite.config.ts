import { readFileSync } from 'node:fs'
import { defineConfig } from 'vite'

const read = (path: string) => readFileSync(new URL(path, import.meta.url), 'utf8').trim()
const guidance = read('../internal/clinical/model-guidance.txt')
const preview = read('../internal/clinical/preview.txt')
const escape = (s: string) => s.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;')

// These documents are bundled so the installation can explain its terms before sign-in.
const license = read('../LICENSE')
const localLinks: Record<string, string> = {
  '../LICENSE': '/license.txt',
  '../PRIVACY.md': '/privacy.html',
  'release.md': 'https://github.com/ctlplne/healthctl/blob/main/docs/release.md',
}
function noticePage(title: string, markdown: string) {
  const sections = markdown.split(/\n\s*\n/).map(block => {
    const heading = /^(#{1,2}) (.*)$/.exec(block)
    if (heading) {
      const tag = 'h' + heading[1].length
      return `<${tag}>${escape(heading[2])}</${tag}>`
    }
    const copy = escape(block).replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_match, label, target) => {
      const href = localLinks[target] || target
      if (!href.startsWith('/') && !href.startsWith('https://')) throw new Error('Unsupported notice link: ' + href)
      return `<a href="${href.replaceAll('"', '&quot;')}">${label}</a>`
    })
    return `<p>${copy}</p>`
  })
  return `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="referrer" content="no-referrer"><link rel="stylesheet" href="/privacy.css"><title>${title} · healthctl</title><main><p>${escape(preview)}</p>${sections.join('\n')}</main></html>`
}
const documents: Record<string, string> = {
  'privacy.html': noticePage('Privacy', read('../PRIVACY.md')),
  'use-notice.html': noticePage('About this experimental beta', read('../docs/use-notice.md')),
  'license.txt': license + '\n',
}

export default defineConfig({
  plugins: [{
    name: 'healthctl-notices',
    enforce: 'pre',
    transform(code, id) {
      if (!id.endsWith('/src/styles.css')) return
      return {code: code.replace(/(['"])__MODEL_GUIDANCE_PRINT__\1/, JSON.stringify(guidance)).replace(/(['"])__PREVIEW_NOTICE_PRINT__\1/, JSON.stringify(preview).replace(". Verify", ".\\a Verify")), map: null}
    },
    transformIndexHtml(html) {
      return html.replace('<!-- model-guidance -->', escape(guidance)).replace('<!-- preview-notice -->', escape(preview))
    },
    generateBundle() {
      for (const [fileName, source] of Object.entries(documents)) this.emitFile({type: 'asset', fileName, source})
    },
    configureServer(server) {
      for (const [fileName, source] of Object.entries(documents)) server.middlewares.use('/' + fileName, (_req, res) => {
        res.setHeader('Content-Type', fileName.endsWith('.txt') ? 'text/plain; charset=utf-8' : 'text/html; charset=utf-8')
        res.end(source)
      })
    },
  }],
})
