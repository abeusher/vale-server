package spell

import "strings"

// maxAffixOps bounds how many affixes a word may be read back through: a
// prefix and a suffix at each of the levels expansion follows.
const maxAffixOps = 2 * (maxAffixDepth + 1)

// affixRef locates one rule of one affix class.
type affixRef struct {
	flag string
	idx  int
}

// reverseIndex finds the rules that could have added a word's ending or
// beginning; the upper-cased keys serve an all-caps word.
type reverseIndex struct {
	suffix, prefix           map[string][]affixRef
	suffixUpper, prefixUpper map[string][]affixRef
	maxSuffix, maxPrefix     int
}

// buildIndex indexes the affix rules by the text they add.
func (a *dictConfig) buildIndex() {
	idx := reverseIndex{
		suffix: map[string][]affixRef{}, prefix: map[string][]affixRef{},
		suffixUpper: map[string][]affixRef{}, prefixUpper: map[string][]affixRef{},
	}
	for flag, af := range a.AffixMap {
		for i, r := range af.Rules {
			ref := affixRef{flag: flag, idx: i}
			upper := strings.ToUpper(r.AffixText)
			if af.Type == Prefix {
				idx.prefix[r.AffixText] = append(idx.prefix[r.AffixText], ref)
				idx.prefixUpper[upper] = append(idx.prefixUpper[upper], ref)
				idx.maxPrefix = max(idx.maxPrefix, len(r.AffixText), len(upper))
			} else {
				idx.suffix[r.AffixText] = append(idx.suffix[r.AffixText], ref)
				idx.suffixUpper[upper] = append(idx.suffixUpper[upper], ref)
				idx.maxSuffix = max(idx.maxSuffix, len(r.AffixText), len(upper))
			}
		}
	}
	a.index = idx
}

// revState is a string on the way back from a word to a root, with the
// affix flags whoever produced it must carry: the class of each affix
// taken off since the last continuation, at most a prefix and a suffix.
type revState struct {
	text  string
	needs string // the flags, joined
}

// reverseSet returns word and every string that could be a step on the way
// to it: what remains after taking an affix's added text off and putting
// its stripped text back, up to maxAffixOps times, along paths the flags
// allow. With fold, the affix texts are matched upper-cased, for an
// all-caps word.
func (a *dictConfig) reverseSet(word string, fold bool) map[string]struct{} {
	idx := &a.index
	sufIdx, preIdx := idx.suffix, idx.prefix
	if fold {
		sufIdx, preIdx = idx.suffixUpper, idx.prefixUpper
	}
	strip := func(r *rule) string {
		if fold {
			return strings.ToUpper(r.Strip)
		}
		return r.Strip
	}

	allowed := map[string]struct{}{word: {}}
	seen := map[revState]struct{}{{text: word}: {}}
	frontier := []revState{{text: word}}
	for ops := 0; ops < maxAffixOps && len(frontier) > 0; ops++ {
		var next []revState
		visit := func(st revState, base string, ref affixRef, r *rule) {
			if base == "" {
				return
			}
			if !fold && r.matcher != nil && !r.matcher.MatchString(base) {
				return
			}
			for _, needs := range a.nextNeeds(st.needs, ref, r) {
				ns := revState{text: base, needs: needs}
				if _, ok := seen[ns]; ok {
					continue
				}
				seen[ns] = struct{}{}
				allowed[base] = struct{}{}
				next = append(next, ns)
			}
		}
		for _, st := range frontier {
			s := st.text
			for l := 0; l <= idx.maxSuffix && l <= len(s); l++ {
				for _, ref := range sufIdx[s[len(s)-l:]] {
					r := &a.AffixMap[ref.flag].Rules[ref.idx]
					visit(st, s[:len(s)-l]+strip(r), ref, r)
				}
			}
			for l := 0; l <= idx.maxPrefix && l <= len(s); l++ {
				for _, ref := range preIdx[s[:l]] {
					r := &a.AffixMap[ref.flag].Rules[ref.idx]
					visit(st, strip(r)+s[l:], ref, r)
				}
			}
		}
		frontier = next
	}
	return allowed
}

// nextNeeds returns what the producer of a rule's base must carry, once the
// rule is taken off a string whose producer had to carry needs: the rule may
// share the level with those affixes, or its continuation may have opened
// their level.
func (a *dictConfig) nextNeeds(needs string, ref affixRef, r *rule) []string {
	var out []string
	if needs == "" {
		return []string{ref.flag}
	}
	held := strings.Split(needs, "\x00")
	if len(held) == 1 && a.AffixMap[held[0]].Type != a.AffixMap[ref.flag].Type {
		// Same level: a prefix beside a suffix.
		out = append(out, ref.flag+"\x00"+held[0])
	}
	if r.Cont != "" {
		cont := a.parseFlags(r.Cont)
		opened := true
		for _, h := range held {
			if !stringIn(h, cont) {
				opened = false
				break
			}
		}
		if opened {
			out = append(out, ref.flag)
		}
	}
	return out
}

// analyses returns every reading of word as a root plus affixes, with the
// flags each reading carries. With fold, word is all-caps and matches the
// upper-cased form of any reading.
func (s *goSpell) analyses(word string, fold bool) []entryForm {
	return s.readings(word, fold, s.roots)
}

// readings is analyses over the given entries.
func (s *goSpell) readings(word string, fold bool, roots map[string][]rootEntry) []entryForm {
	allowed := s.affix.reverseSet(word, fold)

	key := func(w string) string { return w }
	rootsOf := func(cand string) []string {
		if _, ok := roots[cand]; ok {
			return []string{cand}
		}
		return nil
	}
	if fold {
		key = s.upperKey
		rootsOf = func(cand string) []string {
			var found []string
			for _, root := range s.upperRoots[cand] {
				if _, ok := roots[root]; ok {
					found = append(found, root)
				}
			}
			return found
		}
	}

	e := s.affix.expander(allowed, key)
	var out []entryForm
	for cand := range allowed {
		for _, root := range rootsOf(cand) {
			for _, entry := range roots[root] {
				for _, f := range e.root(root, entry.flags) {
					if key(f.Word) == word {
						out = append(out, f)
					}
				}
			}
		}
	}
	return out
}
