package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

const maxBodyBytes = 1 << 20

type application struct {
	db *sql.DB
}

type post struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Category    string    `json:"category"`
	Status      string    `json:"status"`
	CreatedDate time.Time `json:"created_date"`
	UpdatedDate time.Time `json:"updated_date"`
}

type postInput struct {
	Title    string `json:"title"`
	Content  string `json:"content"`
	Category string `json:"category"`
	Status   string `json:"status"`
}

func main() {
	// Docker supplies configuration through its environment.  Loading this file
	// makes the documented local Backend/.env configuration work as well, while
	// preserving explicitly supplied environment variables.
	if err := loadEnvFile(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("load .env: %v", err)
	}

	db, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	app := application{db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", app.health)
	mux.HandleFunc("POST /article/", app.createPost)
	mux.HandleFunc("GET /article/{limit}/{offset}", app.listPosts)
	mux.HandleFunc("GET /article/{id}", app.getPost)
	mux.HandleFunc("PUT /article/{id}", app.updatePost)
	mux.HandleFunc("PATCH /article/{id}", app.updatePost)
	mux.HandleFunc("DELETE /article/{id}", app.trashPost)

	port := env("PORT", "8080")
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           cors(recoverer(mux)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("API listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

func openDB() (*sql.DB, error) {
	cfg := mysql.Config{
		User:                 env("DB_USER", "article_user"),
		Passwd:               env("DB_PASSWORD", "article_password"),
		Net:                  "tcp",
		Addr:                 env("DB_HOST", "localhost") + ":" + env("DB_PORT", "3306"),
		DBName:               env("DB_NAME", "article"),
		ParseTime:            true,
		Loc:                  time.UTC,
		TLSConfig:            env("DB_TLS", "false"),
		AllowNativePasswords: true,
	}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(3 * time.Minute)

	deadline := time.Now().Add(45 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := db.PingContext(ctx)
		cancel()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			db.Close()
			return nil, fmt.Errorf("connect database after retries: %w", err)
		}
		log.Printf("database is not ready yet; retrying in 2 seconds: %v", err)
		time.Sleep(2 * time.Second)
	}
	return db, nil
}

func (app application) health(w http.ResponseWriter, r *http.Request) {
	if err := app.db.PingContext(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (app application) createPost(w http.ResponseWriter, r *http.Request) {
	input, ok := decodeAndValidatePost(w, r)
	if !ok {
		return
	}
	result, err := app.db.ExecContext(r.Context(), `
		INSERT INTO posts (title, content, category, status) VALUES (?, ?, ?, ?)`,
		input.Title, input.Content, input.Category, input.Status,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create article")
		return
	}
	id, _ := result.LastInsertId()
	created, err := app.postByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "article was created but could not be read")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (app application) listPosts(w http.ResponseWriter, r *http.Request) {
	limit, err := strconv.Atoi(r.PathValue("limit"))
	if err != nil || limit < 1 || limit > 100 {
		writeError(w, http.StatusBadRequest, "limit must be between 1 and 100")
		return
	}
	offset, err := strconv.Atoi(r.PathValue("offset"))
	if err != nil || offset < 0 {
		writeError(w, http.StatusBadRequest, "offset must be zero or greater")
		return
	}
	rows, err := app.db.QueryContext(r.Context(), `
		SELECT id, title, content, category, status, created_date, updated_date
		FROM posts ORDER BY updated_date DESC, id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list articles")
		return
	}
	defer rows.Close()
	posts := make([]post, 0)
	for rows.Next() {
		item, err := scanPost(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not read articles")
			return
		}
		posts = append(posts, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not read articles")
		return
	}
	writeJSON(w, http.StatusOK, posts)
}

func (app application) getPost(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, err := app.postByID(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "article not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read article")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app application) updatePost(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	input, ok := decodeAndValidatePost(w, r)
	if !ok {
		return
	}
	_, err := app.db.ExecContext(r.Context(), `
		UPDATE posts SET title = ?, content = ?, category = ?, status = ? WHERE id = ?`,
		input.Title, input.Content, input.Category, input.Status, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update article")
		return
	}
	item, err := app.postByID(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "article not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "article was updated but could not be read")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// trashPost is a soft delete so articles remain available in the Trashed tab.
func (app application) trashPost(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	result, err := app.db.ExecContext(r.Context(), "UPDATE posts SET status = 'thrash' WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not trash article")
		return
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		writeError(w, http.StatusNotFound, "article not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (app application) postByID(ctx context.Context, id int64) (post, error) {
	row := app.db.QueryRowContext(ctx, `
		SELECT id, title, content, category, status, created_date, updated_date
		FROM posts WHERE id = ?`, id)
	return scanPost(row)
}

type scanner interface{ Scan(...any) error }

func scanPost(row scanner) (post, error) {
	var item post
	err := row.Scan(&item.ID, &item.Title, &item.Content, &item.Category, &item.Status, &item.CreatedDate, &item.UpdatedDate)
	return item, err
}

func decodeAndValidatePost(w http.ResponseWriter, r *http.Request) (postInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input postInput
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be valid JSON")
		return postInput{}, false
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Content = strings.TrimSpace(input.Content)
	input.Category = strings.TrimSpace(input.Category)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))

	problems := map[string]string{}
	if len([]rune(input.Title)) < 20 {
		problems["title"] = "must contain at least 20 characters"
	}
	if len([]rune(input.Content)) < 200 {
		problems["content"] = "must contain at least 200 characters"
	}
	if len([]rune(input.Category)) < 3 {
		problems["category"] = "must contain at least 3 characters"
	}
	if input.Status != "publish" && input.Status != "draft" && input.Status != "thrash" {
		problems["status"] = "must be publish, draft, or thrash"
	}
	if len(problems) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation failed", "fields": problems})
		return postInput{}, false
	}
	return input, true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "id must be a positive integer")
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// loadEnvFile reads simple KEY=VALUE entries from a local .env file. Values
// already provided by the process environment take precedence, so Docker and
// deployment configuration cannot be overwritten by a development file.
func loadEnvFile(path string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	for lineNumber, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return fmt.Errorf("line %d must use KEY=VALUE format", lineNumber+1)
		}

		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		if _, isSet := os.LookupEnv(key); !isSet {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set %s: %w", key, err)
			}
		}
	}
	return nil
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
