# Stdin Support for Ephemeral Markdown Reviews — 2026-08-09

- **Stdin Input Support:** Support piping markdown content via stdin (or passing `-` / `--stdin`) to review ephemeral markdown without needing a saved file on disk.
- **Implied Stdout & No Persistence:** Using `--stdin` implies `--stdout` on exit—results/annotations are not stored in `.margin/threads`, but are printed directly to stdout upon exiting the review session.
