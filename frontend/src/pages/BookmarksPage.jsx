import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import RecipeCard from '../components/RecipeCard'
import './BookmarksPage.css'

const PLACEHOLDER = 'https://placehold.co/400x300?text=No+Image'

function thumbUrl(url) {
  if (!url) return PLACEHOLDER
  return url.replace(/w_\d+,h_\d+[^/]*/, 'w_240,h_180,c_fill,q_60')
}

export default function BookmarksPage() {
  const navigate = useNavigate()
  const { user, folders, fetchFolders, deleteFolder, removeFromFolder } = useAuth()
  const [recipes, setRecipes] = useState({}) // folderId → Recipe[]
  const [activeFolder, setActiveFolder] = useState(null)
  const [loading, setLoading] = useState(true)
  const [recs, setRecs] = useState([])
  const [recsLoading, setRecsLoading] = useState(false)

  useEffect(() => {
    if (!user) { navigate('/login'); return }
    fetchFolders(user.token).then(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (!user?.token || !activeFolder) return
    const folder = folders.find(f => f.id === activeFolder)
    if (!folder || (folder.recipeIds ?? []).length === 0) {
      setRecs([])
      return
    }
    setRecsLoading(true)
    fetch(`/api/recommendations?folder_id=${activeFolder}`, {
      headers: { Authorization: `Bearer ${user.token}` },
    })
      .then(r => r.json())
      .then(data => setRecs(data.results ?? []))
      .catch(() => setRecs([]))
      .finally(() => setRecsLoading(false))
  }, [activeFolder])

  useEffect(() => {
    if (folders.length > 0 && activeFolder === null) {
      setActiveFolder(folders[0].id)
    }
  }, [folders])

  useEffect(() => {
    if (!activeFolder) return
    const folder = folders.find(f => f.id === activeFolder)
    if (!folder || (folder.recipeIds ?? []).length === 0) {
      setRecipes(prev => ({ ...prev, [activeFolder]: [] }))
      return
    }
    if (recipes[activeFolder]) return // already loaded

    const ids = folder.recipeIds.join(',')
    fetch(`/api/recipes/batch?ids=${ids}`)
      .then(r => r.json())
      .then(data => setRecipes(prev => ({ ...prev, [activeFolder]: data })))
  }, [activeFolder, folders])

  async function handleDeleteFolder(folderId) {
    if (!confirm('Delete this folder and all its bookmarks?')) return
    await deleteFolder(folderId)
    setRecipes(prev => { const n = { ...prev }; delete n[folderId]; return n })
    setActiveFolder(folders.find(f => f.id !== folderId)?.id ?? null)
  }

  async function handleRemove(recipeId, folderId) {
    await removeFromFolder(recipeId, folderId)
    setRecipes(prev => ({
      ...prev,
      [folderId]: (prev[folderId] ?? []).filter(r => String(r.RecipeId) !== recipeId),
    }))
  }

  if (!user) return null
  if (loading) return <p className="bm-loading">Loading bookmarks...</p>

  const activeFolderData = folders.find(f => f.id === activeFolder)
  const activeRecipes = recipes[activeFolder] ?? []

  return (
    <div className="bm-page">
      <div className="bm-header">
        <button className="bm-back" onClick={() => navigate(-1)}>← Back</button>
        <h1>My Bookmarks</h1>
      </div>

      {folders.length === 0 ? (
        <div className="bm-empty">
          <p>No folders yet. Bookmark a recipe to get started!</p>
          <button className="bm-browse" onClick={() => navigate('/')}>Browse Recipes</button>
        </div>
      ) : (
        <div className="bm-layout">
          <aside className="bm-sidebar">
            {folders.map(f => (
              <button
                key={f.id}
                className={`bm-folder-btn${f.id === activeFolder ? ' active' : ''}`}
                onClick={() => setActiveFolder(f.id)}
              >
                <span>📁 {f.name}</span>
                <span className="bm-folder-count">{f.recipeIds?.length ?? 0}</span>
              </button>
            ))}
          </aside>

          <main className="bm-main">
            {activeFolderData && (
              <div className="bm-folder-header">
                <h2>📁 {activeFolderData.name}</h2>
                <button
                  className="bm-delete-folder"
                  onClick={() => handleDeleteFolder(activeFolderData.id)}
                >
                  Delete folder
                </button>
              </div>
            )}

            {activeRecipes.length === 0 ? (
              <p className="bm-empty-folder">This folder is empty.</p>
            ) : (
              <div className="bm-grid">
                {activeRecipes.map(recipe => (
                  <div
                    key={recipe.RecipeId}
                    className="bm-card"
                    onClick={() => navigate(`/recipe/${recipe.RecipeId}`)}
                  >
                    <img
                      src={thumbUrl(recipe.Images?.[0])}
                      alt={recipe.Name}
                      onError={e => (e.target.src = PLACEHOLDER)}
                    />
                    <div className="bm-card-body">
                      <span className="bm-category">{recipe.RecipeCategory}</span>
                      <h3>{recipe.Name}</h3>
                      <div className="bm-meta">
                        <span>⭐ {recipe.AggregatedRating?.toFixed(1) ?? 'N/A'}</span>
                        {recipe.TotalTimeMinutes > 0 && <span>⏱ {recipe.TotalTimeMinutes} min</span>}
                      </div>
                    </div>
                    <button
                      className="bm-remove"
                      onClick={e => { e.stopPropagation(); handleRemove(String(recipe.RecipeId), activeFolder) }}
                      title="Remove from folder"
                    >
                      ✕
                    </button>
                  </div>
                ))}
              </div>
            )}
          </main>
        </div>
      )}
      {(recsLoading || recs.length > 0) && (
        <section className="bm-recs">
          <h2>You might also like</h2>
          {recsLoading ? (
            <p className="bm-recs-loading">Finding recommendations...</p>
          ) : (
            <div className="bm-recs-row">
              {recs.map(recipe => (
                <div key={recipe.RecipeId} className="bm-recs-item">
                  <RecipeCard recipe={recipe} />
                </div>
              ))}
            </div>
          )}
        </section>
      )}
    </div>
  )
}
