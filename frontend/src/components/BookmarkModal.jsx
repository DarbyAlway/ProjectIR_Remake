import { useState } from 'react'
import { useAuth } from '../context/AuthContext'
import './BookmarkModal.css'

export default function BookmarkModal({ recipeId, onClose }) {
  const { folders, createFolder, addBookmark } = useAuth()
  const [newName, setNewName] = useState('')
  const [creating, setCreating] = useState(false)
  const [saving, setSaving] = useState(false)

  async function handleSelectFolder(folderId) {
    setSaving(true)
    await addBookmark(recipeId, folderId)
    setSaving(false)
    onClose()
  }

  async function handleCreate() {
    if (!newName.trim()) return
    setCreating(true)
    const folder = await createFolder(newName.trim())
    if (folder) {
      await addBookmark(recipeId, folder.id)
    }
    setCreating(false)
    onClose()
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <h3>Save to folder</h3>
          <button className="modal-close" onClick={onClose}>✕</button>
        </div>

        {folders.length > 0 && (
          <ul className="folder-list">
            {folders.map(f => (
              <li key={f.id}>
                <button
                  className="folder-item"
                  onClick={() => handleSelectFolder(f.id)}
                  disabled={saving}
                >
                  <span className="folder-icon">📁</span>
                  <span className="folder-name">{f.name}</span>
                  <span className="folder-count">{f.recipeIds?.length ?? 0}</span>
                </button>
              </li>
            ))}
          </ul>
        )}

        <div className="modal-divider" />

        <div className="new-folder">
          <input
            type="text"
            placeholder="New folder name..."
            value={newName}
            onChange={e => setNewName(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && handleCreate()}
            autoFocus={folders.length === 0}
          />
          <button
            className="create-btn"
            onClick={handleCreate}
            disabled={!newName.trim() || creating}
          >
            {creating ? '...' : '+ Create & Save'}
          </button>
        </div>
      </div>
    </div>
  )
}
