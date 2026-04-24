import polars as pl
import numpy as np
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.decomposition import TruncatedSVD
from sklearn.preprocessing import normalize
from tqdm import tqdm

try:
    import torch
    USE_GPU = torch.cuda.is_available()
except ImportError:
    USE_GPU = False

DATA_PATH = "merged.parquet"
TEXT_COLS = ["Name", "Description", "Keywords", "RecipeIngredientParts", "RecipeCategory"]
SVD_DIMS = 200
BATCH_SIZE = 1024


def is_empty_image(val):
    if val is None:
        return True
    if isinstance(val, list):
        return len(val) == 0
    s = str(val).strip()
    return s in ("", "NA", "character(0)", "None", "c()", "[]")


def combine_text(row):
    return " ".join(str(row[c]) if row[c] is not None else "" for c in TEXT_COLS)


def find_neighbors_gpu(has_img_vecs, empty_vecs):
    """Cosine similarity search on GPU using torch. Vectors must already be L2-normalized."""
    device = "cuda"
    print(f"  Running on GPU: {torch.cuda.get_device_name(0)}")
    has_tensor = torch.from_numpy(has_img_vecs).to(device)  # (N, D)

    all_neighbors = []
    total_batches = (len(empty_vecs) + BATCH_SIZE - 1) // BATCH_SIZE
    for i in tqdm(range(0, len(empty_vecs), BATCH_SIZE), total=total_batches, desc="GPU cosine search"):
        batch = torch.from_numpy(empty_vecs[i : i + BATCH_SIZE]).to(device)
        sims = torch.mm(batch, has_tensor.T)           # (batch, N)
        best = sims.argmax(dim=1).cpu().numpy()
        all_neighbors.append(best)

    return np.concatenate(all_neighbors)


def find_neighbors_cpu(has_img_vecs, empty_vecs):
    """Cosine similarity search on CPU using sklearn."""
    from sklearn.neighbors import NearestNeighbors
    print("  Running on CPU (sklearn NearestNeighbors)...")
    nn = NearestNeighbors(n_neighbors=1, metric="cosine", algorithm="brute", n_jobs=-1)
    nn.fit(has_img_vecs)
    _, neighbor_pos = nn.kneighbors(empty_vecs)
    return neighbor_pos.flatten()


def main():
    print(f"Device: {'GPU (' + torch.cuda.get_device_name(0) + ')' if USE_GPU else 'CPU'}")

    print("\nLoading merged.parquet...")
    df = pl.read_parquet(DATA_PATH)
    print(f"Shape: {df.shape}")

    # Work at unique recipe level — avoids redundant computation on 1.4M rows
    recipe_cols = ["RecipeId"] + TEXT_COLS + ["Images"]
    recipes = df.select(recipe_cols).unique(subset=["RecipeId"])
    recipe_rows = recipes.to_dicts()
    print(f"Unique recipes: {len(recipe_rows):,}")

    empty_indices   = [i for i, r in tqdm(enumerate(recipe_rows), total=len(recipe_rows), desc="Scanning images") if     is_empty_image(r["Images"])]
    has_img_indices = [i for i, r in enumerate(recipe_rows) if not is_empty_image(r["Images"])]
    print(f"Recipes with empty images: {len(empty_indices):,}")
    print(f"Recipes with images:       {len(has_img_indices):,}")

    if not empty_indices:
        print("No empty images found. Nothing to do.")
        return

    # Build combined text node per recipe
    print("\nBuilding text features...")
    texts = [combine_text(r) for r in tqdm(recipe_rows, desc="Combining text")]

    # TF-IDF → TruncatedSVD to get dense float32 vectors (fits in GPU VRAM)
    print("Fitting TF-IDF (10k features)...")
    vectorizer = TfidfVectorizer(max_features=10000, stop_words="english", sublinear_tf=True)
    tfidf_matrix = vectorizer.fit_transform(texts)

    print(f"Reducing to {SVD_DIMS} dims via TruncatedSVD...")
    svd = TruncatedSVD(n_components=SVD_DIMS, random_state=42)
    dense_matrix = svd.fit_transform(tfidf_matrix).astype(np.float32)

    # L2 normalize so inner product == cosine similarity
    dense_matrix = normalize(dense_matrix, norm="l2")

    has_img_vecs = dense_matrix[has_img_indices]   # candidates
    empty_vecs   = dense_matrix[empty_indices]     # queries

    # Nearest neighbor search
    print("\nSearching for nearest neighbors...")
    if USE_GPU:
        neighbor_pos = find_neighbors_gpu(has_img_vecs, empty_vecs)
    else:
        neighbor_pos = find_neighbors_cpu(has_img_vecs, empty_vecs)

    # Build RecipeId -> replacement Images map
    replacements = {}
    for i, emp_idx in tqdm(enumerate(empty_indices), total=len(empty_indices), desc="Building replacements"):
        best_idx  = has_img_indices[neighbor_pos[i]]
        recipe_id = recipe_rows[emp_idx]["RecipeId"]
        replacements[recipe_id] = recipe_rows[best_idx]["Images"]

    print(f"\nReplacements built for {len(replacements):,} recipes")

    # Apply to full dataframe
    print("Applying replacements to full dataframe...")
    recipe_ids = df["RecipeId"].to_list()
    images     = df["Images"].to_list()

    new_images = [
        replacements.get(rid, img) if is_empty_image(img) else img
        for rid, img in tqdm(zip(recipe_ids, images), total=len(recipe_ids), desc="Applying replacements")
    ]

    df = df.with_columns(pl.Series("Images", new_images))

    still_empty = sum(1 for img in new_images if is_empty_image(img))
    print(f"Remaining empty images after fill: {still_empty:,}")

    df.write_parquet(DATA_PATH)
    print(f"\nSaved to {DATA_PATH}")


if __name__ == "__main__":
    main()
