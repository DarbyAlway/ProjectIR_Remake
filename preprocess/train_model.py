"""
Train a recipe quality model using LGBMRegressor.

Approach (matches the original working design):
  - Features: AggregatedRating, ReviewCount, 9 nutrition cols, RecipeServings,
    RecipeCategoryEncoded (13 total).
  - Target: AggregatedRating.
    Including the rating as an input feature is intentional — the model learns a
    Bayesian-average-style quality score that blends actual rating with review
    confidence and nutritional profile, rather than sorting by raw rating alone.
  - SimpleImputer (mean) → StandardScaler → LGBMRegressor(n_estimators=1000)

Inference (in recommender_service.py):
  1. Predict quality score for every recipe (precomputed at startup).
  2. Compute cosine similarity of each candidate to the folder's bookmarks profile.
  3. Blend: 60 % quality + 40 % similarity → return top-N unseen recipes.

Output:
  models/recommender_model.pkl  — all artifacts in one file
"""

import os, pickle, warnings
import numpy as np
import pandas as pd
import lightgbm as lgb
from sklearn.model_selection import train_test_split
from sklearn.preprocessing import LabelEncoder, StandardScaler
from sklearn.impute import SimpleImputer
from sklearn.metrics import mean_absolute_error

warnings.filterwarnings("ignore")

BASE_DIR   = os.path.dirname(__file__)
DATA_PATH  = os.path.join(BASE_DIR, "data", "merged.parquet")
MODEL_DIR  = os.path.join(BASE_DIR, "models")
MODEL_PATH = os.path.join(MODEL_DIR, "recommender_model.pkl")

FEATURES = [
    "AggregatedRating", "ReviewCount",
    "Calories", "FatContent", "SaturatedFatContent", "CholesterolContent",
    "SodiumContent", "CarbohydrateContent", "FiberContent", "SugarContent",
    "ProteinContent", "RecipeServings", "RecipeCategoryEncoded",
]
TARGET = "AggregatedRating"


def main():
    print(f"Loading {DATA_PATH} ...")
    df = pd.read_parquet(DATA_PATH)
    df = df.drop_duplicates("RecipeId").reset_index(drop=True)
    print(f"  {len(df):,} unique recipes")

    encoder = LabelEncoder()
    df["RecipeCategoryEncoded"] = encoder.fit_transform(df["RecipeCategory"].fillna("Unknown"))

    imputer = SimpleImputer(strategy="mean")
    df_feat = pd.DataFrame(imputer.fit_transform(df[FEATURES]), columns=FEATURES)

    scaler = StandardScaler()
    X = scaler.fit_transform(df_feat[FEATURES])
    y = df_feat[TARGET].values

    X_train, X_test, y_train, y_test = train_test_split(X, y, test_size=0.2, random_state=0)
    print(f"  Train: {len(X_train):,}  |  Test: {len(X_test):,}")

    print("Training LGBMRegressor ...")
    model = lgb.LGBMRegressor(n_estimators=1000, random_state=0, verbose=-1)
    model.fit(X_train, y_train)

    mae = mean_absolute_error(y_test, model.predict(X_test))
    print(f"  Test MAE: {mae:.4f}")

    os.makedirs(MODEL_DIR, exist_ok=True)
    artifact = {
        "model":                  model,
        "encoder":                encoder,
        "imputer":                imputer,
        "scaler":                 scaler,
        "features":               FEATURES,
        "recipe_ids":             df["RecipeId"].to_numpy(dtype=np.int64),
        "recipe_features_scaled": X,
    }
    with open(MODEL_PATH, "wb") as f:
        pickle.dump(artifact, f)
    print(f"  Saved → {MODEL_PATH}")


if __name__ == "__main__":
    main()
