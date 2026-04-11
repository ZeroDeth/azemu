# Changelog

All notable changes to azemu will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Project scaffold: dual HTTP/HTTPS server, chi routing, zerolog logging
- Metadata service (`/metadata/endpoints`) for azurerm provider redirection
- Mock OAuth2 token endpoint with RS256 JWT signing
- OIDC discovery and JWKS endpoints
- ARM facade: subscriptions, provider registration, resource group CRUD
- Azure-compatible middleware: response headers, api-version enforcement
- In-memory state store with export/import
- Self-signed TLS certificate generation (ECDSA P-256)
- Dockerfile (multi-stage Go build)
- Makefile with build, run, test, docker, smoke targets
- Example Terraform config (`test/terraform/main.tf`)
- CLAUDE.md, AGENTS.md, TASKS.md for AI agent orchestration
- docs/PARITY.md resource compatibility matrix
