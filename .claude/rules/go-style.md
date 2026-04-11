---
paths:
  - "**/*.go"
---

# Go style rules

Loaded only when Claude is reading or editing Go files.

## Error handling

Wrap with context using `%w`. Bare returns lose caller context; swallowed
errors hide bugs.

```go
// CORRECT
return fmt.Errorf("put resource group %q: %w", name, err)

// WRONG: bare return
return err

// WRONG: swallowed
log.Error().Err(err).Msg("failed")
// continues execution
```

## Logging

Use structured zerolog fields, never printf-style.

```go
// CORRECT
log.Info().Str("resource_id", id).Str("method", r.Method).Msg("resource created")

// WRONG
log.Info().Msgf("created resource %s via %s", id, r.Method)
```

## Dependencies

Don't add new dependencies without explicit approval. The pinned set in
`go.mod` is intentionally minimal: `chi/v5`, `jwt/v5`, `uuid`, `zerolog`. No
cobra, viper, urfave/cli, testify, gomock, or any other framework.

## Package boundaries

`cmd/azemu` may import any `internal/*` and `pkg/*`. Within `internal/`:

- `arm/` may import `store/` and `pkg/armtypes/`
- `metadata/` may import `pkg/config/`
- `middleware/` and `auth/` are standalone (no internal imports)
- No cross-imports between sibling internal packages

If you need a shared type across `internal/*` packages, put it in `pkg/`.

## Naming

| Item | Convention | Example |
|------|------------|---------|
| Go files | lowercase, underscore separated | `resource_group.go` |
| Test files | `*_test.go` alongside source | `resource_group_test.go` |
| ARM resource handlers | one file per resource type | `arm/vnet.go`, `arm/subnet.go` |
| Test helpers | `testutil_test.go` | `arm/testutil_test.go` |
