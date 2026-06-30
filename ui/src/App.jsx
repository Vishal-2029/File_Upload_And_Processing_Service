import { BrowserRouter, Routes, Route } from 'react-router-dom'
import UploadPage from './pages/UploadPage.jsx'
import EditorPage from './pages/EditorPage.jsx'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<UploadPage />} />
        <Route path="/editor/:fileId" element={<EditorPage />} />
      </Routes>
    </BrowserRouter>
  )
}
