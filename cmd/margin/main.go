// Command margin is a terminal tool for reviewing markdown: read the rendered
// document, leave anchored comments, mark blocks reviewed, and hand the result
// back to whatever wrote it.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/neuroplastio/margin/internal/review"
)

func usage() {
	fmt.Fprint(os.Stderr, `margin — review markdown in the terminal

usage:
  margin FILE.md

keys:
  j/k   move          c  comment        r  mark reviewed
  g/G   top/bottom    e  edit           f  flag for later
  q     quit

In the composer every key belongs to nvim:
  ctrl+s submit · esc esc keep a draft · :q! discard
`)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 1 {
		usage()
		os.Exit(2)
	}

	if err := review.Run(flag.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, "margin:", err)
		os.Exit(1)
	}
}
