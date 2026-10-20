// Renders assets/how-it-works.mmd to light and dark SVGs for the README.
// GitHub only draws Mermaid on its website, so the README embeds these
// images instead, which show up everywhere (mobile app, ghcr.io, editors).
//
//   npm run diagram
//
// Needs a Chromium for Playwright, like npm run screenshots.
import { readFile, writeFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright'

const assets = fileURLToPath(new URL('../../../../assets/', import.meta.url))
const mermaidJS = createRequire(import.meta.url).resolve('mermaid/dist/mermaid.min.js')
const source = await readFile(`${assets}how-it-works.mmd`, 'utf8')

// The web UI's palette (src/index.css).
const themes = {
  light: { bg: '#f1f0e9', panel: '#ffffff', line: '#ddd8cb', ink: '#1f2421', muted: '#746f62', pine: '#2f5d50', soft: '#e4ede9' },
  dark: { bg: '#16191a', panel: '#1d2220', line: '#33362f', ink: '#ede9df', muted: '#9c9788', pine: '#6fa593', soft: '#223530' },
}

const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined })
try {
  for (const [name, c] of Object.entries(themes)) {
    const page = await browser.newPage()
    await page.addScriptTag({ path: mermaidJS })
    const svg = await page.evaluate(
      async ({ source, c }) => {
        window.mermaid.initialize({
          startOnLoad: false,
          theme: 'base',
          // Plain SVG text instead of HTML labels, so the file renders as an <img>.
          htmlLabels: false,
          flowchart: { htmlLabels: false, curve: 'basis', padding: 14 },
          themeVariables: {
            fontFamily: '-apple-system, "Segoe UI", Helvetica, Arial, sans-serif',
            fontSize: '15px',
            background: 'transparent',
            primaryColor: c.panel,
            primaryBorderColor: c.line,
            primaryTextColor: c.ink,
            lineColor: c.muted,
            textColor: c.ink,
            edgeLabelBackground: c.bg,
            secondaryColor: c.soft,
            tertiaryColor: c.soft,
          },
          themeCSS: `
            .node rect, .node path { rx: 8; ry: 8; }
            #flowchart-S-1 rect, [id*="flowchart-S-"] rect, [id*="flowchart-S-"] path { fill: ${c.pine}; stroke: ${c.pine}; }
            [id*="flowchart-S-"] text, [id*="flowchart-S-"] tspan { fill: ${c.bg} !important; font-weight: 600; }
            .edgeLabel text, .edgeLabel tspan { fill: ${c.muted} !important; }
          `,
        })
        const { svg } = await window.mermaid.render('how-it-works', source)
        return svg
      },
      { source, c },
    )
    await writeFile(`${assets}how-it-works-${name}.svg`, svg)
    console.log(`wrote assets/how-it-works-${name}.svg`)
    await page.close()
  }
} finally {
  await browser.close()
}
