package review

import (
	"neuroplast.io/hotty-surf/surfrun"
	"neuroplast.io/hotty-surf/surftea"
)

// margin's hotty-surf seam: a hotty-surf model shown as one of margin's
// surfaces. margin places its surfaces itself, with hottytea's Session, so a
// surf model is hosted the way a multiplexer hosts its panes: by a
// surftea.Host, which routes the host's events and the keys to the model,
// writes what changed through the Session, and draws the model in cells where
// the terminal is no host. margin's side of it is the order in hotty.go:
// Surface for each one in the Layout's list, AfterLayout and Sync after, and
// Event, Key and Ready from Update.
//
// None of margin's models posts from a goroutine of its own, so the Hosts
// have no program to send to (surftea.New's nil); a Post before one is made
// does nothing.

// newSurfHost hosts m as the surface name. keys are margin's own keys, by
// myelin's names, which a surf text field gives back to margin. A surf
// binding to Quit never ends margin, and no pane has a menu, so a secondary
// press opens none that nothing would draw and that would hold the keys
// (Surface.NoMenu).
func newSurfHost(name string, m surfrun.Model, keys []string) *surftea.Host {
	h := surftea.New(name, m, nil, surftea.Options{Program: keys, NoQuit: true})
	h.Surf().NoMenu = true
	return h
}
