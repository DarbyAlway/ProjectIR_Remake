import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import BookmarkModal from './BookmarkModal'
import './RecipeCard.css'

const PLACEHOLDER = 'https://placehold.co/400x300?text=No+Image'

function thumbUrl(url) {
  if (!url) return PLACEHOLDER
  return url.replace(/w_\d+,h_\d+[^/]*/, 'w_240,h_180,c_fill,q_60')
}

export default function RecipeCard({ recipe, onPause, onResume }) {
  const image = thumbUrl(recipe.Images?.[0])
  const navigate = useNavigate()
  const { user, removeBookmark, fetchFolders } = useAuth()
  const [showModal, setShowModal] = useState(false)
  const [warning, setWarning] = useState(false)

  const recipeId = String(recipe.RecipeId)
  const isBookmarked = user?.bookmarks?.includes(recipeId) ?? false

  async function handleBookmark(e) {
    e.stopPropagation()
    if (!user) {
      setWarning(true)
      setTimeout(() => setWarning(false), 3000)
      return
    }
    if (isBookmarked) {
      removeBookmark(recipeId)
    } else {
      await fetchFolders()
      onPause?.()
      setShowModal(true)
    }
  }

  return (
    <>
      <div className="recipe-card" onClick={() => navigate(`/recipe/${recipe.RecipeId}`)}>
        <div className="recipe-card-image-wrap">
          <img src={image} alt={recipe.Name} loading="lazy" onError={(e) => (e.target.src = PLACEHOLDER)} />
          <button
            className={`bookmark-btn${isBookmarked ? ' bookmarked' : ''}`}
            onClick={handleBookmark}
            title={isBookmarked ? 'Remove bookmark' : 'Bookmark this recipe'}
          >
            <span className="bookmark-icon">{isBookmarked ? '★' : '☆'}</span>
          </button>
        </div>
        {warning && (
          <div className="bookmark-warning">Please log in to bookmark recipes</div>
        )}
        <div className="recipe-card-body">
          <span className="category">{recipe.RecipeCategory}</span>
          <h3>{recipe.Name}</h3>
          <p className="description">{recipe.Description?.slice(0, 80)}...</p>
          <div className="meta">
            <span>⭐ {recipe.AggregatedRating?.toFixed(1) ?? 'N/A'}</span>
            {recipe.TotalTimeMinutes > 0 && <span>⏱ {recipe.TotalTimeMinutes} min</span>}
            <span>🔥 {recipe.Calories?.toFixed(0)} cal</span>
          </div>
        </div>
      </div>

      {showModal && (
        <BookmarkModal recipeId={recipeId} onClose={() => { setShowModal(false); onResume?.() }} />
      )}
    </>
  )
}
