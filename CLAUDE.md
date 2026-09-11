# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based cryptocurrency exchange implementation. The project is in early development.

## Build and Run Commands

```bash
# Build the project
make build

# Build and run
make run

# Run tests
go test ./...

# Run a specific test
go test -run TestName ./internal/orderbook/

# Run tests with verbose output
go test -v ./...
```

## Project Structure

- `main.go` - Application entry point
- `internal/orderbook/` - Order book implementation (matching engine core)
- `config/` - Configuration files (gitignored)

## Architecture

The exchange uses Go's standard `internal/` package layout. The order book in `internal/orderbook/` is the core matching engine component that will handle bid/ask matching.

Module name: `my-go-exchange`
Go version: 1.26

## Agent skills

### Issue tracker

Local markdown under `.scratch/<feature>/`. See `docs/agents/issue-tracker.md`.

### Domain docs

Single-context — `CONTEXT.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.
