package review

import (
	oldhotty "github.com/neuroplastio/hotty-go"

	"neuroplast.io/hotty-go"
)

// oldEvent is a host's event as hotty-a2ui's html rendition takes it.
// hotty-a2ui is archived and imports hotty-go at its old path
// (github.com/neuroplastio/hotty-go, pinned at 23af564 in go.mod), so its
// Event is another type from the one the host's SDK now gives. The types
// have the same fields besides Button and At (hotty 0.7's secondary press),
// which a pane of the kit has no use for.
//
// Not a bridge between two ends of a wire (gov P-3): both are this
// binary, and the file goes with the last pane that is on hotty-a2ui
// (vault/knowledge/hotty-surf-port.md, leg 10).
func oldEvent(ev hotty.Event) oldhotty.Event {
	o := oldhotty.Event{
		Surface: ev.Surface, Kind: ev.Kind, Target: ev.Target, Value: ev.Value,
		Checked: ev.Checked, Rev: ev.Rev, Sel: ev.Sel, Mods: ev.Mods,
		FitRows: ev.FitRows, Count: ev.Count, Detail: ev.Detail,
	}
	for _, f := range ev.Fields {
		o.Fields = append(o.Fields, oldhotty.FormField(f))
	}
	if ev.Link != nil {
		l := oldhotty.Link(*ev.Link)
		o.Link = &l
	}
	if ev.Drag != nil {
		d := oldhotty.Drag(*ev.Drag)
		o.Drag = &d
	}
	if ev.Hover != nil {
		h := oldhotty.Hover(*ev.Hover)
		o.Hover = &h
	}
	if ev.View != nil {
		v := oldhotty.View{AtEnd: ev.View.AtEnd}
		if ev.View.Areas != nil {
			v.Areas = make(map[string]oldhotty.Area, len(ev.View.Areas))
			for k, a := range ev.View.Areas {
				v.Areas[k] = oldhotty.Area(a)
			}
		}
		o.View = &v
	}
	if ev.Area != nil {
		a := oldhotty.Area(*ev.Area)
		o.Area = &a
	}
	if ev.Command != nil {
		c := oldhotty.Command(*ev.Command)
		o.Command = &c
	}
	if ev.Changed != nil {
		c := ev.Changed
		o.Changed = &oldhotty.Caps{
			V: c.V, CellCSS: oldhotty.Size(c.CellCSS), Scale: c.Scale, Scheme: c.Scheme,
			Mod: c.Mod, Features: c.Features, Limits: c.Limits, Net: c.Net, Edit: c.Edit,
			Host: c.Host, Version: c.Version, Raw: c.Raw,
		}
	}
	return o
}
