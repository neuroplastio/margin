# Visual Mode Range Selection & Line Reference Prepending — 2026-08-09

- **Visual Line Selection (`Shift+V`):** Support a visual line mode triggered by `Shift+V` to select multiple blocks/lines.
- **Line/Range Prepending in Comments:** Threads remain line-based, but when commenting with an active selection:
  - If a multi-line range is selected, automatically prepend the line range (e.g. `L12-18: `) into the body of the comment.
  - If a single line is selected, prepend the single line number (e.g. `L12: `).
- **Flexible Comment Placement:** Comments containing prepended line/range references can be placed anywhere in the document, allowing users to reference other parts of the document within their review comments.
- **Yank Content (`y`):** Selecting a block/range and pressing `y` should copy the selected text contents into the system clipboard.
- **Yank Reference (`gy` or `yr`):** Provide a distinct keybinding (e.g. `gy` for "goto-yank reference" or `yr` for "yank reference") to copy the line number / range reference (e.g. `L12-18` or `file.md:L12-18`) to the clipboard instead of the raw text body.
