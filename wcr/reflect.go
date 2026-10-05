package wcr

import (
	"fmt"
	"strconv"
)

// reflect.go implements the attribute <-> property linkage:
//
//   - Element.Set assigns a property and reflects serializable values to
//     attributes (bool -> presence, string/number -> value).
//   - Element.SetAttribute rewrites an attribute directly; observed
//     attributes fire AttributeChanged and update the property mirror.
//   - All mutations within one turn are coalesced: the element is marked
//     dirty once and re-rendered a single time on Document.Flush, so no
//     intermediate state is ever observable.

// Set assigns property name and reflects it to an attribute when the value
// is of a serializable kind. Reflection is re-entrancy guarded.
func (e *Element) Set(name string, value any) {
	if e.props == nil {
		e.props = map[string]any{}
	}
	e.props[name] = value
	e.reflecting = true
	switch v := value.(type) {
	case bool:
		if v {
			e.setAttrRaw(name, "")
		} else {
			e.removeAttrRaw(name)
		}
	case string:
		e.setAttrRaw(name, v)
	case int:
		e.setAttrRaw(name, strconv.Itoa(v))
	case int64:
		e.setAttrRaw(name, strconv.FormatInt(v, 10))
	case float64:
		e.setAttrRaw(name, strconv.FormatFloat(v, 'g', -1, 64))
	case fmt.Stringer:
		e.setAttrRaw(name, v.String())
	}
	e.reflecting = false
	e.markDirty()
}

// SetAttribute rewrites attribute name directly. Observed attributes fire
// the AttributeChanged callback and update the property mirror.
func (e *Element) SetAttribute(name, value string) {
	old, had := e.Node.GetAttr(name)
	if had && old == value {
		return
	}
	e.Node.SetAttr(name, value)
	e.onAttrChanged(name, old, value, had)
}

// RemoveAttribute removes the attribute, firing callbacks when observed.
func (e *Element) RemoveAttribute(name string) {
	old, had := e.Node.GetAttr(name)
	if !had {
		return
	}
	e.Node.RemoveAttr(name)
	e.onAttrChanged(name, old, "", true)
}

func (e *Element) onAttrChanged(name, oldVal, newVal string, hadOld bool) {
	if e.Def == nil {
		return
	}
	if !e.observes(name) {
		return
	}
	// keep property mirror consistent with direct attribute rewrites
	if !e.reflecting {
		if e.props == nil {
			e.props = map[string]any{}
		}
		e.props[name] = newVal
	}
	if e.AttributeChanged != nil && e.connected {
		e.AttributeChanged(e, name, oldVal, newVal)
	}
	e.markDirty()
}

func (e *Element) observes(name string) bool {
	for _, a := range e.Def.ObservedAttributes {
		if a == name {
			return true
		}
	}
	return false
}

// markDirty coalesces all changes in the current turn into one re-render.
func (e *Element) markDirty() {
	if e.Doc != nil {
		e.Doc.schedule(e)
	}
}

// raw attribute helpers: no callbacks, used by reflection.
func (e *Element) setAttrRaw(name, value string) { e.Node.SetAttr(name, value) }
func (e *Element) removeAttrRaw(name string)     { e.Node.RemoveAttr(name) }

// --- dirty queue on Document ---

// schedule marks el for a single coalesced re-render at Flush time.
func (d *Document) schedule(el *Element) {
	if el.dirty {
		return // coalesced: already pending this turn
	}
	el.dirty = true
	d.pending = append(d.pending, el)
}

// Flush re-renders every dirty element exactly once, in schedule order.
// Callbacks triggered by a re-render may schedule further work; Flush loops
// until the queue drains, but each element renders at most once per pass.
func (d *Document) Flush() {
	for len(d.pending) > 0 {
		batch := d.pending
		d.pending = nil
		for _, el := range batch {
			el.dirty = false
			if el.failed || el.Def == nil || el.Def.Render == nil {
				continue
			}
			el.reRender()
		}
	}
}
