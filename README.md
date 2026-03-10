# CRM Backend — Go + Gin + Cloudinary

Production-grade CRM REST API built with **Go 1.22**, **Gin**, **GORM**, **PostgreSQL**, **Redis**, and **Cloudinary** for all file storage.

---

## Stack

| Layer | Tech |
|---|---|
| HTTP | Gin v1.9 |
| Database | PostgreSQL 15 + GORM v2 |
| Cache / Sessions | Redis 7 |
| File Storage | **Cloudinary** |
| Auth | JWT (access + refresh) + httpOnly cookies |
| Email | SMTP / SendGrid via gomail |
| Async Jobs | Asynq (Redis-backed) |
| Logging | Zap (structured JSON) |
| Config | Viper + `.env` |

---

## Quick Start

```bash
# 1. Clone and enter
git clone https://github.com/yourorg/crm && cd crm

# 2. Copy env
make env          # copies .env.example → .env
# then edit .env and fill in secrets

# 3. Start infrastructure
make docker-up    # starts postgres + redis

# 4. Run the server
make run          # http://localhost:8080
```

---

## Cloudinary Setup

1. Create a free account at [cloudinary.com](https://cloudinary.com)
2. From the **Dashboard**, copy your Cloud Name, API Key, and API Secret
3. Set them in `.env`:

```
CLOUDINARY_CLOUD_NAME=your_cloud_name
CLOUDINARY_API_KEY=your_api_key
CLOUDINARY_API_SECRET=your_api_secret
```

### How uploads are organised

All assets are stored under `crm/{workspace_id}/{type}/`:

| Endpoint | Cloudinary Folder | Transformation |
|---|---|---|
| `POST /contacts/:id/avatar` | `crm/{wsid}/avatars/` | 200×200 face crop |
| `POST /companies/:id/logo` | `crm/{wsid}/logos/` | 300×300 padded |
| `POST /uploads/attachment` | `crm/{wsid}/attachments/` | original |
| `POST /workspace/logo` | `crm/{wsid}/workspace/` | 400×400 padded |

Images get an **eager thumbnail** generated on upload. Raw files (PDFs, docs) are stored as-is.

---

## API Overview

All endpoints are prefixed `/api/v1`. Auth uses `Authorization: Bearer <token>`.

### Auth (public)
```
POST  /auth/register
POST  /auth/login
POST  /auth/refresh       (uses httpOnly cookie)
POST  /auth/forgot-password
POST  /auth/reset-password
```

### Auth (protected)
```
GET   /auth/me
POST  /auth/logout
POST  /auth/change-password
```

### Contacts
```
GET    /contacts
POST   /contacts
GET    /contacts/:id
PATCH  /contacts/:id
DELETE /contacts/:id
POST   /contacts/:id/avatar    ← Cloudinary upload
```

### Deals
```
GET    /deals
POST   /deals
GET    /deals/:id
PATCH  /deals/:id
DELETE /deals/:id
PATCH  /deals/:id/stage
POST   /deals/:id/won
POST   /deals/:id/lost
POST   /deals/:id/contacts/:contact_id
DELETE /deals/:id/contacts/:contact_id
```

### Activities, Tasks
```
GET/POST      /activities
PUT/DELETE    /activities/:id

GET/POST      /tasks
PUT           /tasks/:id
POST          /tasks/:id/complete
DELETE        /tasks/:id
```

### File Uploads
```
POST  /uploads/attachment?entity_id=<uuid>    ← any file → Cloudinary
POST  /workspace/logo                          ← workspace logo → Cloudinary
```

---

## Response Format

```json
{ "success": true, "data": { ... }, "meta": { "page": 1, "per_page": 25, "total": 142 } }
{ "success": false, "error": { "code": "VALIDATION_ERROR", "message": "..." } }
```

---

## Project Structure

```
crm/
├── cmd/server/main.go          # Entry point
├── internal/
│   ├── config/                 # Viper config loader
│   ├── domain/models.go        # All GORM domain models
│   ├── repository/             # DB query layer (interfaces + implementations)
│   ├── service/                # Business logic
│   │   ├── service.go          # Auth, Contact, Deal, Activity, Task services
│   │   └── upload.go           # Cloudinary upload service
│   ├── handler/handler.go      # Gin HTTP handlers
│   ├── middleware/middleware.go # JWT, RBAC, rate limit, CORS, logging
│   ├── router/router.go        # Route registration
│   └── platform/platform.go   # DB, Redis, Logger init
├── pkg/
│   ├── cloudinary/             # Cloudinary SDK wrapper
│   ├── jwtutil/                # JWT generate/parse
│   └── mailer/                 # Email sending + templates
├── Dockerfile
├── docker-compose.yml
├── Makefile
└── .env.example
```

---

## RBAC Roles

| Role | Level |
|---|---|
| Owner | Full access including billing |
| Admin | Full access except billing |
| Manager | Team data + delete records |
| Sales Rep | Own records only |
| Read-Only | View only |

---

## Security

- Passwords hashed with **bcrypt cost 12**
- Refresh tokens stored in **httpOnly, Secure, SameSite** cookies
- **Rotating refresh tokens** — old token invalidated on every refresh
- Rate limiting: 300 requests/minute per IP via Redis
- All uploads size-capped at **25 MB** before hitting Cloudinary
- Security headers: `X-Frame-Options`, `X-XSS-Protection`, `CSP`
