package ingest

import "github.com/zclconf/go-cty/cty"

// ctyToGo converts a wholly-known cty.Value into a plain Go value suitable for
// core.Node.RawAttributes (map[string]any). Only the scalar and simple collection
// types golden/aws's own attributes actually use are handled; anything else (cty
// capsule types, unknown/null values reaching here despite the caller's
// IsWhollyKnown() check) is reported as not convertible rather than guessed at.
func ctyToGo(v cty.Value) (any, bool) {
	if v.IsNull() {
		return nil, true
	}
	t := v.Type()
	switch {
	case t == cty.String:
		return v.AsString(), true
	case t == cty.Bool:
		return v.True(), true
	case t == cty.Number:
		f, _ := v.AsBigFloat().Float64()
		return f, true
	case t.IsListType(), t.IsTupleType(), t.IsSetType():
		var out []any
		for it := v.ElementIterator(); it.Next(); {
			_, ev := it.Element()
			gv, ok := ctyToGo(ev)
			if !ok {
				return nil, false
			}
			out = append(out, gv)
		}
		return out, true
	case t.IsMapType(), t.IsObjectType():
		out := map[string]any{}
		for it := v.ElementIterator(); it.Next(); {
			kv, ev := it.Element()
			gv, ok := ctyToGo(ev)
			if !ok {
				return nil, false
			}
			out[kv.AsString()] = gv
		}
		return out, true
	default:
		return nil, false
	}
}
