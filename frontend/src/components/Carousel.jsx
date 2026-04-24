import { useEffect, useState, useRef, useCallback } from 'react'
import RecipeCard from './RecipeCard'
import './Carousel.css'

const API = '/api'

export default function Carousel() {
  const [recipes, setRecipes] = useState([])
  const [active, setActive] = useState(0)
  const timerRef = useRef(null)
  const pausedRef = useRef(false)

  useEffect(() => {
    fetch(`${API}/random`)
      .then((r) => r.json())
      .then((data) => setRecipes(data.results || []))
      .catch(console.error)
  }, [])

  const startTimer = useCallback((recipeCount) => {
    clearInterval(timerRef.current)
    if (pausedRef.current) return
    timerRef.current = setInterval(() => {
      setActive((prev) => (prev + 1) % recipeCount)
    }, 3000)
  }, [])

  useEffect(() => {
    if (recipes.length === 0) return
    startTimer(recipes.length)
    return () => clearInterval(timerRef.current)
  }, [recipes, startTimer])

  const pause = useCallback(() => {
    pausedRef.current = true
    clearInterval(timerRef.current)
  }, [])

  const resume = useCallback(() => {
    pausedRef.current = false
    if (recipes.length > 0) startTimer(recipes.length)
  }, [recipes, startTimer])

  const go = (dir) => {
    clearInterval(timerRef.current)
    setActive((prev) => (prev + dir + recipes.length) % recipes.length)
  }

  if (recipes.length === 0) return <p className="loading">Loading recipes...</p>

  const visible = [-2, -1, 0, 1, 2].map((offset) => {
    const index = (active + offset + recipes.length) % recipes.length
    return { recipe: recipes[index], offset }
  })

  return (
    <div className="carousel">
      <button className="carousel-btn left" onClick={() => go(-1)}>&#8249;</button>

      <div className="carousel-track">
        {visible.map(({ recipe, offset }) => (
          <div
            key={`${recipe.RecipeId}-${offset}`}
            className={`carousel-item offset-${offset}`}
          >
            <RecipeCard recipe={recipe} onPause={pause} onResume={resume} />
          </div>
        ))}
      </div>

      <button className="carousel-btn right" onClick={() => go(1)}>&#8250;</button>

      <div className="carousel-dots">
        {recipes.map((_, i) => (
          <span
            key={i}
            className={`dot ${i === active ? 'active' : ''}`}
            onClick={() => setActive(i)}
          />
        ))}
      </div>
    </div>
  )
}
