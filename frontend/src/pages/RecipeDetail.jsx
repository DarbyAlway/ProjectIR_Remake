import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import BookmarkModal from '../components/BookmarkModal'
import './RecipeDetail.css'

const PLACEHOLDER = 'https://placehold.co/800x500?text=No+Image'

export default function RecipeDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { user, removeBookmark, fetchFolders } = useAuth()
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [warning, setWarning] = useState(false)
  const [showModal, setShowModal] = useState(false)

  useEffect(() => {
    fetch(`/api/recipe/${id}`)
      .then((r) => r.json())
      .then((d) => { setData(d); setLoading(false) })
      .catch(() => setLoading(false))
  }, [id])

  if (loading) return <p className="loading">Loading recipe...</p>
  if (!data)   return <p className="loading">Recipe not found.</p>

  const { recipe, reviews } = data
  const image = recipe.Images?.[0] ?? PLACEHOLDER
  const recipeId = String(id)
  const isBookmarked = user?.bookmarks?.includes(recipeId) ?? false

  async function handleBookmark() {
    if (!user) {
      setWarning(true)
      setTimeout(() => setWarning(false), 3000)
      return
    }
    if (isBookmarked) {
      removeBookmark(recipeId)
    } else {
      await fetchFolders()
      setShowModal(true)
    }
  }

  return (
    <>
    <div className="detail-page">
      <button className="back-btn" onClick={() => navigate(-1)}>← Back</button>

      <div className="detail-hero">
        <img src={image} alt={recipe.Name} onError={(e) => (e.target.src = PLACEHOLDER)} />
        <div className="detail-hero-info">
          <span className="category">{recipe.RecipeCategory}</span>
          <div className="detail-title-row">
            <h1>{recipe.Name}</h1>
            <button
              className={`detail-bookmark-btn${isBookmarked ? ' bookmarked' : ''}`}
              onClick={handleBookmark}
              title={isBookmarked ? 'Remove bookmark' : 'Bookmark this recipe'}
            >
              {isBookmarked ? '★ Bookmarked' : '☆ Bookmark'}
            </button>
          </div>
          {warning && <p className="detail-bookmark-warning">Please log in to bookmark recipes.</p>}
          <p className="description">{recipe.Description}</p>

          <div className="stats">
            <div className="stat"><span>⭐</span><strong>{recipe.AggregatedRating?.toFixed(1)}</strong><small>Rating</small></div>
            <div className="stat"><span>💬</span><strong>{recipe.ReviewCount}</strong><small>Reviews</small></div>
            {recipe.TotalTimeMinutes > 0 && <div className="stat"><span>⏱</span><strong>{recipe.TotalTimeMinutes}</strong><small>Minutes</small></div>}
            <div className="stat"><span>🍽️</span><strong>{recipe.RecipeServings}</strong><small>Servings</small></div>
          </div>
        </div>
      </div>

      <div className="detail-body">
        <div className="detail-left">
          <section>
            <h2>Ingredients</h2>
            <ul className="ingredients">
              {recipe.RecipeIngredientParts?.map((ing, i) => (
                <li key={i}>{ing}</li>
              ))}
            </ul>
          </section>

          <section>
            <h2>Nutrition</h2>
            <div className="nutrition-grid">
              <div className="nutrition-item"><strong>{recipe.Calories?.toFixed(0)}</strong><small>Calories</small></div>
              <div className="nutrition-item"><strong>{recipe.ProteinContent?.toFixed(1)}g</strong><small>Protein</small></div>
              <div className="nutrition-item"><strong>{recipe.FatContent?.toFixed(1)}g</strong><small>Fat</small></div>
              <div className="nutrition-item"><strong>{recipe.CarbohydrateContent?.toFixed(1)}g</strong><small>Carbs</small></div>
            </div>
          </section>
        </div>

        <div className="detail-right">
          <section>
            <h2>Instructions</h2>
            <ol className="instructions">
              {recipe.RecipeInstructions?.map((step, i) => (
                <li key={i}>{step}</li>
              ))}
            </ol>
          </section>
        </div>
      </div>

      <section className="reviews-section">
        <h2>Reviews ({reviews?.length ?? 0})</h2>
        {reviews?.length === 0 && <p className="no-reviews">No reviews yet.</p>}
        <div className="reviews-list">
          {reviews?.map((rev, i) => (
            <div className="review-card" key={i}>
              <div className="review-header">
                <span className="stars">{'★'.repeat(rev.Rating)}{'☆'.repeat(5 - rev.Rating)}</span>
              </div>
              <p>{rev.Review}</p>
            </div>
          ))}
        </div>
      </section>
    </div>

    {showModal && (
      <BookmarkModal recipeId={recipeId} onClose={() => setShowModal(false)} />
    )}
  </>
  )
}
