package compare

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/medom/terraform-drift-detector/internal/model"
)

var globalIgnore = map[string]bool{
	"id":                           true,
	"arn":                          true,
	"timeouts":                     true,
	"tags":                         true,
	"tags_all":                     true,
	"resource_tags":                true,
	"primary_network_interface_id": true,
}

type Options struct {
	Unmanaged bool
}

func Diff(expected, actual []model.Resource, opt Options) []model.Drift {
	expIdx := index(expected)
	actIdx := index(actual)
	var drifts []model.Drift

	for key, exp := range expIdx {
		act, ok := actIdx[key]
		if !ok {
			drifts = append(drifts, model.Drift{
				Kind:    model.KindDeleted,
				Address: exp.Address,
				Type:    exp.Type,
				ID:      exp.ID,
			})
			continue
		}
		drifts = append(drifts, tagDiffs(exp, act)...)
		drifts = append(drifts, attrDiffs(exp, act)...)
	}

	if opt.Unmanaged {
		for key, act := range actIdx {
			if _, ok := expIdx[key]; ok {
				continue
			}
			addr := act.Address
			if addr == "" {
				addr = act.Type + "/" + act.ID
			}
			drifts = append(drifts, model.Drift{
				Kind:    model.KindCreated,
				Address: addr,
				Type:    act.Type,
				ID:      act.ID,
			})
		}
	}

	sort.Slice(drifts, func(i, j int) bool {
		if drifts[i].Address == drifts[j].Address {
			if drifts[i].Kind == drifts[j].Kind {
				return drifts[i].Path < drifts[j].Path
			}
			return drifts[i].Kind < drifts[j].Kind
		}
		return drifts[i].Address < drifts[j].Address
	})
	return drifts
}

func index(rs []model.Resource) map[string]model.Resource {
	m := make(map[string]model.Resource, len(rs))
	for _, r := range rs {
		m[r.Key()] = r
	}
	return m
}

func tagDiffs(exp, act model.Resource) []model.Drift {
	keys := map[string]struct{}{}
	for k := range exp.Tags {
		keys[k] = struct{}{}
	}
	for k := range act.Tags {
		keys[k] = struct{}{}
	}
	var out []model.Drift
	for k := range keys {
		ev, eok := exp.Tags[k]
		av, aok := act.Tags[k]
		if eok && aok && ev == av {
			continue
		}
		var before, after any
		if eok {
			before = ev
		}
		if aok {
			after = av
		}
		out = append(out, model.Drift{
			Kind:    model.KindTagChanged,
			Address: exp.Address,
			Type:    exp.Type,
			ID:      exp.ID,
			Path:    "tags." + k,
			Before:  before,
			After:   after,
		})
	}
	return out
}

func attrDiffs(exp, act model.Resource) []model.Drift {
	if hash(exp.Attributes) == hash(act.Attributes) {
		return nil
	}
	paths := map[string]struct{}{}
	collectPaths(act.Attributes, "", paths)

	// Also include expected-side paths, but only for top-level keys
	// that the live fetcher actually returned (to avoid false positives
	// from computed state-only attributes the fetcher doesn't track).
	actTopKeys := map[string]bool{}
	for k := range act.Attributes {
		actTopKeys[k] = true
	}
	expPaths := map[string]struct{}{}
	collectPaths(exp.Attributes, "", expPaths)
	for p := range expPaths {
		root := strings.SplitN(p, ".", 2)[0]
		root = strings.SplitN(root, "[", 2)[0]
		if actTopKeys[root] {
			paths[p] = struct{}{}
		}
	}

	var out []model.Drift
	for p := range paths {
		root := strings.SplitN(p, ".", 2)[0]
		root = strings.SplitN(root, "[", 2)[0]
		if globalIgnore[root] {
			continue
		}
		ev := valueAt(exp.Attributes, p)
		av := valueAt(act.Attributes, p)
		if equalish(ev, av) {
			continue
		}
		out = append(out, model.Drift{
			Kind:    model.KindModified,
			Address: exp.Address,
			Type:    exp.Type,
			ID:      exp.ID,
			Path:    p,
			Before:  ev,
			After:   av,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func collectPaths(v any, prefix string, paths map[string]struct{}) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			collectPaths(child, p, paths)
		}
	case []any:
		if prefix != "" {
			paths[prefix] = struct{}{}
		}
	default:
		if prefix != "" {
			paths[prefix] = struct{}{}
		}
	}
}

func valueAt(attrs map[string]any, path string) any {
	cur := any(attrs)
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
	}
	return cur
}

func equalish(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if isEmpty(a) && isEmpty(b) {
		return true
	}
	if reflect.DeepEqual(normalizeScalar(a), normalizeScalar(b)) {
		return true
	}
	ab, err1 := json.Marshal(canonical(a))
	bb, err2 := json.Marshal(canonical(b))
	if err1 != nil || err2 != nil {
		return false
	}
	return string(ab) == string(bb)
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return t == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	default:
		return false
	}
}

func normalizeScalar(v any) any {
	switch t := v.(type) {
	case json.Number:
		return t.String()
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprint(t)
	default:
		return v
	}
}

func canonical(v any) any {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		m := make(map[string]any, len(keys))
		for _, k := range keys {
			m[k] = canonical(t[k])
		}
		return m
	case []any:
		cp := make([]any, len(t))
		for i, x := range t {
			cp[i] = canonical(x)
		}
		// order-insensitive for simple ID lists
		if allStringish(cp) {
			ss := make([]string, len(cp))
			for i, x := range cp {
				ss[i] = fmt.Sprint(x)
			}
			sort.Strings(ss)
			out := make([]any, len(ss))
			for i, s := range ss {
				out[i] = s
			}
			return out
		}
		return cp
	default:
		return normalizeScalar(t)
	}
}

func allStringish(vs []any) bool {
	for _, v := range vs {
		switch v.(type) {
		case string, json.Number:
		default:
			return false
		}
	}
	return true
}

func hash(attrs map[string]any) string {
	b, err := json.Marshal(canonical(stripIgnored(attrs)))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum)
}

func stripIgnored(attrs map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range attrs {
		if globalIgnore[k] {
			continue
		}
		out[k] = v
	}
	return out
}
