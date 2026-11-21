import { readFileSync } from 'node:fs'
import { defineConfig } from 'vite'

// Use the same installation guidance before authentication, during loading and in print.
const guidance = readFileSync(
  new URL('../internal/clinical/model-guidance.txt', import.meta.url),
  'utf8',
).trim()
const htmlText = guidance
  .replaceAll('&', '&amp;')
  .replaceAll('<', '&lt;')
  .replaceAll('>', '&gt;')
export default defineConfig({
  plugins: [
    {
      name: 'clinical-model-guidance',
      enforce: 'pre',
      transform(code, id) {
        if (!id.endsWith('/src/styles.css')) return
        return {
          code: code.replace(
            /(['"])__MODEL_GUIDANCE_PRINT__\1/,
            JSON.stringify(guidance),
          ),
          map: null,
        }
      },
      transformIndexHtml(html) {
        return html.replace('<!-- model-guidance -->', htmlText)
      },
    },
  ],
})
