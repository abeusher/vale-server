package spell

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
)

type wordMatch struct {
	word  string
	score float64
}

type goSpell struct {
	dict  map[string]struct{}
	exact map[string]struct{} // KEEPCASE words, matched as written

	// forbidden holds FORBIDDENWORD forms, which no other path may accept.
	forbidden map[string]struct{}

	// segments holds the words a compound may be built from, and where.
	segments map[string]segment
	// pairs holds the run-together form of each two-word entry, which is
	// not a compound.
	pairs map[string]struct{}

	ireplacer   *strings.Replacer
	compounds   []*regexp.Regexp
	splitter    *splitter
	canCompound bool // dictionary uses COMPOUNDFLAG/BEGIN/MIDDLE/END
	compoundMin int
	checkDup    bool // CHECKCOMPOUNDDUP
	checkTriple bool // CHECKCOMPOUNDTRIPLE
	simplified  bool // SIMPLIFIEDTRIPLE
	checkCase   bool // CHECKCOMPOUNDCASE
	patterns    []compoundPattern
	breaks      []breakRule
	ignored     []string // .aff directives the reader does not implement
}

// breakRule is one Hunspell BREAK pattern: a literal that a word may be
// split on, or, when anchored, stripped from one end.
type breakRule struct {
	pattern string
	atStart bool // written `^pattern`
	atEnd   bool // written `pattern$`
}

// maxBreakDepth bounds how many times a word may be split by BREAK rules.
const maxBreakDepth = 10

// defaultBreaks is what Hunspell uses when a dictionary declares no BREAK.
var defaultBreaks = []string{"-", "^-", "-$"}

func newBreakRules(patterns []string) []breakRule {
	rules := make([]breakRule, 0, len(patterns))
	for _, p := range patterns {
		r := breakRule{pattern: p}
		if len(r.pattern) > 1 && strings.HasPrefix(r.pattern, "^") {
			r.atStart = true
			r.pattern = r.pattern[1:]
		}
		if len(r.pattern) > 1 && strings.HasSuffix(r.pattern, "$") {
			r.atEnd = true
			r.pattern = r.pattern[:len(r.pattern)-1]
		}
		if r.pattern != "" {
			rules = append(rules, r)
		}
	}
	return rules
}

type dictionary struct {
	dic string
	aff string
}

// inputConversion does any character substitution before checking
//
//	This is based on the ICONV stanza
func (s *goSpell) inputConversion(raw []byte) string {
	sraw := string(raw)
	if s.ireplacer == nil {
		return sraw
	}
	return s.ireplacer.Replace(sraw)
}

// addWordRaw adds a single word to the internal dictionary without modifications
// returns true if added
// return false is already exists
func (s *goSpell) addWordRaw(word string) bool {
	_, ok := s.dict[word]
	if ok {
		// already exists
		return false
	}
	s.dict[word] = struct{}{}
	return true
}

// addWordListFile reads in a word list file
func (s *goSpell) addWordListFile(name string) ([]string, error) {
	fd, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	return s.addWordList(fd)
}

// addWordList adds basic word lists, just one word per line
//
//	Assumed to be in UTF-8
//
// TODO: hunspell compatible with "*" prefix for forbidden words
// and affix support
// returns list of duplicated words and/or error
func (s *goSpell) addWordList(r io.Reader) ([]string, error) {
	var duplicates []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		if len(word) == 0 || word == "#" {
			continue
		}
		if !s.addWordRaw(word) {
			duplicates = append(duplicates, word)
		}
	}
	if err := scanner.Err(); err != nil {
		return duplicates, err
	}
	return duplicates, nil
}

func (s *goSpell) keys() []string {
	keys := make([]string, len(s.dict))

	i := 0
	for k := range s.dict {
		keys[i] = k
		i++
	}

	return keys
}

func (s *goSpell) suggest(word string) []wordMatch {
	metric := metrics.NewLevenshtein()

	// Distance is measured case-insensitively; the case comes back below.
	lower := strings.ToLower(word)

	matches := []wordMatch{}
	for _, option := range s.keys() {
		sim := strutil.Similarity(option, lower, metric)
		matches = append(matches, wordMatch{option, sim})
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	hits := matches
	if len(hits) > 5 {
		hits = hits[:5]
	}

	// Suggestions take the case the word was written in.
	switch {
	case allUpper(word):
		for i := range hits {
			hits[i].word = strings.ToUpper(hits[i].word)
		}
	case initialUpper(word):
		for i := range hits {
			hits[i].word = capitalize(hits[i].word)
		}
	}

	return hits
}

// allUpper reports whether word has letters and every one is upper-case.
func allUpper(word string) bool {
	letters := false
	for _, r := range word {
		if !unicode.IsLetter(r) {
			continue
		}
		if !unicode.IsUpper(r) {
			return false
		}
		letters = true
	}
	return letters
}

// initialUpper reports whether word starts with an upper-case letter.
func initialUpper(word string) bool {
	for _, r := range word {
		return unicode.IsUpper(r)
	}
	return false
}

// spell checks to see if a given word is in the internal dictionaries
func (s *goSpell) spell(word string) bool {
	return s.spellDepth(word, 0)
}

// spellDepth is spell with a count of how many BREAK splits led here.
func (s *goSpell) spellDepth(word string, depth int) bool {
	if _, ok := s.forbidden[word]; ok {
		return false
	}
	if _, ok := s.exact[word]; ok {
		return true
	}
	_, ok := s.dict[word]
	if ok {
		return true
	}
	lower := strings.ToLower(word)
	if _, ok = s.dict[lower]; ok {
		return true
	}
	// An upper-cased word matches a capitalized entry: PARIS for Paris.
	if allUpper(word) {
		if _, ok = s.dict[capitalize(lower)]; ok {
			return true
		}
	}

	if isNumber(word) {
		return true
	}
	if isNumberHex(word) {
		return true
	}

	if isNumberBinary(word) {
		return true
	}

	if isHash(word) {
		return true
	}

	// check compounds
	for _, pat := range s.compounds {
		if pat.MatchString(word) {
			return true
		}
	}

	// Affix-flag compounding (German, Dutch, ...): accept a word that splits
	// into dictionary segments. See #848.
	if s.isCompound(word) {
		return true
	}

	// Maybe a word with units? e.g. 100GB
	units := isNumberUnits(word)
	if units != "" {
		// dictionary appears to have list of units
		if _, ok = s.dict[units]; ok {
			return true
		}
	}

	return s.breakParts(word, depth)
}

// breakParts reports whether word is valid once split by the dictionary's
// BREAK rules: an anchored rule strips its pattern from that end, and any
// other rule splits at each occurrence, with both sides then checked on
// their own (and split again as needed). See #1165.
func (s *goSpell) breakParts(word string, depth int) bool {
	if depth >= maxBreakDepth {
		return false
	}
	for _, r := range s.breaks {
		n := len(r.pattern)
		switch {
		case r.atStart:
			if len(word) > n && strings.HasPrefix(word, r.pattern) &&
				s.spellDepth(word[n:], depth+1) {
				return true
			}
		case r.atEnd:
			if len(word) > n && strings.HasSuffix(word, r.pattern) &&
				s.spellDepth(word[:len(word)-n], depth+1) {
				return true
			}
		default:
			// Only interior occurrences: both sides must be non-empty.
			for i := strings.Index(word[1:], r.pattern); i >= 0; {
				at := i + 1
				if at+n >= len(word) {
					break
				}
				if s.spellDepth(word[:at], depth+1) &&
					s.spellDepth(word[at+n:], depth+1) {
					return true
				}
				next := strings.Index(word[at+1:], r.pattern)
				if next < 0 {
					break
				}
				i = at + next
			}
		}
	}
	return false
}

// segment returns what a compound may use word as, trying its exact,
// lower-cased, and capitalized forms: a German compound writes interior
// nouns lower-case, while the dictionary stores them capitalized.
func (s *goSpell) segment(word string) (segment, bool) {
	if seg, ok := s.segments[word]; ok {
		return seg, true
	}
	if seg, ok := s.segments[strings.ToLower(word)]; ok {
		return seg, true
	}
	seg, ok := s.segments[capitalize(word)]
	return seg, ok
}

// isCompound reports whether word is built from segments the dictionary's
// compound flags allow, for dictionaries that enable flag compounding.
func (s *goSpell) isCompound(word string) bool {
	if !s.canCompound {
		return false
	}
	if _, ok := s.pairs[word]; ok {
		return false
	}
	// Bound the work: very long inputs are unlikely to be real words and the
	// recursion is super-linear.
	if r := []rune(word); len(r) <= 100 {
		return s.compoundParts(r, word, nil, nil, 0)
	}
	return false
}

// piece is a segment of a compound as written, with what it may do.
type piece struct {
	text string
	use  segment
}

// compoundParts reports whether runes split into segments allowed at their
// positions. prev is the segment before them; need is the pattern whose
// replacement they follow, if any; word is the whole word, for FORCEUCASE.
func (s *goSpell) compoundParts(runes []rune, word string, prev *piece, need *compoundPattern, depth int) bool {
	if depth > 4 { // cap the number of segments
		return false
	}
	minLen := max(s.compoundMin, 1)
	n := len(runes)

	for i := minLen; i <= n-minLen; i++ {
		if !s.boundaryOK(runes, i) {
			continue
		}
		left, rest := string(runes[:i]), runes[i:]

		// With SIMPLIFIEDTRIPLE, `glassko` is glass+sko: the letter the two
		// share is written once.
		candidates := []string{left}
		if s.simplified && runes[i-1] == runes[i] {
			candidates = append(candidates, left+string(runes[i]))
		}
		for _, seg := range candidates {
			if s.compoundFrom(seg, rest, word, prev, need, nil, depth) {
				return true
			}
		}
	}

	// A pattern with a replacement writes the boundary as the replacement:
	// `a/A u/A O` makes sUrya+udayaM into sUryOdayaM.
	for pi := range s.patterns {
		p := &s.patterns[pi]
		if p.repl == "" {
			continue
		}
		repl := []rune(p.repl)
		for j := 1; j+len(repl) < n; j++ {
			if string(runes[j:j+len(repl)]) != p.repl {
				continue
			}
			seg := string(runes[:j]) + p.end
			rest := append([]rune(p.begin), runes[j+len(repl):]...)
			if s.compoundFrom(seg, rest, word, prev, need, p, depth) {
				return true
			}
		}
	}
	return false
}

// compoundFrom reports whether seg, followed by rest, completes a compound.
// need is the pattern seg has to satisfy the right side of; next the one
// whose replacement separates seg from what follows.
func (s *goSpell) compoundFrom(seg string, rest []rune, word string, prev *piece, need, next *compoundPattern, depth int) bool {
	use, ok := s.segment(seg)
	first := prev == nil
	if !ok || (first && !use.begin) || (!first && !use.middle) {
		return false
	}
	if need != nil && need.beginFlag != "" && !hasFlag(use.flags, need.beginFlag) {
		return false
	}
	if next != nil && ((next.endFlag != "" && !hasFlag(use.flags, next.endFlag)) ||
		(next.stemOnly && use.affixed)) {
		return false
	}
	cur := &piece{text: seg, use: use}
	if prev != nil && s.patternForbids(prev, cur) {
		return false
	}

	last := string(rest)
	if end, isEnd := s.segment(last); isEnd && end.end &&
		!(s.checkDup && last == seg) && (!end.upper || initialUpper(word)) {
		if next != nil {
			// Hunspell forbids a duplicate only at the end: foofoobar is
			// fine, foobarbar is not.
			return next.beginFlag == "" || hasFlag(end.flags, next.beginFlag)
		}
		if !s.patternForbids(cur, &piece{text: last, use: end}) {
			return true
		}
	}
	return s.compoundParts(rest, word, cur, next, depth+1)
}

// patternForbids reports whether a CHECKCOMPOUNDPATTERN forbids writing
// left and right together.
func (s *goSpell) patternForbids(left, right *piece) bool {
	for i := range s.patterns {
		p := &s.patterns[i]
		if p.stemOnly && left.use.affixed {
			continue
		}
		if !strings.HasSuffix(left.text, p.end) || !strings.HasPrefix(right.text, p.begin) {
			continue
		}
		if (p.endFlag != "" && !hasFlag(left.use.flags, p.endFlag)) ||
			(p.beginFlag != "" && !hasFlag(right.use.flags, p.beginFlag)) {
			continue
		}
		return true
	}
	return false
}

// boundaryOK applies the checks a dictionary asks for at a compound boundary
// before runes[i], as the word is written.
func (s *goSpell) boundaryOK(runes []rune, i int) bool {
	before, after := runes[i-1], runes[i]
	if s.checkTriple {
		if (i >= 2 && runes[i-2] == before && before == after) ||
			(i+1 < len(runes) && before == after && after == runes[i+1]) {
			return false
		}
	}
	if s.checkCase && before != '-' && after != '-' &&
		(unicode.IsUpper(before) || unicode.IsUpper(after)) {
		return false
	}
	return true
}

// isMorphField reports whether a `.dic` field is morphology, `po:noun`.
func isMorphField(field string) bool {
	return len(field) > 2 && field[2] == ':' &&
		unicode.IsLetter(rune(field[0])) && unicode.IsLetter(rune(field[1]))
}

// capitalize upper-cases the first rune of s, leaving the rest unchanged.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// newGoSpellReader creates a speller from io.Readers for
// Hunspell files
func newGoSpellReader(aff, dic io.Reader) (*goSpell, error) {
	affix, err := newDictConfig(aff)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(dic)
	// get first line
	if !scanner.Scan() {
		return nil, scanner.Err()
	}

	gs := goSpell{
		// TODO: Use fixed size from first list?
		dict:        make(map[string]struct{}),
		exact:       make(map[string]struct{}),
		compounds:   make([]*regexp.Regexp, 0, len(affix.CompoundRule)),
		splitter:    newSplitter(affix.WordChars),
		canCompound: affix.compoundingEnabled(),
		compoundMin: affix.CompoundMin,
		checkDup:    affix.CheckCompoundDup,
		checkTriple: affix.CheckCompoundTriple,
		simplified:  affix.SimplifiedTriple,
		checkCase:   affix.CheckCompoundCase,
		patterns:    affix.CompoundPatterns,
		breaks:      newBreakRules(affix.Break),
		ignored:     affix.Ignored,
		forbidden:   make(map[string]struct{}),
		segments:    make(map[string]segment),
		pairs:       make(map[string]struct{}),
	}
	if !affix.BreakDeclared {
		gs.breaks = newBreakRules(defaultBreaks)
	}

	forbidden := []string{}
	stems := map[string]struct{}{} // of the entries read so far that are not forbidden
	for scanner.Scan() {
		line := scanner.Text()
		// A .dic entry is `word/flags` optionally followed by whitespace-
		// separated morphological fields, e.g.
		//
		//	abandonware/M	Noun: uncountable
		//	coitus/10,39,31 al:coituum
		//
		// Keep only the first field; otherwise the morphology corrupts flag
		// parsing (e.g., FLAG num would read "31 al:coituum" as a flag).
		//
		// Both tab- and space-separated morphology occur in the wild -- the
		// Danish dictionary from stavekontrolden.dk uses spaces. See #1065.
		// The word ends at the first tab; morphology follows it.
		line, _, _ = strings.Cut(line, "\t")
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		line = fields[0]

		// A space inside the word makes a pair, `compound word`, whose
		// run-together form is not a compound.
		if len(fields) > 1 && !isMorphField(fields[1]) {
			gs.pairs[fields[0]+fields[1]] = struct{}{}
			gs.dict[fields[0]+" "+fields[1]] = struct{}{}
			continue
		}

		forms, expandErr := affix.expandEntry(line)
		if expandErr != nil {
			// Skip malformed entries (e.g., a line with flags but no word)
			// rather than abandoning the entire dictionary, which would leave
			// every word unrecognized and flagged. See #1065.
			continue
		}

		if len(forms) == 0 {
			continue
		}

		switch {
		case affix.entryHas(line, affix.ForbiddenFlag):
			// Hunspell reads homonyms in order and the first decides, so
			// an earlier entry keeps its stem valid.
			for _, f := range forms {
				if _, seen := stems[f.Word]; !seen {
					forbidden = append(forbidden, f.Word)
				}
			}
			continue
		case affix.entryHas(line, affix.KeepCaseFlag):
			for _, f := range forms {
				gs.exact[f.Word] = struct{}{}
			}
		default:
			for _, f := range forms {
				if !hasFlag(f.Flags, affix.CompoundOnly) {
					gs.dict[f.Word] = struct{}{}
				}
				if seg, ok := affix.compoundUse(f); ok {
					gs.segments[f.Word] = seg
				}
			}
		}
		stem, _, _ := strings.Cut(line, "/")
		stems[stem] = struct{}{}
	}

	if err = scanner.Err(); err != nil {
		return nil, err
	}

	// A forbidden word overrides any entry that generates it.
	for _, word := range forbidden {
		delete(gs.dict, word)
		delete(gs.exact, word)
		delete(gs.segments, word)
		gs.forbidden[word] = struct{}{}
	}

	for _, compoundRule := range affix.CompoundRule {
		pattern := "^"
		for _, key := range affix.parseFlags(compoundRule) {
			if len(key) == 1 {
				r := rune(key[0])
				switch r {
				case '(', ')', '+', '?', '*':
					pattern += regexp.QuoteMeta(key)
					continue
				}
			}
			groups := affix.compoundMap[key]
			pattern = pattern + "(" + strings.Join(groups, "|") + ")"
		}
		pattern += "$"

		pat, perr := regexp.Compile(pattern)
		if perr != nil {
			return nil, perr
		}
		gs.compounds = append(gs.compounds, pat)
	}

	if len(affix.IconvReplacements) > 0 {
		gs.ireplacer = strings.NewReplacer(affix.IconvReplacements...)
	}
	return &gs, nil
}

// newGoSpell from AFF and DIC Hunspell filenames
func newGoSpell(affFile, dicFile string) (*goSpell, error) {
	aff, err := os.Open(affFile)
	if err != nil {
		return nil, fmt.Errorf("unable to open aff: %s", err.Error())
	}
	defer aff.Close()
	dic, err := os.Open(dicFile)
	if err != nil {
		return nil, fmt.Errorf("unable to open dic: %s", err.Error())
	}
	defer dic.Close()
	h, err := newGoSpellReader(aff, dic)
	return h, err
}
