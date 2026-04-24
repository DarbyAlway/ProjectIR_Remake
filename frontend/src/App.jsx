import { useState } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import Carousel from './components/Carousel'
import { useAuth } from './context/AuthContext'
import './App.css'

export default function App() {
  const [query, setQuery] = useState('')
  const navigate = useNavigate()
  const { user, logout } = useAuth()

  const handleSearch = (e) => {
    e.preventDefault()
    if (query.trim()) navigate(`/search?q=${encodeURIComponent(query.trim())}`)
  }

  return (
    <div className="home">
      <div className="hero">
        <div className="hero-nav">
          {user ? (
            <div className="hero-user">
              <span>Hi, <strong>{user.username}</strong></span>
              <Link to="/bookmarks" className="nav-link">My Bookmarks</Link>
              <button className="logout-btn" onClick={logout}>Log out</button>
            </div>
          ) : (
            <div className="hero-user">
              <Link to="/login" className="nav-link">Log in</Link>
              <Link to="/register" className="nav-link nav-link--primary">Sign up</Link>
            </div>
          )}
        </div>

        <h1>🍽️ Discover Recipes</h1>
        <p>Search by ingredient, dish name, or category</p>
        <form className="search-bar" onSubmit={handleSearch}>
          <input
            type="text"
            placeholder="e.g. chicken pasta, vegan, chocolate cake..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            autoFocus
          />
          <button type="submit">Search</button>
        </form>
      </div>

      <section className="carousel-section">
        <h2>Featured Recipes</h2>
        <Carousel />
      </section>
    </div>
  )
}
