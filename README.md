# ProjectIR — Recipe Search & Recommendation App

A full-stack recipe discovery web app with semantic search and personalized recommendations powered by a LightGBM model.

**Live demo:** [https://projectir-frontend.onrender.com](https://projectir-frontend.onrender.com)

---

## Features

- **Full-text search** across 50k+ recipes with synonym expansion (e.g. `zucchini ↔ courgette`, `cilantro ↔ coriander`, `bbq ↔ barbecue`)
- **Recipe detail** page with nutrition info, ingredients, instructions, and reviews
- **User authentication** (register / login) with JWT
- **Bookmark folders** — organize saved recipes into named folders
- **Personalized recommendations** per folder using a trained LightGBM model + cosine similarity

---

## Tech Stack

| Layer | Technology |
|---|---|
| Frontend | React 18, Vite, React Router |
| Backend API | Go (net/http, no framework) |
| Search | Elasticsearch / OpenSearch |
| Database | MongoDB Atlas |
| ML Model | LightGBM (scikit-learn pipeline) |
| Recommender | Python HTTP microservice |
| Hosting | Render (frontend + API), Bonsai OpenSearch |

---

## Architecture

```
Browser (React SPA)
      │
      ▼
Render Static Site          ← frontend/dist/
      │  /api/*
      ▼
Render Web Service (Go)     ← server/main.go  :8080
      │              │
      ▼              ▼
Bonsai OpenSearch   MongoDB Atlas
(recipes/reviews)   (users/folders)
      │
      ▼
Python Recommender          ← preprocess/recommender_service.py  :5001
(LightGBM model)
```

---

## Recommendation System

The recommender uses a two-stage approach:

**1. Global quality score (LightGBM)**
A `LGBMRegressor` trained on 522k recipes predicts a quality score for every recipe based on:
- `AggregatedRating`, `ReviewCount`
- 9 nutrition features (Calories, Protein, Fat, etc.)
- `RecipeServings`, `RecipeCategoryEncoded`

Scores are precomputed once at service startup for all recipes.

**2. Per-folder personalization (cosine similarity)**
When you request recommendations for a folder, the service builds a profile from the bookmarked recipes' feature vectors and computes cosine similarity to all candidates.

**Final score:**
```
final = 0.6 × quality_score + 0.4 × cosine_similarity
```

Different folders get different recommendations because their bookmarked recipes define different profile vectors.

---

## Project Structure

```
ProjectIR/
├── frontend/               React + Vite SPA
│   └── src/
│       ├── pages/          App, SearchResults, RecipeDetail, BookmarksPage
│       ├── components/     Carousel, RecipeCard, BookmarkModal
│       └── context/        AuthContext (JWT + folders state)
├── server/
│   └── main.go             Go HTTP API (all routes in one file)
├── preprocess/
│   ├── index_data.py       Bulk-loads recipes into Elasticsearch
│   ├── train_model.py      Trains the LightGBM recommendation model
│   ├── recommender_service.py  Python HTTP microservice (port 5001)
│   └── models/
│       └── recommender_model.pkl
├── deploy/
│   ├── setup.sh            One-time server setup script
│   ├── start.sh            Start/restart all services
│   └── *.service           Systemd service files
├── docker-compose.yml      Elasticsearch for local development
├── nginx.conf              Production reverse proxy config
└── dev.sh                  Start all services locally
```

---

## API Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/search?q=` | — | Full-text recipe search |
| GET | `/random` | — | 12 random featured recipes |
| GET | `/recipe/:id` | — | Recipe detail + reviews |
| GET | `/recipes/batch?ids=` | — | Fetch multiple recipes by ID |
| POST | `/auth/register` | — | Create account |
| POST | `/auth/login` | — | Login, returns JWT |
| POST/DELETE | `/auth/bookmark` | ✓ | Add/remove bookmark |
| GET/POST | `/user/folders` | ✓ | List folders / create folder |
| GET/DELETE | `/user/folders/:id` | ✓ | Get folder / delete folder |
| POST | `/user/folders/:id/remove` | ✓ | Remove recipe from folder |
| GET | `/recommendations?folder_id=` | ✓ | Per-folder recommendations |

---

## Local Development

**Prerequisites:** Go 1.22+, Node 20+, Python 3.10+ (conda), Docker

**1. Clone and set up environment**
```bash
git clone https://github.com/DarbyAlway/ProjectIR_Remake.git
cd ProjectIR_Remake
cp .env.example .env
# fill in your values in .env
```

**2. Start Elasticsearch**
```bash
docker compose up -d
```

**3. Index data (first time only)**
```bash
cd preprocess
python index_data.py
```

**4. Start all services**
```bash
./dev.sh
```

App runs at `http://localhost:5173`

---

## Training the Recommendation Model

```bash
cd preprocess
python train_model.py
```

Trains on `data/merged.parquet` (~522k recipes), saves `models/recommender_model.pkl`.

---

## Environment Variables

Copy `.env.example` to `.env` and fill in:

| Variable | Description |
|---|---|
| `MONGODB_URI` | MongoDB Atlas connection string |
| `ES_URL` | Elasticsearch / OpenSearch URL (supports `https://user:pass@host`) |
| `JWT_SECRET` | Random secret for signing JWT tokens (`openssl rand -hex 64`) |
| `ALLOWED_ORIGIN` | Frontend URL for CORS (e.g. `https://yourapp.onrender.com`) |

---

## Deployment

Deployed on **Render** (frontend static site + Go API) and **Bonsai OpenSearch** (search index).

See `deploy/` folder for server setup scripts and systemd service files for VPS deployment.
