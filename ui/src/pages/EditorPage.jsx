import { useState, useEffect, useRef, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import * as pdfjsLib from 'pdfjs-dist'
import pdfWorkerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
import { PDFDocument, StandardFonts, rgb } from 'pdf-lib'
import { getToken } from '../api.js'

pdfjsLib.GlobalWorkerOptions.workerSrc = pdfWorkerUrl

const BASE = '/api'

/* Fetch the original PDF bytes same-origin (through the /api proxy). */
async function fetchRawPdf(token, fileId) {
  const res = await fetch(`${BASE}/files/${fileId}/raw`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) throw new Error('Failed to load the original PDF')
  return new Uint8Array(await res.arrayBuffer())
}

/* Fetch the original file name for the download. */
async function fetchFileName(token, fileId) {
  const res = await fetch(`${BASE}/files/${fileId}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) return 'document.pdf'
  const d = await res.json()
  return d.OriginalName || d.original_name || 'document.pdf'
}

/* Trigger a browser download of raw bytes. */
function downloadBytes(bytes, filename) {
  const blob = new Blob([bytes], { type: 'application/pdf' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 4000)
}

/*
 * One rendered PDF page: a canvas showing the real page, plus an absolutely
 * positioned, editable text layer sitting exactly on top of each text run.
 *
 * Text spans are transparent by default (the canvas shows the original glyphs).
 * Once a run is focused or edited, the span becomes opaque with a white
 * background so it visually covers the original — mirroring exactly what the
 * export step does (white-out old text, draw new text at the same spot).
 */
function PdfPageView({ pdfPage, pageIndex, scale, registerItems, onEdit, edits }) {
  const canvasRef = useRef(null)
  const [items, setItems] = useState([])
  const [dims, setDims] = useState({ w: 0, h: 0 })

  useEffect(() => {
    let cancelled = false
    let renderTask = null
    const viewport = pdfPage.getViewport({ scale })

    const run = async () => {
      const canvas = canvasRef.current
      if (!canvas) return
      canvas.width = Math.floor(viewport.width)
      canvas.height = Math.floor(viewport.height)
      setDims({ w: canvas.width, h: canvas.height })
      const ctx = canvas.getContext('2d')

      // Render the page bitmap. Keep the task so we can cancel it on cleanup
      // (StrictMode double-mount / zoom changes must not collide on one canvas).
      try {
        renderTask = pdfPage.render({ canvasContext: ctx, viewport })
        await renderTask.promise
      } catch (e) {
        if (e?.name === 'RenderingCancelledException') return
        // Any other render error: still try to build the text layer below.
      }
      if (cancelled) return

      // Build the editable text layer.
      let content
      try {
        content = await pdfPage.getTextContent()
      } catch {
        return
      }
      if (cancelled) return

      const styles = content.styles || {}
      const built = []
      for (const it of content.items) {
        if (typeof it.str !== 'string' || it.str.length === 0) continue
        // it.transform is in PDF user space (origin bottom-left, Y up).
        const screen = pdfjsLib.Util.transform(viewport.transform, it.transform)
        const fontHeightPx = Math.hypot(screen[2], screen[3])
        const left = screen[4]
        const top = screen[5] - fontHeightPx
        const fontSizePdf = Math.hypot(it.transform[2], it.transform[3])
        const st = styles[it.fontName] || {}
        // After render(), pdf.js registers each embedded font as an @font-face
        // whose family name is the item's fontName (e.g. "g_d0_f1"). Point the
        // editable span at it, with the generic family as a safe fallback.
        const genericFamily = st.fontFamily || 'sans-serif'
        built.push({
          str: it.str,
          left,
          top,
          fontHeightPx,
          widthPx: it.width * scale,
          cssFontFamily: `"${it.fontName}", ${genericFamily}`,
          genericFamily,
          // Geometry needed for pdf-lib export (PDF user space):
          pdfX: it.transform[4],
          pdfY: it.transform[5],
          pdfFontSize: fontSizePdf || fontHeightPx / scale,
          pdfWidth: it.width,
        })
      }
      setItems(built)
      registerItems(pageIndex, built)
    }

    run()

    return () => {
      cancelled = true
      if (renderTask) {
        try { renderTask.cancel() } catch { /* already settled */ }
      }
    }
  }, [pdfPage, scale, pageIndex, registerItems])

  return (
    <div className="pdf-page" style={{ width: dims.w, height: dims.h }}>
      <canvas ref={canvasRef} className="pdf-canvas" />
      <div className="pdf-textlayer">
        {items.map((it, i) => {
          const edited = edits?.[i]
          const value = edited !== undefined ? edited : it.str
          const isChanged = edited !== undefined && edited !== it.str
          return (
            <span
              key={i}
              className={`pdf-textrun${isChanged ? ' changed' : ''}`}
              contentEditable
              suppressContentEditableWarning
              spellCheck={false}
              style={{
                left: `${it.left}px`,
                top: `${it.top}px`,
                height: `${it.fontHeightPx}px`,
                fontSize: `${it.fontHeightPx}px`,
                lineHeight: `${it.fontHeightPx}px`,
                minWidth: `${it.widthPx}px`,
                fontFamily: it.cssFontFamily,
              }}
              onFocus={(e) => {
                // Select all so replacing a value (a number/amount) is quick.
                const range = document.createRange()
                range.selectNodeContents(e.currentTarget)
                const sel = window.getSelection()
                sel.removeAllRanges()
                sel.addRange(range)
              }}
              onBlur={(e) => {
                const next = e.currentTarget.textContent
                if (next !== value) onEdit(pageIndex, i, next)
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') { e.preventDefault(); e.currentTarget.blur() }
                if (e.key === 'Escape') {
                  e.currentTarget.textContent = it.str
                  e.currentTarget.blur()
                }
              }}
            >
              {value}
            </span>
          )
        })}
      </div>
    </div>
  )
}

export default function EditorPage() {
  const { fileId } = useParams()
  const navigate = useNavigate()

  const [token, setToken] = useState(null)
  const [pdfDoc, setPdfDoc] = useState(null)
  const [pages, setPages] = useState([])
  const [filename, setFilename] = useState('document.pdf')
  const [scale, setScale] = useState(1.3)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [exporting, setExporting] = useState(false)
  const [toast, setToast] = useState('')
  const [dirtyCount, setDirtyCount] = useState(0)

  // Original PDF bytes + per-page item geometry + edits, kept in refs so the
  // export can read them without re-rendering.
  const rawBytesRef = useRef(null)
  const itemsRef = useRef({})   // { [pageIndex]: [item, ...] }
  const editsRef = useRef({})   // { [pageIndex]: { [itemIndex]: newStr } }
  const [editsVersion, setEditsVersion] = useState(0)

  const showToast = (msg) => {
    setToast(msg)
    setTimeout(() => setToast(''), 2800)
  }

  useEffect(() => {
    getToken().then(setToken).catch(() => { setError('Auth error'); setLoading(false) })
  }, [])

  useEffect(() => {
    if (!token) return
    let cancelled = false
    ;(async () => {
      try {
        const [bytes, name] = await Promise.all([
          fetchRawPdf(token, fileId),
          fetchFileName(token, fileId),
        ])
        if (cancelled) return
        rawBytesRef.current = bytes.slice() // keep a pristine copy for export
        setFilename(name)
        const doc = await pdfjsLib.getDocument({ data: bytes }).promise
        if (cancelled) return
        setPdfDoc(doc)
        const list = []
        for (let i = 1; i <= doc.numPages; i++) {
          list.push(await doc.getPage(i))
        }
        if (cancelled) return
        setPages(list)
        setLoading(false)
      } catch (e) {
        if (!cancelled) { setError(e.message); setLoading(false) }
      }
    })()
    return () => { cancelled = true }
  }, [token, fileId])

  const registerItems = useCallback((pageIndex, built) => {
    itemsRef.current[pageIndex] = built
  }, [])

  const handleEdit = useCallback((pageIndex, itemIndex, newStr) => {
    const orig = itemsRef.current[pageIndex]?.[itemIndex]?.str
    editsRef.current[pageIndex] = editsRef.current[pageIndex] || {}
    if (newStr === orig) {
      delete editsRef.current[pageIndex][itemIndex]
    } else {
      editsRef.current[pageIndex][itemIndex] = newStr
    }
    // Recompute dirty count.
    let n = 0
    for (const p of Object.values(editsRef.current)) n += Object.keys(p).length
    setDirtyCount(n)
    setEditsVersion(v => v + 1)
  }, [])

  const handleExport = async () => {
    setExporting(true)
    try {
      const doc = await PDFDocument.load(rawBytesRef.current)
      const libPages = doc.getPages()

      // Lazily embed the three standard-font families we may need and pick the
      // nearest one for each run based on the category pdf.js reports.
      const fontCache = {}
      const fontFor = async (generic) => {
        let key = 'helvetica'
        if (/serif/.test(generic) && !/sans/.test(generic)) key = 'times'
        else if (/mono/.test(generic)) key = 'courier'
        if (!fontCache[key]) {
          const std = key === 'times' ? StandardFonts.TimesRoman
            : key === 'courier' ? StandardFonts.Courier
              : StandardFonts.Helvetica
          fontCache[key] = await doc.embedFont(std)
        }
        return fontCache[key]
      }

      for (const [pi, pageEdits] of Object.entries(editsRef.current)) {
        const page = libPages[pi]
        if (!page) continue
        const items = itemsRef.current[pi] || []
        for (const [ii, newStr] of Object.entries(pageEdits)) {
          const it = items[ii]
          if (!it) continue
          const size = it.pdfFontSize
          const font = await fontFor(it.genericFamily || 'sans-serif')
          // White-out the original text run.
          page.drawRectangle({
            x: it.pdfX - 1,
            y: it.pdfY - size * 0.28,
            width: Math.max(it.pdfWidth, font.widthOfTextAtSize(newStr, size)) + 2,
            height: size * 1.28,
            color: rgb(1, 1, 1),
          })
          // Draw the edited text at the same baseline.
          page.drawText(newStr, {
            x: it.pdfX,
            y: it.pdfY,
            size,
            font,
            color: rgb(0, 0, 0),
          })
        }
      }

      const out = await doc.save()
      const base = filename.replace(/\.pdf$/i, '')
      downloadBytes(out, `${base}-edited.pdf`)
      showToast('Edited PDF downloaded ✓')
    } catch (e) {
      showToast(`Export failed: ${e.message}`)
    } finally {
      setExporting(false)
    }
  }

  if (loading) return (
    <div className="full-center">
      <div className="spinner lg" />
      <p className="loading-msg">Loading document…</p>
    </div>
  )

  if (error) return (
    <div className="full-center">
      <p className="error-text">{error}</p>
      <button className="btn-back-plain" onClick={() => navigate('/')}>← Go back</button>
    </div>
  )

  return (
    <div className="editor-page">
      <header className="editor-header">
        <button className="btn-back" onClick={() => navigate('/')}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <polyline points="15 18 9 12 15 6" />
          </svg>
          Back
        </button>

        <span className="editor-filename" title={filename}>{filename}</span>

        <div className="editor-zoom">
          <button onClick={() => setScale(s => Math.max(0.6, +(s - 0.2).toFixed(2)))} title="Zoom out">−</button>
          <span>{Math.round(scale * 100)}%</span>
          <button onClick={() => setScale(s => Math.min(3, +(s + 0.2).toFixed(2)))} title="Zoom in">+</button>
        </div>

        <button className="btn-export" onClick={handleExport} disabled={exporting}>
          {exporting ? 'Exporting…' : (
            <>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
                <polyline points="7 10 12 15 17 10" />
                <line x1="12" y1="15" x2="12" y2="3" />
              </svg>
              Export PDF{dirtyCount > 0 ? ` (${dirtyCount})` : ''}
            </>
          )}
        </button>
      </header>

      <div className="editor-hint">
        Click any text on the page to edit it in place · <kbd>Enter</kbd> confirms · <kbd>Esc</kbd> cancels · the original layout is preserved
      </div>

      <div className="editor-body pdf-scroll">
        {pages.map((p, i) => (
          <PdfPageView
            key={i}
            pdfPage={p}
            pageIndex={i}
            scale={scale}
            registerItems={registerItems}
            onEdit={handleEdit}
            edits={editsRef.current[i]}
            _v={editsVersion}
          />
        ))}
      </div>

      {toast && <div className="toast">{toast}</div>}
    </div>
  )
}
