package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

var esHost = getEnv("ES_URL", "http://localhost:9200")
const recommenderURL = "http://localhost:5001/recommend"

var es *elasticsearch.Client
var userCol *mongo.Collection
var folderCol *mongo.Collection

var jwtSecret = []byte(getEnv("JWT_SECRET", "change-this-secret-in-production"))

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ── Elasticsearch types ──────────────────────────────────────────────────────

type Recipe struct {
	RecipeId              any      `json:"RecipeId"`
	Name                  string   `json:"Name"`
	Description           string   `json:"Description"`
	RecipeCategory        string   `json:"RecipeCategory"`
	Keywords              []string `json:"Keywords"`
	RecipeIngredientParts []string `json:"RecipeIngredientParts"`
	RecipeInstructions    []string `json:"RecipeInstructions"`
	AggregatedRating      float64  `json:"AggregatedRating"`
	ReviewCount           int      `json:"ReviewCount"`
	Calories              float64  `json:"Calories"`
	ProteinContent        float64  `json:"ProteinContent"`
	FatContent            float64  `json:"FatContent"`
	CarbohydrateContent   float64  `json:"CarbohydrateContent"`
	CookTimeMinutes       int      `json:"CookTimeMinutes"`
	PrepTimeMinutes       int      `json:"PrepTimeMinutes"`
	TotalTimeMinutes      int      `json:"TotalTimeMinutes"`
	RecipeServings        int      `json:"RecipeServings"`
	Images                []string `json:"Images"`
}

type Review struct {
	RecipeId string `json:"RecipeId"`
	Rating   int    `json:"Rating"`
	Review   string `json:"Review"`
}

type RecipeDetail struct {
	Recipe  Recipe   `json:"recipe"`
	Reviews []Review `json:"reviews"`
}

type SearchResponse struct {
	Total   int      `json:"total"`
	Results []Recipe `json:"results"`
}

// ── MongoDB types ────────────────────────────────────────────────────────────

type User struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Username  string             `bson:"username"      json:"username"`
	Email     string             `bson:"email"         json:"email"`
	Password  string             `bson:"password"      json:"-"`
	Bookmarks []string           `bson:"bookmarks"     json:"bookmarks"`
	CreatedAt time.Time          `bson:"createdAt"     json:"createdAt"`
}

type Folder struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    primitive.ObjectID `bson:"userId"        json:"-"`
	Name      string             `bson:"name"          json:"name"`
	RecipeIDs []string           `bson:"recipeIds"     json:"recipeIds"`
	CreatedAt time.Time          `bson:"createdAt"     json:"createdAt"`
}

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token     string   `json:"token"`
	Username  string   `json:"username"`
	Email     string   `json:"email"`
	Id        string   `json:"id"`
	Bookmarks []string `json:"bookmarks"`
}

type BookmarkRequest struct {
	RecipeId string `json:"recipeId"`
	FolderId string `json:"folderId"`
}

type FolderRequest struct {
	Name string `json:"name"`
}

// ── Middleware ───────────────────────────────────────────────────────────────

var allowedOrigin = getEnv("ALLOWED_ORIGIN", "http://localhost:5173")

func cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return jwtSecret, nil
		})
		if err != nil || !token.Valid {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		claims := token.Claims.(jwt.MapClaims)
		ctx := context.WithValue(r.Context(), "userId", claims["id"].(string))
		next(w, r.WithContext(ctx))
	}
}

// ── Auth handlers ────────────────────────────────────────────────────────────

func registerHandler(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.Username == "" || req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"username, email and password are required"}`, http.StatusBadRequest)
		return
	}
	if len(req.Password) < 6 {
		http.Error(w, `{"error":"password must be at least 6 characters"}`, http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	var existing User
	err := userCol.FindOne(ctx, bson.M{"email": req.Email}).Decode(&existing)
	if err == nil {
		http.Error(w, `{"error":"email already registered"}`, http.StatusConflict)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	user := User{
		Username:  req.Username,
		Email:     req.Email,
		Password:  string(hashed),
		Bookmarks: []string{},
		CreatedAt: time.Now(),
	}

	res, err := userCol.InsertOne(ctx, user)
	if err != nil {
		http.Error(w, `{"error":"could not create user"}`, http.StatusInternalServerError)
		return
	}

	user.ID = res.InsertedID.(primitive.ObjectID)
	token, err := makeToken(user)
	if err != nil {
		http.Error(w, `{"error":"could not create token"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AuthResponse{
		Token:     token,
		Username:  user.Username,
		Email:     user.Email,
		Id:        user.ID.Hex(),
		Bookmarks: user.Bookmarks,
	})
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"email and password are required"}`, http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	var user User
	err := userCol.FindOne(ctx, bson.M{"email": req.Email}).Decode(&user)
	if err != nil {
		http.Error(w, `{"error":"invalid email or password"}`, http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		http.Error(w, `{"error":"invalid email or password"}`, http.StatusUnauthorized)
		return
	}

	token, err := makeToken(user)
	if err != nil {
		http.Error(w, `{"error":"could not create token"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AuthResponse{
		Token:     token,
		Username:  user.Username,
		Email:     user.Email,
		Id:        user.ID.Hex(),
		Bookmarks: user.Bookmarks,
	})
}

func makeToken(user User) (string, error) {
	claims := jwt.MapClaims{
		"id":       user.ID.Hex(),
		"username": user.Username,
		"email":    user.Email,
		"exp":      time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
}

// ── Bookmark handlers ────────────────────────────────────────────────────────

// POST /auth/bookmark  — add recipe to a folder
// DELETE /auth/bookmark — remove recipe from all folders
func bookmarkHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		addBookmarkHandler(w, r)
	case http.MethodDelete:
		removeBookmarkHandler(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func addBookmarkHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value("userId").(string)
	userObjId, err := primitive.ObjectIDFromHex(userId)
	if err != nil {
		http.Error(w, `{"error":"invalid user"}`, http.StatusBadRequest)
		return
	}

	var req BookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RecipeId == "" || req.FolderId == "" {
		http.Error(w, `{"error":"recipeId and folderId required"}`, http.StatusBadRequest)
		return
	}

	folderObjId, err := primitive.ObjectIDFromHex(req.FolderId)
	if err != nil {
		http.Error(w, `{"error":"invalid folderId"}`, http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	count, _ := folderCol.CountDocuments(ctx, bson.M{"_id": folderObjId, "userId": userObjId})
	if count == 0 {
		http.Error(w, `{"error":"folder not found"}`, http.StatusNotFound)
		return
	}

	folderCol.UpdateOne(ctx, bson.M{"_id": folderObjId}, bson.M{"$addToSet": bson.M{"recipeIds": req.RecipeId}})
	userCol.UpdateOne(ctx, bson.M{"_id": userObjId}, bson.M{"$addToSet": bson.M{"bookmarks": req.RecipeId}})

	var user User
	userCol.FindOne(ctx, bson.M{"_id": userObjId}).Decode(&user)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"bookmarks": user.Bookmarks})
}

func removeBookmarkHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value("userId").(string)
	userObjId, err := primitive.ObjectIDFromHex(userId)
	if err != nil {
		http.Error(w, `{"error":"invalid user"}`, http.StatusBadRequest)
		return
	}

	var req BookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RecipeId == "" {
		http.Error(w, `{"error":"recipeId required"}`, http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	folderCol.UpdateMany(ctx, bson.M{"userId": userObjId}, bson.M{"$pull": bson.M{"recipeIds": req.RecipeId}})
	userCol.UpdateOne(ctx, bson.M{"_id": userObjId}, bson.M{"$pull": bson.M{"bookmarks": req.RecipeId}})

	var user User
	userCol.FindOne(ctx, bson.M{"_id": userObjId}).Decode(&user)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"bookmarks": user.Bookmarks})
}

// ── Folder handlers ──────────────────────────────────────────────────────────

// GET/POST /user/folders
func foldersListHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getUserFolders(w, r)
	case http.MethodPost:
		createFolder(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func getUserFolders(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value("userId").(string)
	userObjId, _ := primitive.ObjectIDFromHex(userId)

	ctx := context.Background()
	cursor, err := folderCol.Find(ctx, bson.M{"userId": userObjId})
	if err != nil {
		http.Error(w, `{"error":"could not fetch folders"}`, http.StatusInternalServerError)
		return
	}
	defer cursor.Close(ctx)

	var folders []Folder
	cursor.All(ctx, &folders)
	if folders == nil {
		folders = []Folder{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(folders)
}

func createFolder(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value("userId").(string)
	userObjId, _ := primitive.ObjectIDFromHex(userId)

	var req FolderRequest
	json.NewDecoder(r.Body).Decode(&req)
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, `{"error":"name required"}`, http.StatusBadRequest)
		return
	}

	folder := Folder{
		UserID:    userObjId,
		Name:      req.Name,
		RecipeIDs: []string{},
		CreatedAt: time.Now(),
	}

	ctx := context.Background()
	res, err := folderCol.InsertOne(ctx, folder)
	if err != nil {
		http.Error(w, `{"error":"could not create folder"}`, http.StatusInternalServerError)
		return
	}

	folder.ID = res.InsertedID.(primitive.ObjectID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(folder)
}

// DELETE /user/folders/{id}          — delete entire folder
// POST   /user/folders/{id}/remove   — remove one recipe from folder
func folderItemHandler(w http.ResponseWriter, r *http.Request) {
	// Parse path: everything after /user/folders/
	rest := strings.TrimPrefix(r.URL.Path, "/user/folders/")
	parts := strings.SplitN(rest, "/", 2)
	folderId := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case r.Method == http.MethodDelete && action == "":
		deleteFolderHandler(w, r, folderId)
	case r.Method == http.MethodPost && action == "remove":
		removeFromFolderHandler(w, r, folderId)
	default:
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	}
}

func deleteFolderHandler(w http.ResponseWriter, r *http.Request, folderId string) {
	userId := r.Context().Value("userId").(string)
	userObjId, _ := primitive.ObjectIDFromHex(userId)
	folderObjId, err := primitive.ObjectIDFromHex(folderId)
	if err != nil {
		http.Error(w, `{"error":"invalid folder id"}`, http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	var folder Folder
	if err := folderCol.FindOne(ctx, bson.M{"_id": folderObjId, "userId": userObjId}).Decode(&folder); err != nil {
		http.Error(w, `{"error":"folder not found"}`, http.StatusNotFound)
		return
	}

	folderCol.DeleteOne(ctx, bson.M{"_id": folderObjId, "userId": userObjId})

	// Recipes no longer in any folder should be removed from user.bookmarks
	cursor, _ := folderCol.Find(ctx, bson.M{"userId": userObjId})
	var remaining []Folder
	cursor.All(ctx, &remaining)
	cursor.Close(ctx)

	stillBookmarked := map[string]bool{}
	for _, f := range remaining {
		for _, id := range f.RecipeIDs {
			stillBookmarked[id] = true
		}
	}

	for _, id := range folder.RecipeIDs {
		if !stillBookmarked[id] {
			userCol.UpdateOne(ctx, bson.M{"_id": userObjId}, bson.M{"$pull": bson.M{"bookmarks": id}})
		}
	}

	var user User
	userCol.FindOne(ctx, bson.M{"_id": userObjId}).Decode(&user)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"bookmarks": user.Bookmarks})
}

func removeFromFolderHandler(w http.ResponseWriter, r *http.Request, folderId string) {
	userId := r.Context().Value("userId").(string)
	userObjId, _ := primitive.ObjectIDFromHex(userId)
	folderObjId, err := primitive.ObjectIDFromHex(folderId)
	if err != nil {
		http.Error(w, `{"error":"invalid folder id"}`, http.StatusBadRequest)
		return
	}

	var req BookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RecipeId == "" {
		http.Error(w, `{"error":"recipeId required"}`, http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	folderCol.UpdateOne(ctx, bson.M{"_id": folderObjId, "userId": userObjId}, bson.M{"$pull": bson.M{"recipeIds": req.RecipeId}})

	// Check if recipe is still in any other folder
	count, _ := folderCol.CountDocuments(ctx, bson.M{"userId": userObjId, "recipeIds": req.RecipeId})
	if count == 0 {
		userCol.UpdateOne(ctx, bson.M{"_id": userObjId}, bson.M{"$pull": bson.M{"bookmarks": req.RecipeId}})
	}

	var user User
	userCol.FindOne(ctx, bson.M{"_id": userObjId}).Decode(&user)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"bookmarks": user.Bookmarks})
}

// ── Elasticsearch handlers ───────────────────────────────────────────────────

func parseHits(raw map[string]any) []Recipe {
	hits := raw["hits"].(map[string]any)["hits"].([]any)
	var results []Recipe
	for _, hit := range hits {
		src := hit.(map[string]any)["_source"]
		b, _ := json.Marshal(src)
		var recipe Recipe
		json.Unmarshal(b, &recipe)
		recipe.RecipeId = hit.(map[string]any)["_id"]
		results = append(results, recipe)
	}
	return results
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, `{"error": "missing ?q="}`, http.StatusBadRequest)
		return
	}

	query := map[string]any{
		"size": 10,
		"query": map[string]any{
			"multi_match": map[string]any{
				"query":  q,
				"fields": []string{"Name^3", "Keywords^2", "RecipeIngredientParts^2", "Description", "RecipeCategory"},
				"type":   "best_fields",
			},
		},
	}

	body, _ := json.Marshal(query)
	res, err := es.Search(
		es.Search.WithContext(context.Background()),
		es.Search.WithIndex("recipes"),
		es.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	defer res.Body.Close()

	var raw map[string]any
	json.NewDecoder(res.Body).Decode(&raw)

	total := int(raw["hits"].(map[string]any)["total"].(map[string]any)["value"].(float64))
	results := parseHits(raw)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SearchResponse{Total: total, Results: results})
}

func randomHandler(w http.ResponseWriter, r *http.Request) {
	query := map[string]any{
		"size": 12,
		"query": map[string]any{
			"function_score": map[string]any{
				"query":     map[string]any{"match_all": map[string]any{}},
				"functions": []map[string]any{{"random_score": map[string]any{}}},
			},
		},
	}

	body, _ := json.Marshal(query)
	res, err := es.Search(
		es.Search.WithContext(context.Background()),
		es.Search.WithIndex("recipes"),
		es.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	defer res.Body.Close()

	var raw map[string]any
	json.NewDecoder(res.Body).Decode(&raw)
	results := parseHits(raw)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SearchResponse{Total: len(results), Results: results})
}

func recipeHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/recipe/")
	if id == "" {
		http.Error(w, `{"error": "missing recipe id"}`, http.StatusBadRequest)
		return
	}

	res, err := es.Get("recipes", id,
		es.Get.WithContext(context.Background()),
	)
	if err != nil || res.IsError() {
		http.Error(w, `{"error": "recipe not found"}`, http.StatusNotFound)
		return
	}
	defer res.Body.Close()

	var raw map[string]any
	json.NewDecoder(res.Body).Decode(&raw)

	src := raw["_source"]
	b, _ := json.Marshal(src)
	var recipe Recipe
	json.Unmarshal(b, &recipe)
	recipe.RecipeId = raw["_id"]

	reviewQuery := map[string]any{
		"size": 20,
		"query": map[string]any{
			"term": map[string]any{"RecipeId": id},
		},
	}

	reviewBody, _ := json.Marshal(reviewQuery)
	reviewRes, err := es.Search(
		es.Search.WithContext(context.Background()),
		es.Search.WithIndex("reviews"),
		es.Search.WithBody(bytes.NewReader(reviewBody)),
	)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	defer reviewRes.Body.Close()

	var reviewRaw map[string]any
	json.NewDecoder(reviewRes.Body).Decode(&reviewRaw)

	var reviews []Review
	if hitsOuter, ok := reviewRaw["hits"].(map[string]any); ok {
		if reviewHits, ok := hitsOuter["hits"].([]any); ok {
			for _, hit := range reviewHits {
				rb, _ := json.Marshal(hit.(map[string]any)["_source"])
				var review Review
				json.Unmarshal(rb, &review)
				reviews = append(reviews, review)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RecipeDetail{Recipe: recipe, Reviews: reviews})
}

func batchRecipesHandler(w http.ResponseWriter, r *http.Request) {
	idsParam := r.URL.Query().Get("ids")
	if idsParam == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Recipe{})
		return
	}

	type mgetDoc struct {
		ID string `json:"_id"`
	}
	type mgetBody struct {
		Docs []mgetDoc `json:"docs"`
	}

	var docs []mgetDoc
	for _, id := range strings.Split(idsParam, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			docs = append(docs, mgetDoc{ID: id})
		}
	}

	bodyBytes, _ := json.Marshal(mgetBody{Docs: docs})
	res, err := es.Mget(
		bytes.NewReader(bodyBytes),
		es.Mget.WithIndex("recipes"),
		es.Mget.WithContext(context.Background()),
	)
	if err != nil || res.IsError() {
		http.Error(w, `{"error":"failed to fetch recipes"}`, http.StatusInternalServerError)
		return
	}
	defer res.Body.Close()

	var raw map[string]any
	json.NewDecoder(res.Body).Decode(&raw)

	var recipes []Recipe
	for _, d := range raw["docs"].([]any) {
		doc := d.(map[string]any)
		if found, _ := doc["found"].(bool); !found {
			continue
		}
		b, _ := json.Marshal(doc["_source"])
		var recipe Recipe
		json.Unmarshal(b, &recipe)
		recipe.RecipeId = doc["_id"]
		recipes = append(recipes, recipe)
	}

	if recipes == nil {
		recipes = []Recipe{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recipes)
}

// ── Recommendations handler ───────────────────────────────────────────────────

func recommendHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value("userId").(string)
	userObjId, err := primitive.ObjectIDFromHex(userId)
	if err != nil {
		http.Error(w, `{"error":"invalid user"}`, http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	empty := SearchResponse{Total: 0, Results: []Recipe{}}

	// Resolve recipe IDs: use the specified folder only, not all bookmarks.
	folderID := r.URL.Query().Get("folder_id")
	var likedIDs []string
	if folderID != "" {
		folderObjId, err := primitive.ObjectIDFromHex(folderID)
		if err != nil {
			http.Error(w, `{"error":"invalid folder_id"}`, http.StatusBadRequest)
			return
		}
		var folder Folder
		if err := folderCol.FindOne(ctx, bson.M{"_id": folderObjId, "userId": userObjId}).Decode(&folder); err != nil {
			http.Error(w, `{"error":"folder not found"}`, http.StatusNotFound)
			return
		}
		likedIDs = folder.RecipeIDs
	} else {
		var user User
		if err := userCol.FindOne(ctx, bson.M{"_id": userObjId}).Decode(&user); err != nil {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}
		likedIDs = user.Bookmarks
	}

	if len(likedIDs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(empty)
		return
	}

	reqBody, _ := json.Marshal(map[string]any{"liked_ids": likedIDs, "top_n": 12})
	resp, err := http.Post(recommenderURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(empty)
		return
	}
	defer resp.Body.Close()

	var pyResp struct {
		RecipeIDs []json.Number `json:"recipe_ids"`
	}
	json.NewDecoder(resp.Body).Decode(&pyResp)
	if len(pyResp.RecipeIDs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(empty)
		return
	}

	type mgetDoc struct {
		ID string `json:"_id"`
	}
	type mgetBody struct {
		Docs []mgetDoc `json:"docs"`
	}
	var docs []mgetDoc
	for _, id := range pyResp.RecipeIDs {
		docs = append(docs, mgetDoc{ID: id.String()})
	}

	bodyBytes, _ := json.Marshal(mgetBody{Docs: docs})
	res, err := es.Mget(
		bytes.NewReader(bodyBytes),
		es.Mget.WithIndex("recipes"),
		es.Mget.WithContext(ctx),
	)
	if err != nil || res.IsError() {
		http.Error(w, `{"error":"failed to fetch recipes"}`, http.StatusInternalServerError)
		return
	}
	defer res.Body.Close()

	var raw map[string]any
	json.NewDecoder(res.Body).Decode(&raw)

	var recipes []Recipe
	for _, d := range raw["docs"].([]any) {
		doc := d.(map[string]any)
		if found, _ := doc["found"].(bool); !found {
			continue
		}
		b, _ := json.Marshal(doc["_source"])
		var recipe Recipe
		json.Unmarshal(b, &recipe)
		recipe.RecipeId = doc["_id"]
		recipes = append(recipes, recipe)
	}
	if recipes == nil {
		recipes = []Recipe{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SearchResponse{Total: len(recipes), Results: recipes})
}

// ── Main ─────────────────────────────────────────────────────────────────────

func main() {
	godotenv.Load("../.env")

	// Parse credentials from ES URL (supports https://user:pass@host format for Bonsai)
	parsedURL, _ := url.Parse(esHost)
	cfg := elasticsearch.Config{Addresses: []string{esHost}}
	if parsedURL.User != nil {
		password, _ := parsedURL.User.Password()
		cfg = elasticsearch.Config{
			Addresses: []string{parsedURL.Scheme + "://" + parsedURL.Host},
			Username:  parsedURL.User.Username(),
			Password:  password,
		}
	}
	var err error
	es, err = elasticsearch.NewClient(cfg)
	if err != nil {
		log.Fatalf("Error creating ES client: %s", err)
	}
	// Use raw HTTP ping so it works with both Elasticsearch and OpenSearch (Bonsai)
	pingReq, _ := http.NewRequest("GET", parsedURL.Scheme+"://"+parsedURL.Host, nil)
	if parsedURL.User != nil {
		password, _ := parsedURL.User.Password()
		pingReq.SetBasicAuth(parsedURL.User.Username(), password)
	}
	pingResp, pingErr := http.DefaultClient.Do(pingReq)
	if pingErr != nil || pingResp.StatusCode >= 500 {
		log.Fatalf("Cannot reach Elasticsearch at %s", esHost)
	}
	log.Println("Connected to Elasticsearch")

	mongoURI := getEnv("MONGODB_URI", "")
	if mongoURI == "" {
		log.Fatal("MONGODB_URI environment variable is required")
	}
	mongoClient, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("MongoDB connect error: %s", err)
	}
	if err := mongoClient.Ping(context.Background(), nil); err != nil {
		log.Fatalf("Cannot reach MongoDB: %s", err)
	}
	log.Println("Connected to MongoDB")
	db := mongoClient.Database("projectir")
	userCol = db.Collection("users")
	folderCol = db.Collection("folders")

	http.HandleFunc("/search", cors(searchHandler))
	http.HandleFunc("/random", cors(randomHandler))
	http.HandleFunc("/recipe/", cors(recipeHandler))
	http.HandleFunc("/recipes/batch", cors(batchRecipesHandler))
	http.HandleFunc("/auth/register", cors(registerHandler))
	http.HandleFunc("/auth/login", cors(loginHandler))
	http.HandleFunc("/auth/bookmark", cors(requireAuth(bookmarkHandler)))
	http.HandleFunc("/user/folders", cors(requireAuth(foldersListHandler)))
	http.HandleFunc("/user/folders/", cors(requireAuth(folderItemHandler)))
	http.HandleFunc("/recommendations", cors(requireAuth(recommendHandler)))

	log.Println("Server running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
