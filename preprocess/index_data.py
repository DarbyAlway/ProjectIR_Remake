import os, re
import polars as pl
import numpy as np
from elasticsearch import Elasticsearch
from elasticsearch.helpers import bulk
from tqdm import tqdm

ES_HOST    = os.getenv("ES_URL", "http://localhost:9200")
DATA_PATH  = "data/merged.parquet"
BATCH      = 500
TOP_N      = 50_000   # index only top-N recipes to fit free-tier storage

RECIPES_MAPPING = {
    "settings": {
        "analysis": {
            "filter": {
                "food_synonyms": {
                    "type": "synonym",
                    "synonyms": [
                        # compound words with/without spaces
                        "pannacotta, panna cotta",
                        "stirfry, stir fry, stir-fry",
                        "stirfried, stir fried, stir-fried",
                        "deepfry, deep fry, deep-fry",
                        "deepfried, deep fried, deep-fried",
                        "airfry, air fry, air-fry",
                        "airfried, air fried, air-fried",
                        "slowcooker, slow cooker, slow-cooker, crockpot, crock pot",
                        "icecream, ice cream, ice-cream",
                        "cheesecake, cheese cake",
                        "shortcake, short cake",
                        "hotdog, hot dog, hot-dog",
                        "hamburger, ham burger",
                        "meatball, meat ball",
                        "meatloaf, meat loaf",
                        "cornbread, corn bread",
                        "sourdough, sour dough",
                        "buttermilk, butter milk",
                        "sweetpotato, sweet potato",
                        # abbreviations and common names
                        "bbq, barbeque, barbecue",
                        "mac and cheese, macaroni and cheese, macaroni cheese",
                        "pb, peanut butter",
                        "pb&j, peanut butter and jelly, peanut butter jelly",
                        # spelling variations
                        "doughnut, donut",
                        "ketchup, catsup",
                        "yogurt, yoghurt",
                        "courgette, zucchini",
                        "aubergine, eggplant",
                        "coriander, cilantro",
                        "chickpea, chick pea, garbanzo",
                        "prawn, shrimp",
                        "spring onion, green onion, scallion",
                        "beetroot, beet",
                        "rocket, arugula",
                        "digestive biscuit, graham cracker",
                        # cuisine shortcuts
                        "italian, italiana",
                        "mex, mexican",
                    ]
                },
                "english_stop": {
                    "type": "stop",
                    "stopwords": "_english_"
                },
                "english_stemmer": {
                    "type": "stemmer",
                    "language": "english"
                }
            },
            "analyzer": {
                "food_analyzer": {
                    "type": "custom",
                    "tokenizer": "standard",
                    "filter": ["lowercase", "food_synonyms", "english_stop", "english_stemmer"]
                }
            }
        }
    },
    "mappings": {
        "properties": {
            # --- full-text search ---
            "Name":                  {"type": "text", "analyzer": "food_analyzer"},
            "Description":           {"type": "text", "analyzer": "food_analyzer"},
            "Keywords":              {"type": "text", "analyzer": "food_analyzer"},
            "RecipeIngredientParts": {"type": "text", "analyzer": "food_analyzer"},
            "RecipeCategory": {
                "type": "text", "analyzer": "food_analyzer",
                "fields": {"keyword": {"type": "keyword"}}
            },
            # --- filter / sort ---
            "AggregatedRating":    {"type": "float"},
            "ReviewCount":         {"type": "integer"},
            "Calories":            {"type": "float"},
            "FatContent":          {"type": "float"},
            "SaturatedFatContent": {"type": "float"},
            "CholesterolContent":  {"type": "float"},
            "SodiumContent":       {"type": "float"},
            "CarbohydrateContent": {"type": "float"},
            "FiberContent":        {"type": "float"},
            "SugarContent":        {"type": "float"},
            "ProteinContent":      {"type": "float"},
            "RecipeServings":      {"type": "integer"},
            "CookTimeMinutes":     {"type": "integer"},
            "PrepTimeMinutes":     {"type": "integer"},
            "TotalTimeMinutes":    {"type": "integer"},
            # --- store only ---
            "Images":             {"type": "keyword", "index": False},
            "RecipeInstructions": {"type": "keyword", "index": False},
            "RecipeYield":        {"type": "keyword", "index": False},
        }
    }
}

REVIEWS_MAPPING = {
    "mappings": {
        "properties": {
            "RecipeId":      {"type": "keyword"},
            "Rating":        {"type": "integer"},
            "Review":        {"type": "keyword", "index": False},
           
        }
    }
}


def parse_minutes(val):
    if not val:
        return None
    hours   = re.search(r"(\d+)H", str(val))
    minutes = re.search(r"(\d+)M", str(val))
    total   = (int(hours.group(1)) * 60 if hours else 0) + (int(minutes.group(1)) if minutes else 0)
    return total if total > 0 else None


def to_str_list(val):
    if isinstance(val, list):
        return [str(v) for v in val if v is not None]
    return val


def reset_index(es, name, mapping):
    if es.indices.exists(index=name):
        es.indices.delete(index=name)
        print(f"  Dropped existing index '{name}'")
    es.indices.create(index=name, body=mapping)
    print(f"  Created index '{name}'")


def run_bulk(es, docs, desc):
    success, failed = 0, 0
    batches = [docs[i : i + BATCH] for i in range(0, len(docs), BATCH)]
    for batch in tqdm(batches, desc=desc):
        ok, errors = bulk(es, batch, raise_on_error=False)
        success += ok
        failed  += len(errors)
    print(f"  Done — indexed: {success:,}  failed: {failed:,}")


def index_recipes(es, df):
    print("\n=== Indexing Recipes ===")
    reset_index(es, "recipes", RECIPES_MAPPING)

    keep = [
        "RecipeId", "Name", "Description", "Keywords", "RecipeIngredientParts",
        "RecipeCategory", "AggregatedRating", "ReviewCount",
        "Calories", "FatContent", "SaturatedFatContent", "CholesterolContent",
        "SodiumContent", "CarbohydrateContent", "FiberContent", "SugarContent",
        "ProteinContent", "RecipeServings", "RecipeYield",
        "CookTime", "PrepTime", "TotalTime", "Images", "RecipeInstructions",
    ]
    rows = df.select(keep).unique(subset=["RecipeId"]).to_dicts()
    print(f"  Unique recipes: {len(rows):,}")

    docs = [{
        "_index": "recipes",
        "_id":    row["RecipeId"],
        "_source": {
            "Name":                  row["Name"],
            "Description":           row["Description"],
            "Keywords":              to_str_list(row["Keywords"]),
            "RecipeIngredientParts": to_str_list(row["RecipeIngredientParts"]),
            "RecipeCategory":        row["RecipeCategory"],
            "AggregatedRating":      row["AggregatedRating"],
            "ReviewCount":           row["ReviewCount"],
            "Calories":              row["Calories"],
            "FatContent":            row["FatContent"],
            "SaturatedFatContent":   row["SaturatedFatContent"],
            "CholesterolContent":    row["CholesterolContent"],
            "SodiumContent":         row["SodiumContent"],
            "CarbohydrateContent":   row["CarbohydrateContent"],
            "FiberContent":          row["FiberContent"],
            "SugarContent":          row["SugarContent"],
            "ProteinContent":        row["ProteinContent"],
            "RecipeServings":        row["RecipeServings"],
            "RecipeYield":           row["RecipeYield"],
            "CookTimeMinutes":       parse_minutes(row["CookTime"]),
            "PrepTimeMinutes":       parse_minutes(row["PrepTime"]),
            "TotalTimeMinutes":      parse_minutes(row["TotalTime"]),
            "Images":                to_str_list(row["Images"]),
            "RecipeInstructions":    to_str_list(row["RecipeInstructions"]),
        }
    } for row in rows]

    run_bulk(es, docs, desc="Indexing recipes")


def index_reviews(es, df):
    print("\n=== Indexing Reviews ===")
    reset_index(es, "reviews", REVIEWS_MAPPING)

    rows = df.select(["RecipeId", "Rating", "Review"]).to_dicts()
    print(f"  Total reviews: {len(rows):,}")

    docs = [{
        "_index": "reviews",
        "_source": {
            "RecipeId":      str(row["RecipeId"]),
            "Rating":        row["Rating"],
            "Review":        row["Review"],
        }
    } for row in rows]

    run_bulk(es, docs, desc="Indexing reviews")


def main():
    es = Elasticsearch(ES_HOST)
    if not es.ping():
        raise ConnectionError(f"Cannot reach Elasticsearch at {ES_HOST}")
    print(f"Connected to Elasticsearch at {ES_HOST}")

    print("\nLoading merged.parquet...")
    df = pl.read_parquet(DATA_PATH)
    print(f"Shape: {df.shape}")

    # Keep only top-N recipes by quality score (rating × log popularity)
    unique = df.unique(subset=["RecipeId"])
    scored = unique.with_columns([
        (pl.col("AggregatedRating").fill_null(0) *
         (pl.col("ReviewCount").fill_null(0).cast(pl.Float64) + 1).log()
        ).alias("_score")
    ]).sort("_score", descending=True).head(TOP_N)
    top_ids = set(scored["RecipeId"].to_list())
    df = df.filter(pl.col("RecipeId").is_in(top_ids))
    print(f"Filtered to top {TOP_N:,} recipes → {len(df):,} rows (incl. reviews)")

    index_recipes(es, df)
    index_reviews(es, df)

    print("\n=== Summary ===")
    print(f"recipes index: {es.count(index='recipes')['count']:,} docs")
    print(f"reviews index: {es.count(index='reviews')['count']:,} docs")


if __name__ == "__main__":
    main()
