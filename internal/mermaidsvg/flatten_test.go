package mermaidsvg

import "testing"

func TestFlatten(t *testing.T) {
	in := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10" width="10" height="10" ` +
		`style="--_bg:var(--ondine-bg,#fff);--_fg:var(--ondine-fg,#000);--_x:var(--ondine-unset,#123456);` +
		`--_a:var(--ondine-surface-alt,color-mix(in srgb,var(--_fg) 20%,var(--_bg)));` +
		`stroke-width:var(--_sw,2px);color:var(--_fg);font:16px monospace;font-family:var(--ondine-font,Menlo, monospace)" ` +
		`fill="var(--_fg)"><rect fill="var(--_x)" stroke="color-mix(in srgb,var(--_a) 50%,var(--_bg))"/></svg>`
	want := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10" width="10" height="10" ` +
		`style="stroke-width:2px;color:#d0d0d0;font:16px monospace;font-family:Menlo, monospace" ` +
		`fill="#d0d0d0"><rect fill="#123456" stroke="#262626"/></svg>`
	if got := string(flatten([]byte(in))); got != want {
		t.Errorf("flatten:\n got %s\nwant %s", got, want)
	}
}

func TestFlattenLeavesOtherDocuments(t *testing.T) {
	for _, in := range []string{`<svg viewBox="0 0 1 1"/>`, `not an svg var(--x)`} {
		if got := string(flatten([]byte(in))); got != in {
			t.Errorf("flatten(%q) = %q, want it unchanged", in, got)
		}
	}
}

func TestMix(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"color-mix(in srgb,#ffffff 20%,#000000)", "#333333"},
		{"color-mix(in srgb,#fff 50%,#000)", "#808080"},
		{"color-mix(in srgb,#ff0000,#0000ff 25%)", "#bf0040"},
		{"color-mix(in srgb,#ff0000,#0000ff)", "#800080"},
		{"color-mix(in srgb,#ff0000 30%,#0000ff 30%)", "#800080"},                    // normalised
		{"color-mix(in srgb,color-mix(in srgb,#fff 50%,#000) 100%,#000)", "#808080"}, // nested
		{"color-mix(in oklab,#ff0000 50%,#0000ff)", "#ff0000"},                       // not srgb: the first colour
		{"color-mix(in srgb,red 50%,#0000ff)", "red"},                                // not hex: the first colour
		{`a="color-mix(in srgb,#000 87.5%,#fff)" b="#1"`, `a="#202020" b="#1"`},
	} {
		if got := mix(tc.in); got != tc.want {
			t.Errorf("mix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveStopsOnACycle(t *testing.T) {
	declared := map[string]string{"--a": "var(--b)", "--b": "var(--a)"}
	if got := resolve("x:var(--a)", declared, 0); len(got) > 64 {
		t.Errorf("resolve ran away: %d bytes", len(got))
	}
}
