# Comment composer text wrapping & cursor misalignment issue

When typing long comments in the comment composer window, text wrapping gets misaligned. Words wrap onto new lines unexpectedly (leaving single short words like "a" orphaned on a line despite plenty of horizontal space), and the cursor ends up positioned incorrectly relative to the text being typed.

In addition, the screenshots highlight two UX feedback notes:
1. When adding a comment into an existing thread, show existing comments so the author doesn't feel like they are writing a parallel thread.
2. `j`/`k` navigation eats focus on threads; consider a dedicated "dive" navigation mechanism for diving into threads and multi-line blocks.

Reference screenshots:
- `/tmp/herdr-clipboard-images-1000/client-12-clipboard-1786262478049154881-0.png`
- `/tmp/herdr-clipboard-images-1000/client-12-clipboard-1786262524317546353-0.png`
