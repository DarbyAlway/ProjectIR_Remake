"""
Recommender microservice — stdlib only, no Flask needed.

Endpoint: POST /recommend
  Request : {"liked_ids": ["123", "456", ...], "top_n": 12}
  Response: {"recipe_ids": [789, 101, ...]}

Endpoint: GET /health
  Response: {"status": "ok", "recipes": <count>}

How it works:
  At startup   — predict quality score for every recipe once (fast).
  Per request  — filter out liked recipes, compute cosine similarity of
                 candidates to the folder's bookmarks profile, then blend:
                   final = 0.6 * quality_score + 0.4 * cosine_similarity
                 Different folders → different profile vectors → different recs.

Run:
    conda activate project_IR
    python recommender_service.py
"""

import json, os, pickle
import numpy as np
from http.server import BaseHTTPRequestHandler, HTTPServer

BASE_DIR   = os.path.dirname(__file__)
MODEL_PATH = os.path.join(BASE_DIR, "models", "recommender_model.pkl")
PORT       = 5001

print("Loading model artifact ...")
with open(MODEL_PATH, "rb") as f:
    art = pickle.load(f)

model    = art["model"]
_ids     = art["recipe_ids"]            # (N,) int64
_X       = art["recipe_features_scaled"]  # (N, F) already scaled

print(f"  Scoring {len(_ids):,} recipes ...")
_scores_raw = model.predict(_X)

# Normalize scores to [0, 1] for blending
_s_min = _scores_raw.min()
_s_rng = _scores_raw.max() - _s_min
_scores = (_scores_raw - _s_min) / (_s_rng if _s_rng > 0 else 1.0)

# L2-normalize feature rows so dot product = cosine similarity
norms = np.linalg.norm(_X, axis=1, keepdims=True)
norms[norms == 0] = 1.0
_X_norm = _X / norms

print(f"  Ready. Score range: [{_scores_raw.min():.4f}, {_scores_raw.max():.4f}]")


def recommend(liked_ids: list, top_n: int = 12) -> list:
    liked_set  = {int(x) for x in liked_ids if str(x).lstrip("-").isdigit()}
    liked_mask = np.isin(_ids, list(liked_set))
    cand_mask  = ~liked_mask

    cand_ids   = _ids[cand_mask]
    cand_score = _scores[cand_mask]
    cand_norm  = _X_norm[cand_mask]

    if liked_mask.any():
        # Build user profile from liked recipes and compute cosine similarity
        profile = _X_norm[liked_mask].mean(axis=0)
        pnorm   = np.linalg.norm(profile)
        if pnorm > 0:
            profile /= pnorm
        sim      = cand_norm @ profile              # (n_cand,) cosine similarity
        sim_norm = (sim + 1.0) / 2.0               # shift [-1,1] → [0,1]
        final    = 0.6 * cand_score + 0.4 * sim_norm
    else:
        final = cand_score

    top_n   = min(top_n, len(final))
    top_idx = np.argpartition(final, -top_n)[-top_n:]
    top_idx = top_idx[np.argsort(final[top_idx])[::-1]]
    return [int(x) for x in cand_ids[top_idx]]


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        pass

    def do_GET(self):
        if self.path in ("/", "/health"):
            payload = json.dumps({"status": "ok", "recipes": len(_ids)}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)
        else:
            self.send_error(404)

    def do_POST(self):
        if self.path != "/recommend":
            self.send_error(404)
            return
        try:
            length  = int(self.headers.get("Content-Length", 0))
            body    = json.loads(self.rfile.read(length))
            result  = recommend(body.get("liked_ids", []), int(body.get("top_n", 12)))
            payload = json.dumps({"recipe_ids": result}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)
        except Exception as e:
            self.send_error(500, str(e))


print(f"Recommender service listening on http://localhost:{PORT}")
HTTPServer(("", PORT), Handler).serve_forever()
