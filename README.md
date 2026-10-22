# API Students — Backend Service (Go Fiber, PostgreSQL, MongoDB)

Layanan backend RESTful API untuk sistem manajemen mahasiswa, Kartu Rencana Studi (KRS), Dosen Pembimbing Tugas Akhir, Ulasan Mata Kuliah (Course Reviews), dan Portofolio Mahasiswa (Student Portfolio).

Dibangun menggunakan **Go**, **Fiber v2**, **PostgreSQL** (via `pgxpool v5`), dan **MongoDB** (via official MongoDB Go Driver v2) dengan arsitektur bersih (*Clean Architecture*: Handler → Service → Repository).

---

## 1. Arsitektur dan Sumber Data

Aplikasi mengadopsi pemisahan tanggung jawab penyimpanan data secara tegas (*polyglot persistence*):
1. **PostgreSQL**: Sumber kebenaran data relasional utama yang menuntut jaminan integritas ACID:
   - Data akun pengguna (`users`), peran & hak akses (`roles`, `permissions`, `role_permissions`).
   - Data induk mahasiswa (`students`), prestasi (`prestasi`).
   - Data kelas perkuliahan (`courses`) dan pendaftaran KRS (`enrollments`).
   - Data dosen pembimbing (`supervisors`) dan penetapan bimbingan tugas akhir (`supervisions`).
2. **MongoDB**: Dokumen fleksibel untuk agregasi konten dinamis dan statistik:
   - `course_reviews`: Ulasan kelas, rating 1–5, feedback komentar, tag dinamis, balasan (*replies*, maks 50), dan *helpful voting*.
   - `student_portfolios`: Portofolio mahasiswa (headline, bio), proyek terstruktur (maks 20 atomik), sertifikat, dan agregasi teknologi.

---

## 2. Prasyarat Sistem

- **Go**: Versi 1.22 atau lebih baru.
- **PostgreSQL**: Versi 14 atau lebih baru (Port 5432).
- **MongoDB**: Versi 6.0 atau lebih baru (Port 27017).

---

## 3. Konfigurasi Environment

Salin file template `.env.example` menjadi `.env`:

```bash
cp .env.example .env
```

Sesuaikan parameter konfigurasi di dalam file `.env`:

```env
# Koneksi PostgreSQL
DATABASE_URL=postgres://postgres:postgres@localhost:5432/praktikum_backend?sslmode=disable

# Konfigurasi JWT
JWT_SECRET=rahasia_jwt_super_aman_untuk_pengembangan_lokal
JWT_ISSUER=api-students
JWT_ACCESS_TTL=15m
JWT_REFRESH_TTL=168h

# Koneksi MongoDB Driver v2
MONGO_URI=mongodb://localhost:27017
MONGO_DATABASE=praktikum_backend
MONGO_MAX_POOL=20

# Port HTTP Server (opsional, default 3000)
PORT=3000
```

> **Keamanan Kredensial**: File `.env` berisi rahasia lokal dan telah masuk dalam `.gitignore`. Jangan pernah menambahkan `.env` atau credential asli ke dalam version control Git.

---

## 4. Menjalankan Database

### Menjalankan PostgreSQL dan MongoDB Lokal
Pastikan service database berjalan di host lokal:
```bash
# Contoh menggunakan service lokal (Windows PowerShell / Services)
# PostgreSQL service: postgresql-x64-16
# MongoDB service: MongoDB

# Atau menggunakan Docker:
docker run -d --name postgres -p 5432:5432 -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=praktikum_backend postgres:16
docker run -d --name mongodb -p 27017:27017 mongo:7.0
```

---

## 5. Menjalankan Migrasi PostgreSQL

Migrasi database PostgreSQL tersimpan secara berurutan pada direktori `migrations/`. Jalankan file SQL secara urut:

```bash
# Menggunakan psql CLI
psql -U postgres -d praktikum_backend -f migrations/001_create_students.sql
psql -U postgres -d praktikum_backend -f migrations/002_auth.sql
psql -U postgres -d praktikum_backend -f migrations/003_rbac.sql
psql -U postgres -d praktikum_backend -f migrations/004_student_permissions.sql
psql -U postgres -d praktikum_backend -f migrations/005_create_prestasi.sql
psql -U postgres -d praktikum_backend -f migrations/006_cursor_students.sql
psql -U postgres -d praktikum_backend -f migrations/007_courses.sql
psql -U postgres -d praktikum_backend -f migrations/008_seed_loadtest.sql
psql -U postgres -d praktikum_backend -f migrations/009_supervisors.sql
psql -U postgres -d praktikum_backend -f migrations/010_review_permissions.sql
```

Index MongoDB dibuat secara otomatis oleh aplikasi pada saat startup (`reviewRepository.EnsureIndexes(ctx)` dan `portfolioRepository.EnsureIndexes(ctx)`).

---

## 6. Menjalankan Server

Jalankan server aplikasi:
```bash
go run main.go
```
Aplikasi akan aktif di `http://localhost:3000`.

### Graceful Shutdown
Aplikasi mendengarkan sinyal OS `SIGINT` (Ctrl+C) dan `SIGTERM`. Saat sinyal diterima, server Fiber akan berhenti menerima request baru, menyelesaikan in-flight request dalam batas waktu timeout 5 detik, kemudian menutup koneksi MongoDB dan pool PostgreSQL secara bersih tanpa kebocoran resource.

---

## 7. Health Check

Aplikasi menyediakan endpoint pemantauan kesehatan ganda (root dan API v1):
- `GET /health`
- `GET /api/v1/health`

**Contoh Response Sehat (200 OK):**
```json
{
  "status": "success",
  "message": "server, PostgreSQL, dan MongoDB berjalan",
  "data": null
}
```

Bila salah satu database tidak dapat dihubungi dalam batas waktu 2 detik, endpoint mengembalikan HTTP **503 Service Unavailable** beserta keterangan database yang gagal.

---

## 8. Autentikasi dan Otorisasi (RBAC)

1. **Header Autentikasi**:
   Semua endpoint terproteksi membutuhkan header:
   ```http
   Authorization: Bearer <access_token>
   ```
2. **Role & Permission**:
   - `student`: Hak akses standar untuk KRS, melihat kelas, mengelola portofolio miliknya sendiri, menulis ulasan kelas, dan voting ulasan.
   - `admin`: Hak administratif penuh (`student:manage`, `course:manage`, `supervision:manage`, `review:moderate`).
3. **Pemeriksaan Kepemilikan (Ownership)**:
   - Endpoint portofolio (`PUT`, `POST`, `DELETE`) dan review (`PATCH`, `DELETE`) memvalidasi kepemilikan data: pengguna biasa hanya diizinkan memodifikasi resource miliknya sendiri, kecuali jika memiliki hak akses administratif.

---

## 9. Daftar Endpoint Modul 9 dan Modul 10

### A. Modul 9 — Transaksi KRS & Courses (`/api/v1/courses`, `/api/v1/enrollments`)

| Method | Endpoint | Hak Akses | Deskripsi |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/v1/courses` | Authenticated | Menampilkan daftar seluruh mata kuliah. |
| `GET` | `/api/v1/courses/mine` | Authenticated | Menampilkan daftar mata kuliah yang diambil mahasiswa saat ini. |
| `GET` | `/api/v1/courses/:id` | Authenticated | Menampilkan detail mata kuliah berdasarkan ID. |
| `POST` | `/api/v1/courses/:id/enroll` | Authenticated | Mendaftar ke kelas secara transaksional (validasi kuota atomik & maks 24 SKS). |
| `DELETE` | `/api/v1/courses/:id/enroll` | Authenticated | Membatalkan pendaftaran kelas (*drop*) dan mengembalikan kuota kursi. |
| `PATCH` | `/api/v1/courses/:id` | `course:manage` | Mengubah kuota kelas dengan *optimistic locking* (validasi versi). |
| `POST` | `/api/v1/enrollments/swap` | Authenticated | Pertukaran kelas atomik (*drop* kelas lama dan *enroll* kelas baru dalam 1 transaksi). |

### B. Modul 9 — Tugas Mandiri Sistem Dosen Pembimbing (`/api/v1/supervisors`, `/api/v1/supervisions`)

| Method | Endpoint | Hak Akses | Deskripsi |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/v1/supervisors` | Authenticated | Menampilkan daftar seluruh dosen pembimbing beserta sisa kuota. |
| `GET` | `/api/v1/supervisors/:id` | Authenticated | Menampilkan detail dosen pembimbing. |
| `POST` | `/api/v1/supervisors/:id/assign` | `supervision:manage` | Menetapkan mahasiswa ke dosen pembimbing (transaksional & kunci row mahasiswa). |
| `PATCH` | `/api/v1/supervisors/:id` | `supervision:manage` | Memperbarui kuota pembimbing dengan *optimistic locking*. |
| `POST` | `/api/v1/supervisions/switch` | `supervision:manage` | Memindahkan mahasiswa ke pembimbing baru (penguncian row terurut mencegah deadlock). |

### C. Modul 10 — Course Reviews (`/api/v1/courses/:id/reviews`, `/api/v1/reviews`)

| Method | Endpoint | Hak Akses | Deskripsi |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/courses/:id/reviews` | Authenticated | Menambahkan ulasan kelas baru di MongoDB (1 review per mahasiswa per kelas). |
| `GET` | `/api/v1/courses/:id/reviews` | Authenticated | Menampilkan daftar ulasan kelas dengan pagination (`page`, `limit`) & sorting. |
| `GET` | `/api/v1/courses/:id/reviews/summary` | Authenticated | Agregasi ringkasan ulasan (rata-rata rating, total ulasan, distribusi bintang 1–5). |
| `GET` | `/api/v1/reviews/:id` | Authenticated | Mengambil satu ulasan berdasarkan ID MongoDB. |
| `PATCH` | `/api/v1/reviews/:id` | Owner / Moderator | Memperbarui rating, komentar, atau tags ulasan. |
| `DELETE` | `/api/v1/reviews/:id` | Owner / Moderator | Menghapus ulasan kelas. |
| `POST` | `/api/v1/reviews/:id/replies` | Authenticated | Menambahkan balasan ulasan secara atomik (maks 50 balasan per ulasan). |
| `POST` | `/api/v1/reviews/:id/helpful` | Authenticated | Memberikan voting bantuan secara atomik dan deduplikasi pemilih (*voters*). |

### D. Modul 10 — Tugas Mandiri Student Portfolio (`/api/v1/students/:id/portfolio`, `/api/v1/portfolios`)

| Method | Endpoint | Hak Akses | Deskripsi |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/v1/students/:id/portfolio` | Authenticated | Mengambil data portofolio mahasiswa (validasi keberadaan mahasiswa di PostgreSQL). |
| `PUT` | `/api/v1/students/:id/portfolio` | Owner / Admin | Membuat atau memperbarui portofolio (upsert atomik di MongoDB). |
| `POST` | `/api/v1/students/:id/portfolio/projects` | Owner / Admin | Menambahkan proyek baru secara atomik dengan batas maksimal 20 proyek. |
| `DELETE` | `/api/v1/students/:id/portfolio/projects/:projectId` | Owner / Admin | Menghapus proyek tertentu dari portofolio menggunakan operator `$pull`. |
| `GET` | `/api/v1/portfolios/stats/technologies` | Authenticated | Statistik 10 teknologi paling populer dari seluruh portofolio via 2x `$unwind`. |

---

## 10. Contoh Request & Response Penting

### 1. Pendaftaran Kelas KRS (POST `/api/v1/courses/1/enroll`)
**Header:**
```http
Authorization: Bearer <token_student>
```
**Response Sukses (201 Created):**
```json
{
  "status": "success",
  "message": "pendaftaran berhasil",
  "data": {
    "id": 1,
    "code": "TI101",
    "name": "Pemrograman Lanjut",
    "credits": 3,
    "quota": 30,
    "enrolled_count": 15,
    "version": 1
  }
}
```

### 2. Pertukaran Kelas KRS (POST `/api/v1/enrollments/swap`)
**Request Body:**
```json
{
  "drop_course_id": 1,
  "enroll_course_id": 2
}
```
**Response Sukses (200 OK):**
```json
{
  "status": "success",
  "message": "pertukaran kelas berhasil",
  "data": {
    "dropped": { "id": 1, "code": "TI101" },
    "enrolled": { "id": 2, "code": "TI102" }
  }
}
```

### 3. Penambahan Proyek Portofolio (POST `/api/v1/students/1/portfolio/projects`)
**Request Body:**
```json
{
  "title": "Sistem KRS dan Portfolio",
  "description": "Microservice Go Fiber dengan PostgreSQL dan MongoDB v2",
  "technologies": ["Go", "Fiber", "PostgreSQL", "MongoDB"],
  "project_url": "https://github.com/example/krs-portfolio",
  "start_date": "2024-01-10",
  "end_date": "2024-06-20"
}
```
**Response Sukses (201 Created):**
```json
{
  "status": "success",
  "message": "proyek berhasil ditambahkan",
  "data": {
    "id": "67184a56c789abcdef123456",
    "title": "Sistem KRS dan Portfolio",
    "description": "Microservice Go Fiber dengan PostgreSQL dan MongoDB v2",
    "technologies": ["go", "fiber", "postgresql", "mongodb"],
    "project_url": "https://github.com/example/krs-portfolio",
    "start_date": "2024-01-10T00:00:00Z",
    "end_date": "2024-06-20T00:00:00Z"
  }
}
```

### 4. Statistik 10 Teknologi Portofolio (GET `/api/v1/portfolios/stats/technologies`)
**Response Sukses (200 OK):**
```json
{
  "status": "success",
  "message": "berhasil mengambil statistik teknologi",
  "data": [
    { "technology": "go", "count": 12 },
    { "technology": "postgresql", "count": 10 },
    { "technology": "mongodb", "count": 9 },
    { "technology": "docker", "count": 7 }
  ]
}
```

---

## 11. Skenario Transaksi dan Concurrency

1. **Batas Maksimal 24 SKS per Mahasiswa**:
   Pada saat `Enroll`, sistem mengunci row mahasiswa (`SELECT id FROM users WHERE id = $1 FOR UPDATE`) dan menjumlahkan SKS yang telah diambil (`SumCredits`). Jika `total + credits > 24`, transaksi di-rollback seketika dan mengembalikan HTTP 422 `CREDIT_LIMIT_EXCEEDED`.
2. **Kondisi Balapan Kuota Kursi Kelas**:
   Kapasitas kelas dikurangi/ditambah secara atomik di tingkat SQL menggunakan query kondisional:
   ```sql
   UPDATE courses SET enrolled_count = enrolled_count + 1 WHERE id = $1 AND enrolled_count < quota RETURNING ...
   ```
   Bila kuota telah penuh, tidak ada baris yang berubah dan sistem mengembalikan HTTP 409 `ErrCourseFull`.
3. **Pencegahan Deadlock pada Pergantian Pembimbing**:
   Saat melakukan switch dosen pembimbing, dua ID dosen (`old_id` dan `new_id`) selalu dikunci dalam urutan ID menaik (`ORDER BY id FOR UPDATE`) terlepas dari arah pertukaran (A ke B atau B ke A). Ini menjamin dua request switch silang tidak akan saling mengunci (deadlock).
4. **Batas Atomik 20 Proyek Portofolio**:
   Penambahan proyek menggunakan filter kondisional array 0-indexed MongoDB:
   ```json
   { "student_id": 1, "projects.19": { "$exists": false } }
   ```
   bersama operator `$push`. Jika dokumen sudah memiliki 20 proyek, filter tidak cocok sehingga operasi ditolak dengan HTTP 409 `PROJECT_LIMIT`.

---

## 12. Panduan Pengujian

### Menjalankan Seluruh Test Suite
Jalankan perintah pengujian standar Go:
```bash
go test -count=1 ./...
```

### Menjalankan Pengujian per Package
```bash
# Pengujian Unit & Concurrency Repository (PostgreSQL & MongoDB)
go test -v -count=1 ./app/repository

# Pengujian Service & HTTP Integration Test
go test -v -count=1 ./app/service

# Pengujian Helper, Database, dan Middleware
go test -v -count=1 ./helper
go test -v -count=1 ./database
go test -v -count=1 ./middleware
```

### Verifikasi Kualitas Kode
```bash
# Analisis statis Go vet
go vet ./...

# Validasi kompilasi seluruh paket
go build ./...
```

> **Catatan Race Detector (`-race`)**: Race detector Go membutuhkan compiler C (CGO). Pada sistem Windows tanpa instalasi gcc/MinGW, penambahan flag `-race` akan menghasilkan notifikasi `go: -race requires cgo`. Seluruh logika konkurensi (kuota kelas, dosen pembimbing, dan proyek portofolio) telah diuji secara fungsional melalui uji beban multi-goroutine serentak di dalam test suite.
