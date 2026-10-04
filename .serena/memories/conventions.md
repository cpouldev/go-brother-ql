# Editing constraints

- Follow AGENTS.md: clarify unclear intent, prefer simplest adequate design, surface unrelated issues separately. Semantic navigation/editing uses Serena when available; docs use Context7.
- Keep root conversion package transport-independent. Hardware access belongs in printer; CLI uses the same public library and spools before opening devices.
- Go files use GPL-3.0-or-later SPDX headers. Preserve upstream credits and fixture provenance.
- No hardware access from ordinary tests. Printer integration uses scripted transports and local TCP. Hardware verification is explicitly recorded separately.
- Follow `mem:memory_maintenance` for memory edits and references.
