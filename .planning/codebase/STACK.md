# Technology Stack

**Analysis Date:** 2026-03-29

## Languages

**Primary:**
- Go 1.26.1 - All source code and tests

## Runtime

**Environment:**
- Go runtime 1.26.1

**Package Manager:**
- Go modules (built-in)
- Lockfile: `go.sum` not present (no external dependencies — stdlib only)

## Frameworks

**Core:**
- None — pure Go stdlib only

**Testing:**
- Go standard library `testing` package — unit tests via `go test`
- No third-party test framework (no testify, gomock, etc.)

**Build/Dev:**
- `go build` / `go test` — standard Go toolchain

## Key Dependencies

**Critical:**
- None — zero external dependencies
- `go.mod` declares `module community-detection` with `go 1.26.1` and no `require` block

**Infrastructure:**
- None

## Configuration

**Environment:**
- No environment variables required
- No `.env` files present

**Build:**
- `go.mod` at repo root: `community-detection` module name

## Platform Requirements

**Development:**
- Go 1.26.1+
- No other tooling required

**Production:**
- Compiled Go binary (cross-platform)
- No runtime dependencies

---

*Stack analysis: 2026-03-29*
