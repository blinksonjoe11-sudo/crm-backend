# CRM Backend — Go + Gin

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
