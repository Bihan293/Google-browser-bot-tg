# Changelog

All notable changes to this project will be documented in this file.

## [0.1.0] - 2026-05-11

### Added
- Initial release of `tg-browser-bot`.
- Telegram webhook server with health-check (`/healthz`).
- Search providers (no API keys required):
  - Web search via DuckDuckGo HTML endpoint.
  - Image search via DuckDuckGo `i.js` endpoint (with `vqd` token).
  - Video search via YouTube `ytInitialData` parsing.
  - News search via Google News RSS.
- In-chat page renderer (extracts readable text + `og:image`).
- Inline keyboards for pagination, mode switching and result opening.
- Reply keyboard with the main menu.
- In-memory session store with 30 minute TTL (no database required).
- Multi-stage Dockerfile and `.env.example`.
- README with architecture overview and usage instructions.
