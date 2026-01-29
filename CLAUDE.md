# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

OpenAI-compatible LLM gateway that routes requests to multiple upstream providers (OpenAI, Azure OpenAI, etc.) with namespace-based model routing and API key authentication.

## Build & Run Commands

```bash
# Build
go build -o gateway ./cmd/gateway

# Run (requires CONFIG and optionally API_KEYS env vars)
CONFIG='{"default":[...]}' API_KEYS="key1,key2" ./gateway

# Docker build
docker build -t openai-gateway .

# Docker run
docker compose up -d
```

## Architecture

The Go implementation follows a standard layout:

- `cmd/gateway/main.go` - Entry point, HTTP server setup with chi router, graceful shutdown
- `internal/config/` - Configuration parsing, route resolution, upstream client management
- `internal/handler/` - HTTP handlers for proxy, models list, health check; handles both streaming and non-streaming responses
- `internal/middleware/` - Bearer token authentication middleware
- `internal/logger/` - JSON structured logging

### Request Flow

1. Auth middleware validates Bearer token against `API_KEYS`
2. Handler extracts model name from request body (format: `namespace/model` or just `model` for default namespace)
3. Config resolves model to `RouteEntry` containing upstream client and actual model name
4. Handler proxies request to upstream, handling SSE streaming if `stream: true`

### Configuration Structure

Config is a JSON map of namespaces to client configs:
- `type`: "openai", "azure", or "alias"
- For openai/azure: `models`, `api_key`, `base_url`/`azure_endpoint`
- For alias: maps alias names to existing `namespace/model` keys

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

- Scope is required (e.g., `sidebar`, `tasks`, `auth`)
- Description in sentence case with capital first letter
- Use present tense action verbs (Add, Fix, Support, Update, Replace, Optimize)
- No period at the end
- Keep it concise and focused

**Examples**:

```
feat(apple): Support apple signin
fix(sidebar): Change the abnormal scrolling
chore(children): Optimize children api
refactor(tasks): Add timeout status
```

**Do NOT include**:

- "Generated with Claude Code" or similar attribution
- "Co-Authored-By: Claude" or any Claude co-author tags
