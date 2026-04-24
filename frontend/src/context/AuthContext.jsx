import { createContext, useContext, useState, useCallback } from 'react'

const AuthContext = createContext(null)

export function AuthProvider({ children }) {
  const [user, setUser] = useState(() => {
    const stored = localStorage.getItem('user')
    return stored ? JSON.parse(stored) : null
  })
  const [folders, setFolders] = useState([])

  function login(data) {
    const userData = { ...data, bookmarks: data.bookmarks ?? [] }
    localStorage.setItem('user', JSON.stringify(userData))
    setUser(userData)
    setFolders([])
  }

  function logout() {
    localStorage.removeItem('user')
    setUser(null)
    setFolders([])
  }

  const fetchFolders = useCallback(async (token) => {
    const t = token ?? user?.token
    if (!t) return
    const res = await fetch('/api/user/folders', {
      headers: { Authorization: `Bearer ${t}` },
    })
    if (res.ok) {
      const data = await res.json()
      setFolders(data ?? [])
    }
  }, [user?.token])

  async function createFolder(name) {
    const res = await fetch('/api/user/folders', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${user.token}` },
      body: JSON.stringify({ name }),
    })
    if (!res.ok) return null
    const folder = await res.json()
    setFolders(prev => [...prev, folder])
    return folder
  }

  async function addBookmark(recipeId, folderId) {
    const res = await fetch('/api/auth/bookmark', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${user.token}` },
      body: JSON.stringify({ recipeId, folderId }),
    })
    if (!res.ok) return
    const { bookmarks } = await res.json()
    const updated = { ...user, bookmarks }
    localStorage.setItem('user', JSON.stringify(updated))
    setUser(updated)
    setFolders(prev => prev.map(f =>
      f.id === folderId ? { ...f, recipeIds: [...(f.recipeIds ?? []), recipeId] } : f
    ))
  }

  async function removeBookmark(recipeId) {
    const res = await fetch('/api/auth/bookmark', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${user.token}` },
      body: JSON.stringify({ recipeId }),
    })
    if (!res.ok) return
    const { bookmarks } = await res.json()
    const updated = { ...user, bookmarks }
    localStorage.setItem('user', JSON.stringify(updated))
    setUser(updated)
    setFolders(prev => prev.map(f => ({
      ...f,
      recipeIds: (f.recipeIds ?? []).filter(id => id !== recipeId),
    })))
  }

  async function removeFromFolder(recipeId, folderId) {
    const res = await fetch(`/api/user/folders/${folderId}/remove`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${user.token}` },
      body: JSON.stringify({ recipeId }),
    })
    if (!res.ok) return
    const { bookmarks } = await res.json()
    const updated = { ...user, bookmarks }
    localStorage.setItem('user', JSON.stringify(updated))
    setUser(updated)
    setFolders(prev => prev.map(f =>
      f.id === folderId ? { ...f, recipeIds: (f.recipeIds ?? []).filter(id => id !== recipeId) } : f
    ))
  }

  async function deleteFolder(folderId) {
    const res = await fetch(`/api/user/folders/${folderId}`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${user.token}` },
    })
    if (!res.ok) return
    const { bookmarks } = await res.json()
    const updated = { ...user, bookmarks }
    localStorage.setItem('user', JSON.stringify(updated))
    setUser(updated)
    setFolders(prev => prev.filter(f => f.id !== folderId))
  }

  return (
    <AuthContext.Provider value={{
      user, login, logout,
      folders, fetchFolders,
      createFolder, addBookmark, removeBookmark, removeFromFolder, deleteFolder,
    }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  return useContext(AuthContext)
}
