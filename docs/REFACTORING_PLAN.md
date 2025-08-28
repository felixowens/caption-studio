# Caption Studio Backend Refactoring Plan

## Current Problem

- `main.go` is 2000+ lines handling everything
- HTTP handlers, business logic, database, file I/O all mixed together
- Hard to test, hard to maintain, violates guidelines

## Strategy: Vertical Slices

Extract complete features end-to-end instead of horizontal layers.

## Slice #1: Auto Captioning

### Current Files

- `main.go`: 7 HTTP handlers (~300 lines)
- `auto_caption.go`: Business logic (good)
- `captioning.go`: Service layer (good)

### Target Structure

```bash
internal/
├── handler/auto_caption.go     # HTTP only
├── service/auto_caption.go     # Business logic
└── captioning/
    ├── service.go
    ├── gemini.go
    └── types.go
```

### Steps

1. Create internal package structure
2. Move HTTP handlers from main.go
3. Move auto_caption.go to internal/service/
4. Move captioning.go to internal/captioning/
5. Wire together, test endpoints
