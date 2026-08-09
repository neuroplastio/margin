# Visual Mode Range Selection & Line Reference Prepending — 2026-08-09

- **Visual Line Selection (`Shift+V`):** Support a visual line mode triggered by `Shift+V` to select multiple blocks/lines.
- **Line/Range Prepending in Comments:** Threads remain line-based, but when commenting with an active selection:
  - If a multi-line range is selected, automatically prepend the line range (e.g. `L12-18: `) into the body of the comment.
  - If a single line is selected, prepend the single line number (e.g. `L12: `).
- **Flexible Comment Placement:** Comments containing prepended line/range references can be placed anywhere in the document, allowing users to reference other parts of the document within their review comments.
