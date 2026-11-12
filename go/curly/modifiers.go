package curly

import (
	"cmp"
	"slices"
)

// builtin is the modifier the format defines under name, and whether it is
// one of the format's selections, which select among options and are asked
// for none in vain.
func builtin(name string) (run func(*slot) (string, Code), selection, known bool) {
	switch name {
	case "eq":
		return eq, true, true
	case "ne":
		return ne, true, true
	case "lt":
		return lt, true, true
	case "gt":
		return gt, true, true
	case "lte":
		return lte, true, true
	case "gte":
		return gte, true, true
	}

	return nil, false, false
}

// A selection that matched answers with that option's value, empty or not;
// only one that matched nothing reads the fallback chain.
func selected(s *slot, i int, ok bool) (string, Code) {
	if ok {
		return s.option(i), ""
	}

	return s.chain(), ""
}

// equal is the first option whose key equals the value as text,
// case-insensitively, or, with differ set, the first whose key does not.
// The value is lowered once, ahead of the options.
func equal(s *slot, differ bool) (int, bool) {
	needle := lower(s.value)

	for i, o := range s.d.options {
		if (lower(o.key) == needle) != differ {
			return i, true
		}
	}

	return 0, false
}

// numeric is the first option, in the order of its numeric key, whose key
// the value is strictly below (descending false) or strictly above
// (descending true). An option whose key is not a number takes no part:
// left in, it would compare as neither less nor greater and freeze the
// options around it. The order is a stable sort of a copy, so options of
// one value keep their source order and the message's list stays as it is.
func numeric(s *slot, descending bool) (int, bool) {
	input, ok := toNumber(s.value)

	if !ok {
		return 0, false
	}

	type keyed struct {
		n float64
		i int
	}

	var keys []keyed

	for i, o := range s.d.options {
		if n, ok := toNumber(o.key); ok {
			keys = append(keys, keyed{n, i})
		}
	}

	slices.SortStableFunc(keys, func(a, b keyed) int {
		if descending {
			return cmp.Compare(b.n, a.n)
		}

		return cmp.Compare(a.n, b.n)
	})

	for _, k := range keys {
		if descending && input > k.n || !descending && input < k.n {
			return k.i, true
		}
	}

	return 0, false
}

func eq(s *slot) (string, Code) {
	i, ok := equal(s, false)

	return selected(s, i, ok)
}

func ne(s *slot) (string, Code) {
	i, ok := equal(s, true)

	return selected(s, i, ok)
}

func lt(s *slot) (string, Code) {
	i, ok := numeric(s, false)

	return selected(s, i, ok)
}

func gt(s *slot) (string, Code) {
	i, ok := numeric(s, true)

	return selected(s, i, ok)
}

// lte and gte compare for equality over every option first, and reach the
// numeric order only where that selects nothing.
func lte(s *slot) (string, Code) {
	if i, ok := equal(s, false); ok {
		return s.option(i), ""
	}

	return lt(s)
}

func gte(s *slot) (string, Code) {
	if i, ok := equal(s, false); ok {
		return s.option(i), ""
	}

	return gt(s)
}
