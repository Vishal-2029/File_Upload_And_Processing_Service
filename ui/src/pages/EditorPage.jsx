import { useState, useEffect, useRef, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { getToken, getFile, getPages, updatePage, exportPDF } from '../api.js'

/* ── Single editable line ─────────────────────────────────────────────────── */
function Line({ text, lineIdx, isEditing, onStartEdit, onCommit }) {
  const inputRef = useRef(null)
  const [value, setValue] = useState(text)

  useEffect(() => { setValue(text) }, [text])
  useEffect(() => { if (isEditing) inputRef.current?.focus() }, [isEditing])

  const commit = () => onCommit(value)
  const handleKey = (e) => {
    if (e.key === 'Enter')  { e.preventDefault(); commit() }
    if (e.key === 'Escape') { setValue(text); onCommit(text) }
  }

  if (isEditing) {
    return (
      <input
        ref={inputRef}
        className="line-input"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onBlur={commit}
        onKeyDown={handleKey}
      />
    )
  }

  return (
    <div className="line-text" onClick={() => onStartEdit(lineIdx)} title="Click to edit">
      {text || <span className="line-empty">&nbsp;</span>}
    </div>
  )
}

/* ── One page block ───────────────────────────────────────────────────────── */
function PageBlock({ page, token, fileId, showToast }) {
  const [lines, setLines]         = useState((page.text || '').split('\n'))
  const [editingIdx, setEditingIdx] = useState(null)
  const [dirty, setDirty]         = useState(false)
  const [saving, setSaving]       = useState(false)

  const startEdit = useCallback((idx) => {
    setEditingIdx(idx)
  }, [])

  const commitEdit = useCallback((idx, newValue) => {
    setLines(prev => {
      const next = [...prev]
      if (next[idx] !== newValue) { next[idx] = newValue; setDirty(true) }
      return next
    })
    setEditingIdx(null)
  }, [])

  const savePage = async () => {
    setSaving(true)
    try {
      await updatePage(token, fileId, page.page, lines.join('\n'))
      setDirty(false)
      showToast(`Page ${page.page} saved ✓`)
    } catch {
      showToast('Save failed — please try again')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="page-block">
      <div className="page-header">
        <span className="page-label">Page {page.page}</span>
        {dirty && (
          <button
            className="btn-save-page"
            onClick={savePage}
            disabled={saving}
          >
            {saving ? 'Saving…' : 'Save page'}
          </button>
        )}
      </div>

      <div className="page-lines">
        {lines.map((line, idx) => (
          <Line
            key={idx}
            text={line}
            lineIdx={idx}
            isEditing={editingIdx === idx}
            onStartEdit={startEdit}
            onCommit={(val) => commitEdit(idx, val)}
          />
        ))}
      </div>
    </div>
  )
}

/* ── Editor page ──────────────────────────────────────────────────────────── */
export default function EditorPage() {
  const { fileId }  = useParams()
  const navigate    = useNavigate()
  const [token,     setToken]     = useState(null)
  const [pages,     setPages]     = useState([])
  const [filename,  setFilename]  = useState('')
  const [loading,   setLoading]   = useState(true)
  const [error,     setError]     = useState('')
  const [exporting, setExporting] = useState(false)
  const [toast,     setToast]     = useState('')

  const showToast = (msg) => {
    setToast(msg)
    setTimeout(() => setToast(''), 2800)
  }

  useEffect(() => {
    getToken().then(setToken).catch(() => setError('Auth error'))
  }, [])

  useEffect(() => {
    if (!token) return
    Promise.all([getPages(token, fileId), getFile(token, fileId)])
      .then(([pagesData, fileData]) => {
        setPages(pagesData.pages || [])
        // Go JSON without tags keeps PascalCase field names
        setFilename(fileData.OriginalName || fileData.original_name || 'document.pdf')
        setLoading(false)
      })
      .catch((e) => { setError(e.message); setLoading(false) })
  }, [token, fileId])

  const handleExport = async () => {
    setExporting(true)
    try {
      const data = await exportPDF(token, fileId)
      window.open(data.url, '_blank')
      showToast('PDF exported — download started ✓')
    } catch {
      showToast('Export failed — please try again')
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

      {/* Sticky top bar */}
      <header className="editor-header">
        <button className="btn-back" onClick={() => navigate('/')}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <polyline points="15 18 9 12 15 6"/>
          </svg>
          Back
        </button>

        <span className="editor-filename" title={filename}>{filename}</span>

        <button
          className="btn-export"
          onClick={handleExport}
          disabled={exporting}
        >
          {exporting
            ? 'Exporting…'
            : (
              <>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/>
                  <polyline points="7 10 12 15 17 10"/>
                  <line x1="12" y1="15" x2="12" y2="3"/>
                </svg>
                Export PDF
              </>
            )
          }
        </button>
      </header>

      {/* Hint bar */}
      <div className="editor-hint">
        Click any line to edit it &nbsp;·&nbsp; Press <kbd>Enter</kbd> to confirm &nbsp;·&nbsp; Press <kbd>Esc</kbd> to cancel
      </div>

      {/* Pages */}
      <div className="editor-body">
        {pages.length === 0 && (
          <div className="empty-state">No text could be extracted from this PDF.</div>
        )}
        {pages.map(page => (
          <PageBlock
            key={page.page}
            page={page}
            token={token}
            fileId={fileId}
            showToast={showToast}
          />
        ))}
      </div>

      {/* Toast */}
      {toast && <div className="toast">{toast}</div>}
    </div>
  )
}
