# Changelog

All notable changes to Starcat CLI are documented here.

The project follows Semantic Versioning. GitHub Releases are the source of published binaries and release notes.

## v1.0.0 - 2026-07-20

- Added verified cross-platform self-update support.
- Added daily interactive update notifications with an opt-out.
- Added macOS/Linux and Windows one-line installers.
- Added staged installer progress, PATH guidance, pairing steps, and common command hints.
- Added GitHub Actions CI, release archives, checksums, and build provenance attestations.
- Changed pairing so users can paste a complete pairing command or press Enter after entering a one-time URI.
- Changed human-facing commands to terminal-friendly text while keeping data commands machine-readable JSON.
- Kept `doctor --json` only for callers that explicitly need structured diagnostics.
- Rejected unknown CLI flags instead of silently accepting them.
- Standardized all command-line output in English.
