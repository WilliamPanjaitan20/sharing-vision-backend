# Sharing Vision Backend

API Go untuk tes Sharing Vision. API ini menyimpan artikel pada MySQL dan menyediakan endpoint create, read, update, serta soft delete.

## Prasyarat

- Go 1.27+
- MySQL 8+ pada port `3306`
- phpMyAdmin (opsional)

## Siapkan database melalui phpMyAdmin

1. Buka phpMyAdmin dan pilih tab **Databases**.
2. Buat database bernama `article` dengan collation `utf8mb4_unicode_ci`.
3. Pilih database `article`, buka tab **SQL**, lalu jalankan isi file `migrations/001_create_posts.sql`.
4. Buat user khusus aplikasi melalui tab **User accounts** dan beri akses penuh hanya ke database `article`.
5. Salin `.env.example` menjadi `.env`, lalu sesuaikan nilai `DB_USER` dan `DB_PASSWORD`.

Contoh konfigurasi MySQL lokal:

```env
PORT=8080
DB_HOST=localhost
DB_PORT=3306
DB_NAME=article
DB_USER=article_user
DB_PASSWORD=ganti-dengan-password-anda
```

## Jalankan API

```powershell
go mod download
go run ./cmd/api
```

API berjalan pada `http://localhost:8080`. Periksa koneksi database melalui `http://localhost:8080/health`.

## Endpoint

| Method | Endpoint | Keterangan |
| --- | --- | --- |
| `POST` | `/article/` | Membuat artikel |
| `GET` | `/article/{limit}/{offset}` | Menampilkan artikel dengan pagination |
| `GET` | `/article/{id}` | Menampilkan satu artikel |
| `PUT` / `PATCH` | `/article/{id}` | Mengubah artikel |
| `DELETE` | `/article/{id}` | Memindahkan artikel ke Trashed |

## Pengujian

Import `postman/Sharing-Vision-Article.postman_collection.json` ke Postman untuk mencoba seluruh endpoint.

```powershell
go test ./...
```
