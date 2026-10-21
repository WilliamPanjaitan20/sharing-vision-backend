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

## Deploy demo online: Render + TiDB Cloud

Gunakan [TiDB Cloud Starter](https://tidbcloud.com/) sebagai database MySQL-compatible dan Render sebagai host API. Keduanya menyediakan kuota gratis untuk demo kecil.

1. Buat instance **MySQL Compatible Starter** di TiDB Cloud, lalu pada SQL Editor buat database `article` dan jalankan `migrations/001_create_posts.sql` di dalamnya.
2. Pada dialog **Connect** TiDB Cloud, salin host, port, username, dan password. Username TiDB memiliki prefix khusus; gunakan nilai tersebut apa adanya.
3. Di Render, buat **Web Service** dari repository backend ini dengan konfigurasi berikut:

   ```text
   Runtime: Go
   Build command: go build -tags netgo -ldflags '-s -w' -o app ./cmd/api
   Start command: ./app
   Health check path: /health
   Instance type: Free
   ```

4. Tambahkan environment variables berikut pada layanan Render. Jangan commit nilainya.

   ```env
   DB_HOST=<host-dari-TiDB>
   DB_PORT=4000
   DB_NAME=article
   DB_USER=<username-dari-TiDB>
   DB_PASSWORD=<password-dari-TiDB>
   DB_TLS=true
   ```

Setelah deploy selesai, salin URL API Render (misalnya `https://sharing-vision-api.onrender.com`) untuk dipakai oleh frontend. Layanan web gratis Render dapat berhenti saat tidak digunakan dan membutuhkan waktu sekitar satu menit saat diakses kembali.

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
