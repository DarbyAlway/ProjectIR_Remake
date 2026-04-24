# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

ProjectIR is a recipe search and discovery web app. It has three parts:
- **`server/`** — Go HTTP API (port 8080)
- **`frontend/`** — React + Vite SPA
- **`preprocess/`** — Python scripts for data ingestion into Elasticsearch

## Architecture

### Data flow
Raw recipe data (parquet) → `preprocess/fill_images.py` (fills missing images via TF-IDF + cosine similarity) → `preprocess/index_data.py` (bulk-loads into Elasticsearch `recipes` and `reviews` indices) → Go server queries ES at runtime.

### Infrastructure dependencies
- **Elasticsearch** at `http://10.255.255.254:9200` (hardcoded in both Go server and Python scripts)
- **MongoDB** URI via `MONGODB_URI` env var — stores users (`projectir.users` collection)
- JWT secret via `JWT_SECRET` env var (defaults to `change-this-secret-in-production`)

### Go server (`server/main.go`)
Single-file server using only the standard `net/http` package plus three external libs (Elasticsearch, MongoDB, JWT). All routes are registered in `main()`:
- `GET /search?q=` — multi-match ES query across Name, Keywords, Ingredients, Description, Category
- `GET /random` — 12 random recipes via function_score
- `GET /recipe/:id` — ES get by ID + fetches up to 20 reviews from the `reviews` index
- `POST /auth/register` and `POST /auth/login` — bcrypt + JWT, stored in MongoDB

### React frontend (`frontend/src/`)
- `main.jsx` — router root; wraps all routes in `AuthProvider`
- `context/AuthContext.jsx` — auth state persisted to `localStorage`; exposes `user`, `login`, `logout`
- `App.jsx` (home) → `SearchResults` → `RecipeDetail` (pages)
- `Carousel.jsx` — calls `/random` on mount to display featured recipes

### Vite dev proxy
`/api/*` → `http://localhost:8080` (strips `/api` prefix)  
`/recipe/*` → `http://localhost:8080` (no rewrite)

## Commands

### Frontend
```bash
cd frontend
npm install        # install deps
npm run dev        # start dev server (http://localhost:5173)
npm run build      # production build → dist/
npm run lint       # ESLint
npm run preview    # preview production build
```

### Go server
```bash
cd server
go mod tidy                          # sync dependencies
MONGODB_URI=<uri> go run main.go     # run server
go build -o server main.go           # compile binary
```

### Preprocessing (Python)
```bash
cd preprocess
python fill_images.py    # fill missing recipe images (reads/writes merged.parquet)
python index_data.py     # (re-)index recipes and reviews into Elasticsearch
```

## ES Index Details

The `recipes` index uses a custom `food_analyzer` with synonym expansion (e.g. `zucchini ↔ courgette`, `cilantro ↔ coriander`, `bbq ↔ barbecue`), English stop-word filtering, and stemming. The `reviews` index stores `RecipeId` (keyword), `Rating`, and `Review` text only for retrieval (not searched).
