import { useEffect, useState } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import RecipeCard from '../components/RecipeCard'
import './SearchResults.css'

const API = '/api'

export default function SearchResults() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const q = params.get('q') || ''
  const [results, setResults] = useState([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [query, setQuery] = useState(q)

  useEffect(() => {
    if (!q) return
    setLoading(true)
    fetch(`${API}/search?q=${encodeURIComponent(q)}`)
      .then((r) => r.json())
      .then((data) => {
        setResults(data.results || [])
        setTotal(data.total || 0)
        setLoading(false)
      })
      .catch(() => setLoading(false))
  }, [q])

  const handleSearch = (e) => {
    e.preventDefault()
    if (query.trim()) navigate(`/search?q=${encodeURIComponent(query.trim())}`)
  }

  return (
    <div className="search-page">
      <div className="search-header">
        <h1 onClick={() => navigate('/')} className="logo">🍽️ Recipes</h1>
        <form className="search-bar" onSubmit={handleSearch}>
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search recipes..."
          />
          <button type="submit">Search</button>
        </form>
      </div>

      <div className="results-info">
        {!loading && <p>{total.toLocaleString()} results for <strong>"{q}"</strong></p>}
      </div>

      {loading ? (
        <p className="loading">Searching...</p>
      ) : (
        <div className="results-grid">
          {results.map((recipe) => (
            <RecipeCard key={recipe.RecipeId} recipe={recipe} />
          ))}
        </div>
      )}
    </div>
  )
}
