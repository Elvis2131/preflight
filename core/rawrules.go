package core

// rawRuleMaps reads an SG/NACL rule list out of Node.RawAttributes in either of its two
// real shapes: []map[string]any while the IR is still the in-memory value ingest just
// built, and []any of map[string]any once it has been through the session store's JSON
// round trip (every stored version is read back that way).
//
// Every reader of these lists MUST go through here. A bare type assertion to
// []map[string]any silently reads a stored list as EMPTY — which turned every stored
// security group into "attached, zero rules" (PC-137) and made sg_rule_change /
// nacl_rule_change faults against a stored version find no rule to remove (found while
// building the canvas route/NACL editors, PC-138/139). Absent or any other type: nil.
func rawRuleMaps(v any) []map[string]any {
	switch rules := v.(type) {
	case []map[string]any:
		return rules
	case []any:
		out := make([]map[string]any, 0, len(rules))
		for _, r := range rules {
			if m, ok := r.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// hasRawRules reports whether the key is present as a rule list of either shape, even
// an empty one — "present but empty" and "absent" mean different things to callers.
func hasRawRules(v any) bool {
	switch v.(type) {
	case []map[string]any, []any:
		return true
	}
	return false
}
