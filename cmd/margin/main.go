// Command margin is a terminal tool for reviewing markdown: read the rendered
// document, leave anchored comments, mark blocks reviewed, and hand the result
// back to whatever wrote it.
package main

import (
	"fmt"
	"os"

	"github.com/AnatolyRugalev/margin/internal/review"
)

func main() {
	if err := review.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "margin:", err)
		os.Exit(1)
	}
}
