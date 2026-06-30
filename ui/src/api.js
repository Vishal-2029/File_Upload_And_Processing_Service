const BASE = '/api'
const DEMO_EMAIL = 'demo@pdfeditor.local'
const DEMO_PASS  = 'demo123456'

export async function getToken() {
  const stored = localStorage.getItem('pdf_token')
  if (stored) return stored
  return _login()
}

async function _login() {
  // Try register first; if email already exists fall through to login.
  const regRes = await fetch(`${BASE}/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: DEMO_EMAIL, password: DEMO_PASS })
  })
  const regData = await regRes.json()
  if (regData.token) {
    localStorage.setItem('pdf_token', regData.token)
    return regData.token
  }

  const loginRes = await fetch(`${BASE}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: DEMO_EMAIL, password: DEMO_PASS })
  })
  const loginData = await loginRes.json()
  if (!loginData.token) throw new Error('Could not authenticate with backend.')
  localStorage.setItem('pdf_token', loginData.token)
  return loginData.token
}

export async function uploadFile(token, file) {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch(`${BASE}/upload`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: form
  })
  if (!res.ok) {
    const d = await res.json().catch(() => ({}))
    throw new Error(d.error || 'Upload failed')
  }
  return res.json()
}

export async function getFile(token, fileId) {
  const res = await fetch(`${BASE}/files/${fileId}`, {
    headers: { Authorization: `Bearer ${token}` }
  })
  if (!res.ok) throw new Error('Failed to load file info')
  return res.json()
}

export async function getPages(token, fileId) {
  const res = await fetch(`${BASE}/files/${fileId}/pages`, {
    headers: { Authorization: `Bearer ${token}` }
  })
  if (!res.ok) throw new Error('Failed to load pages')
  return res.json()
}

export async function updatePage(token, fileId, pageNum, text) {
  const res = await fetch(`${BASE}/files/${fileId}/pages/${pageNum}`, {
    method: 'PUT',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({ text })
  })
  if (!res.ok) throw new Error('Failed to save page')
  return res.json()
}

export async function exportPDF(token, fileId) {
  const res = await fetch(`${BASE}/files/${fileId}/export-pdf`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` }
  })
  if (!res.ok) throw new Error('Export failed')
  return res.json()
}
