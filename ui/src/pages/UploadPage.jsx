import { useState, useCallback, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { getToken, uploadFile, getFile } from '../api.js'

export default function UploadPage() {
  const navigate  = useNavigate()
  const [token,   setToken]   = useState(null)
  const [dragging, setDragging] = useState(false)
  const [phase,   setPhase]   = useState('idle')   // idle | uploading | processing | error
  const [error,   setError]   = useState('')

  useEffect(() => {
    getToken()
      .then(setToken)
      .catch(() => setError('Cannot reach the backend. Is the server running on port 3000?'))
  }, [])

  const handleFile = useCallback(async (file) => {
    if (!file) return
    if (!file.name.toLowerCase().endsWith('.pdf')) {
      setError('Only PDF files are supported.')
      return
    }
    if (!token) {
      setError('Still initialising — please wait a moment.')
      return
    }

    setError('')
    setPhase('uploading')

    try {
      const { file_id } = await uploadFile(token, file)
      setPhase('processing')

      const pollId = setInterval(async () => {
        const info = await getFile(token, file_id)
        if (info.Status === 'done' || info.status === 'done') {
          clearInterval(pollId)
          navigate(`/editor/${file_id}`)
        } else if (info.Status === 'error' || info.status === 'error') {
          clearInterval(pollId)
          setPhase('error')
          setError('Processing failed. Please try a different PDF.')
        }
      }, 2000)
    } catch (e) {
      setPhase('error')
      setError(e.message)
    }
  }, [token, navigate])

  const onDrop = useCallback((e) => {
    e.preventDefault()
    setDragging(false)
    handleFile(e.dataTransfer.files[0])
  }, [handleFile])

  const retry = () => { setPhase('idle'); setError('') }

  return (
    <div className="upload-page">
      <div className="upload-card">

        {/* Logo / Title */}
        <div className="upload-hero">
          <div className="upload-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/>
              <polyline points="14 2 14 8 20 8"/>
              <line x1="12" y1="12" x2="12" y2="18"/>
              <polyline points="9 15 12 18 15 15"/>
            </svg>
          </div>
          <h1 className="upload-title">PDF Editor</h1>
          <p className="upload-sub">Upload a PDF — edit any line of text, then export a new PDF.</p>
        </div>

        {/* Drop zone */}
        {phase === 'idle' && (
          <label
            className={`drop-zone ${dragging ? 'dragging' : ''}`}
            onDragOver={(e) => { e.preventDefault(); setDragging(true) }}
            onDragLeave={() => setDragging(false)}
            onDrop={onDrop}
          >
            <svg className="drop-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
              <polyline points="16 16 12 12 8 16"/>
              <line x1="12" y1="12" x2="12" y2="21"/>
              <path d="M20.39 18.39A5 5 0 0018 9h-1.26A8 8 0 103 16.3"/>
            </svg>
            <span className="drop-primary">Drag &amp; drop your PDF here</span>
            <span className="drop-or">— or —</span>
            <span className="drop-btn">Browse file</span>
            <input
              type="file"
              accept=".pdf"
              style={{ display: 'none' }}
              onChange={(e) => handleFile(e.target.files[0])}
            />
          </label>
        )}

        {/* Uploading state */}
        {phase === 'uploading' && (
          <div className="status-box">
            <div className="spinner" />
            <p className="status-msg">Uploading PDF…</p>
          </div>
        )}

        {/* Processing state */}
        {phase === 'processing' && (
          <div className="status-box">
            <div className="spinner" />
            <p className="status-msg">Extracting text from PDF…</p>
            <p className="status-hint">This may take a few seconds</p>
          </div>
        )}

        {/* Error state */}
        {(phase === 'error' || error) && (
          <div className="error-block">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
              <circle cx="12" cy="12" r="10"/>
              <line x1="12" y1="8" x2="12" y2="12"/>
              <line x1="12" y1="16" x2="12.01" y2="16"/>
            </svg>
            <p>{error}</p>
            {phase === 'error' && (
              <button className="btn-retry" onClick={retry}>Try again</button>
            )}
          </div>
        )}

      </div>
    </div>
  )
}
