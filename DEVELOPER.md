# CLAUDE.md

This file provides guidance to developers when working with code in this repository.

## Build and Run Commands

```bash
# Build the gateway
go build -o gateway ./cmd/gateway

# Run the gateway
./gateway
# Or directly:
go run ./cmd/gateway

# Run tests
go test ./...

# Run a single test
go test ./internal/sse -run TestSSEServer -v

# Build Docker image
docker build -t openai-gateway .

# Run with Docker Compose
docker compose up -d
```

## Architecture

OpenAI-compatible LLM gateway that routes requests to multiple upstream providers (OpenAI, Azure, etc.) based on model namespace configuration.

### Request Flow
1. Client sends OpenAI-compatible request with `Authorization: Bearer <key>`
2. Auth middleware validates API key against `API_KEYS` env var
3. Handler extracts model name (e.g., `azure/gpt-4o` or `gpt-4o`)
4. Config routes to appropriate upstream client based on namespace/model mapping
5. Request is proxied to upstream with provider-specific auth (Bearer token for OpenAI, `api-key` header for Azure)
6. Response is streamed back (SSE for streaming, JSON for non-streaming)

### Key Components
- `internal/config/` - Configuration parsing, model routing, upstream client management
- `internal/handler/` - HTTP handlers for proxy, health, and models endpoints
- `internal/middleware/` - Auth middleware for API key validation
- `internal/logger/` - JSON structured logging

### Model Routing
Models are addressed as `namespace/model` where:
- `default` namespace can be omitted (e.g., `gpt-4o` = `default/gpt-4o`)
- `alias` type allows mapping custom names to existing routes

### Environment Variables
- `CONFIG` - JSON config defining namespaces, providers, and models
- `API_KEYS` - Comma-separated list of valid API keys
- `PORT` - Server port (default: 8000)
- `HOST` - Server host (default: 0.0.0.0)

## Git Commit Guidelines

**Format**: `type(scope): Description`

**Types**:

- `feat` - New features
- `fix` - Bug fixes
- `docs` - Documentation changes
- `style` - Styling changes
- `refactor` - Code refactoring
- `perf` - Performance improvements
- `test` - Test additions or changes
- `chore` - Maintenance tasks
- `revert` - Revert previous commits
- `build` - Build system changes

**Rules**:

- Scope is required (e.g., `auth`, `resources`, `user`)
- Description in sentence case with capital first letter
- Use present tense action verbs (Add, Fix, Support, Update, Replace, Optimize)
- No period at the end
- Keep it concise and focused

**Examples**:

```
feat(auth): Support Apple signin
fix(resources): Fix tree ordering on drag-drop
chore(migrations): Add index for namespace lookup
refactor(tasks): Add timeout status handling
```

**Do NOT include**:

- "Generated with Claude Code" or similar attribution
- "Co-Authored-By: Claude" or any Claude co-author tags

