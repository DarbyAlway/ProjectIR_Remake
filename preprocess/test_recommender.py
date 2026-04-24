"""
Test the LGBMRecommender end-to-end.

Mirrors exactly what the Go server must do at inference:
  1. Load the pkl artifact.
  2. Given a list of liked RecipeIds → compute user_profile (mean of their
     NUMERIC_BASE feature vectors).
  3. For every unseen candidate → compute affinity = candidate − profile.
  4. Predict score → rank → return top-N.

Run:
    cd preprocess
    python test_recommender.py
"""

import os, pickle
import numpy as np
import pandas as pd

BASE_DIR   = os.path.dirname(__file__)
MODEL_PATH = os.path.join(BASE_DIR, "models", "lgbm_recommender.pkl")

# ── Load artifact ──────────────────────────────────────────────────────────────

print("Loading model artifact ...")
with open(MODEL_PATH, "rb") as f:
    art = pickle.load(f)

model          = art["model"]
all_features   = art["features"]       # BASE_FEATURES + AFFINITY_FEATURES
base_features  = art["base_features"]  # BASE_FEATURES (includes RecipeCategoryEncoded)
numeric_base   = art["numeric_base"]   # features that get affinity-differenced
recipe_df      = art["recipe_features"].loc[:, ~art["recipe_features"].columns.duplicated()].copy()

print(f"  Recipes in index : {len(recipe_df):,}")
print(f"  Model features   : {len(all_features)}")
print(f"  Trees            : {model.num_trees()}")

# ── Core inference function ────────────────────────────────────────────────────

def recommend(liked_ids: list, top_n: int = 10, exclude_liked: bool = True) -> pd.DataFrame:
    """
    Return a DataFrame of top_n recommended recipes ranked by predicted score.

    Parameters
    ----------
    liked_ids     : list of RecipeId values the user has bookmarked
    top_n         : number of recommendations to return
    exclude_liked : whether to filter out already-liked recipes (default True)
    """
    liked_set = set(liked_ids)

    # ── Build user profile ──────────────────────────────────────────────────
    liked_rows = recipe_df[recipe_df["RecipeId"].isin(liked_set)]
    if liked_rows.empty:
        print("  [warn] No liked recipes found in the index — returning top-rated globally.")
        # Fall back: profile = global mean (all affinities → 0, model uses base features only)
        profile = recipe_df[numeric_base].mean().to_numpy()
    else:
        profile = liked_rows[numeric_base].mean().to_numpy()  # (nf,)

    # ── Build candidate matrix ──────────────────────────────────────────────
    candidates = recipe_df[~recipe_df["RecipeId"].isin(liked_set)].copy() if exclude_liked \
                 else recipe_df.copy()

    cand_base    = candidates[base_features].to_numpy(dtype=np.float64)
    cand_numeric = candidates[numeric_base].to_numpy(dtype=np.float64)
    affinity     = cand_numeric - profile                          # (n_cand, nf)

    X = np.concatenate([cand_base, affinity], axis=1)             # (n_cand, n_features)
    X_df = pd.DataFrame(X, columns=all_features)

    scores = model.predict(X_df)

    candidates = candidates.copy()
    candidates["predicted_score"] = scores
    top = candidates.nlargest(top_n, "predicted_score")[
        ["RecipeId"] + base_features + ["predicted_score"]
    ].reset_index(drop=True)
    top.index += 1  # 1-based rank
    return top


# ── Helpers ────────────────────────────────────────────────────────────────────

def show(label: str, result: pd.DataFrame):
    print(f"\n{'═'*60}")
    print(f"  {label}")
    print(f"{'═'*60}")
    cols = ["RecipeId", "RecipeCategoryEncoded", "Calories",
            "ProteinContent", "TotalTimeMinutes", "predicted_score"]
    print(result[[c for c in cols if c in result.columns]].to_string())


def sample_ids_from_category(category_encoded: int, n: int = 3, seed: int = 0) -> list:
    pool = recipe_df[recipe_df["RecipeCategoryEncoded"] == category_encoded]["RecipeId"]
    return pool.sample(n=min(n, len(pool)), random_state=seed).tolist()


# ── Tests ──────────────────────────────────────────────────────────────────────

print("\n" + "─"*60)
print("TEST 1 — user likes a specific category (encoded=0)")
print("  Expect: recommendations from the same or similar category")
cat0_ids = sample_ids_from_category(category_encoded=0, n=3)
print(f"  Liked IDs: {cat0_ids}")
result1 = recommend(cat0_ids, top_n=5)
show("Top-5 for user who likes category 0", result1)

# Check: majority of results should share the same category
dominant_cat = result1["RecipeCategoryEncoded"].mode()[0]
same_cat_pct = (result1["RecipeCategoryEncoded"] == 0).mean() * 100
print(f"\n  Same-category hits: {same_cat_pct:.0f}%  (dominant={dominant_cat})")

# ── Test 2 ─────────────────────────────────────────────────────────────────────

print("\n" + "─"*60)
print("TEST 2 — low-calorie profile (top-20 percentile by Calories)")
low_cal = recipe_df.nsmallest(int(len(recipe_df) * 0.05), "Calories")
liked_low_cal = low_cal["RecipeId"].sample(5, random_state=1).tolist()
result2 = recommend(liked_low_cal, top_n=5)
show("Top-5 for low-calorie user", result2)

avg_cal_liked = recipe_df[recipe_df["RecipeId"].isin(liked_low_cal)]["Calories"].mean()
avg_cal_recs  = result2["Calories"].mean()
print(f"\n  Avg Calories liked   : {avg_cal_liked:.1f}")
print(f"  Avg Calories in recs : {avg_cal_recs:.1f}  (lower = more personalized)")

# ── Test 3 ─────────────────────────────────────────────────────────────────────

print("\n" + "─"*60)
print("TEST 3 — empty liked list (cold-start fallback)")
result3 = recommend([], top_n=5)
show("Top-5 cold-start (no bookmarks)", result3)

# ── Test 4 ─────────────────────────────────────────────────────────────────────

print("\n" + "─"*60)
print("TEST 4 — unknown / garbage IDs (graceful degradation)")
result4 = recommend([-999, -1], top_n=5)
show("Top-5 with unknown liked IDs", result4)

# ── Test 5 ─────────────────────────────────────────────────────────────────────

print("\n" + "─"*60)
print("TEST 5 — exclude_liked=False (liked recipes can be re-recommended)")
some_ids = recipe_df["RecipeId"].sample(5, random_state=7).tolist()
result5_excl = recommend(some_ids, top_n=5, exclude_liked=True)
result5_incl = recommend(some_ids, top_n=5, exclude_liked=False)
liked_appear = result5_incl["RecipeId"].isin(some_ids).any()
print(f"  Liked IDs appear when exclude_liked=False : {liked_appear}")
print(f"  Liked IDs appear when exclude_liked=True  : "
      f"{result5_excl['RecipeId'].isin(some_ids).any()}")

# ── Test 6 — sanity: scores should vary ───────────────────────────────────────

print("\n" + "─"*60)
print("TEST 6 — score variance (model should not return the same score for all)")
all_scores = model.predict(
    pd.DataFrame(
        np.concatenate([
            recipe_df[base_features].to_numpy(dtype=np.float64),
            np.zeros((len(recipe_df), len(numeric_base))),  # zero affinity (neutral profile)
        ], axis=1),
        columns=all_features,
    )
)
print(f"  Score min  : {all_scores.min():.4f}")
print(f"  Score max  : {all_scores.max():.4f}")
print(f"  Score mean : {all_scores.mean():.4f}")
print(f"  Score std  : {all_scores.std():.4f}")
assert all_scores.std() > 0.01, "FAIL: model outputs near-constant scores — it learned nothing!"
print("  PASS: scores have meaningful variance")

print("\n" + "═"*60)
print("All tests done.")
