package spell

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// affixType is either an affix prefix or suffix
type affixType int

// specific Affix types
const (
	Prefix affixType = iota
	Suffix
)

// affix is a rule for affix (adding prefixes or suffixes)
type affix struct {
	Rules        []rule    // -
	Type         affixType // either PFX or SFX
	CrossProduct bool      // -
}

// entryForm is one word an entry generates: the flags it carries, from the
// stem and every rule applied, and the affixes it was built with.
type entryForm struct {
	Word  string
	Flags []string

	prefix, suffix *rule    // nil when none was applied
	cont           []string // the flags of the rule that built it

	// wantsAffix is set by NEEDAFFIX on the stem or a rule; plainAffix by
	// a rule without it, which satisfies the want.
	wantsAffix, plainAffix bool

	// open is the affix type of a CIRCUMFIX rule still waiting for its
	// partner of the other type.
	open *affixType
}

// virtual reports whether f is only a step toward another form.
func (f entryForm) virtual() bool {
	return f.open != nil || (f.wantsAffix && !f.plainAffix)
}

// union returns a followed by what b adds to it.
func union(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	out = append(out, a...)
	for _, x := range b {
		if !stringIn(x, out) {
			out = append(out, x)
		}
	}
	return out
}

// rule is a Affix rule
type rule struct {
	Strip     string
	AffixText string // suffix or prefix text to add

	// Cont holds the continuation flags the rule carries, if any -- the
	// "34,22" of `SFX 1 0 t/34,22 e`. They name the affix classes that apply
	// again to the form this rule produces, which is how a dictionary spells
	// out an inflection built in more than one step.
	Cont string

	Pattern string         // original matching pattern from AFF file
	matcher *regexp.Regexp // matcher to see if this rule applies or not
}

// dictConfig is a partial representation of a Hunspell AFF (Affix) file.
const (
	// defaultCompoundMin is Hunspell's own default for COMPOUNDMIN.
	defaultCompoundMin = 3
	// maxCompoundMin is where a COMPOUNDMIN stops being a plausible word
	// length and starts being a typo or worse.
	maxCompoundMin = 100
	// maxCompoundRules caps what a COMPOUNDRULE count may preallocate.
	maxCompoundRules = 1 << 16
	// maxBreakRules caps how many BREAK patterns are kept; a real
	// dictionary declares a handful.
	maxBreakRules = 64
)

type dictConfig struct {
	IconvReplacements  []string
	Replacements       [][2]string
	CompoundRule       []string
	Break              []string
	Flag               string
	TryChars           string
	WordChars          string
	CompoundOnly       string
	CompoundFlag       string
	CompoundBegin      string
	CompoundMiddle     string
	CompoundEnd        string
	AffixMap           map[string]affix
	CamelCase          int
	CompoundMin        int
	compoundMap        map[string][]string
	NoSuggestFlag      string
	ForbiddenFlag      string   // FORBIDDENWORD: the entry's forms are not words
	NeedAffixFlag      string   // NEEDAFFIX: the bare stem is not a word
	KeepCaseFlag       string   // KEEPCASE: accepted only as written
	CircumfixFlag      string   // CIRCUMFIX: a prefix and suffix that go together
	CompoundPermitFlag string   // COMPOUNDPERMITFLAG: an affix allowed inside a compound
	CompoundForbidFlag string   // COMPOUNDFORBIDFLAG: a form kept out of compounds
	ForceUCaseFlag     string   // FORCEUCASE: a compound ending here is capitalized
	Aliases            []string // AF: flag sets that an entry names by number

	BreakDeclared       bool // any BREAK line; otherwise Hunspell's defaults apply
	CheckCompoundDup    bool // CHECKCOMPOUNDDUP: no segment twice in a row
	CheckCompoundTriple bool // CHECKCOMPOUNDTRIPLE: no letter three times at a boundary
	SimplifiedTriple    bool // SIMPLIFIEDTRIPLE: a triple may be written as a double
	CheckCompoundCase   bool // CHECKCOMPOUNDCASE: no upper-case letter at a boundary
	CheckCompoundRep    bool // CHECKCOMPOUNDREP: no compound a REP entry turns into a word
	CompoundWordMax     int  // COMPOUNDWORDMAX: segments a compound may have; 0 is no limit

	// COMPOUNDSYLLABLE: a compound of more segments than COMPOUNDWORDMAX is
	// allowed when it has no more syllables than this, counted as vowels.
	CompoundSyllable int
	CompoundVowels   string
	CompoundPatterns []compoundPattern

	// Ignored names the directives the file used that this reader does not
	// implement, in the order they were first seen.
	Ignored []string

	index reverseIndex

	// byteFlags says a flag is one byte, as Hunspell reads them from a
	// UTF-8 file without FLAG: a non-ASCII flag character is then two
	// flags, and a class name is its first byte.
	byteFlags bool
}

// compoundingEnabled reports whether the dictionary uses affix-flag-based
// compounding (COMPOUNDFLAG / COMPOUNDBEGIN / MIDDLE / END), as German, Dutch,
// etc. do. COMPOUNDRULE is handled separately. See #848.
func (a *dictConfig) compoundingEnabled() bool {
	return a.CompoundFlag != "" || a.CompoundBegin != "" ||
		a.CompoundMiddle != "" || a.CompoundEnd != ""
}

// parseFlags splits a flag string into individual flags based on the FLAG type.
//
// Hunspell supports several flag formats:
//   - "ASCII" (default): each character is a flag
//   - "num": flags are comma-separated numbers (e.g., "14308,10482,4720")
//   - "UTF-8": each UTF-8 character is a flag
//   - "long": each pair of ASCII characters is a flag
func (a dictConfig) parseFlags(flagStr string) []string {
	// With AF, an entry's flags are the number of an alias line.
	if len(a.Aliases) > 0 && allDigits(flagStr) {
		n, err := strconv.Atoi(flagStr)
		if err != nil || n < 1 || n > len(a.Aliases) {
			return nil
		}
		flagStr = a.Aliases[n-1]
	}
	switch a.Flag {
	case "num":
		return strings.Split(flagStr, ",")
	case "long":
		flags := make([]string, 0, len(flagStr)/2)
		for i := 0; i+1 < len(flagStr); i += 2 {
			flags = append(flags, flagStr[i:i+2])
		}
		return flags
	case "UTF-8":
		flags := make([]string, 0, len(flagStr))
		for _, r := range flagStr {
			flags = append(flags, string(r))
		}
		return flags
	default:
		if a.byteFlags {
			flags := make([]string, 0, len(flagStr))
			for i := 0; i < len(flagStr); i++ {
				flags = append(flags, flagStr[i:i+1])
			}
			return flags
		}
		flags := make([]string, 0, len(flagStr))
		for _, r := range flagStr {
			flags = append(flags, string(r))
		}
		return flags
	}
}

// ruleToken is one element of a COMPOUNDRULE: a flag, or the `*` or `?`
// that follows one.
type ruleToken struct {
	flag, op string
}

// compoundRuleTokens splits a COMPOUNDRULE. A flag in parentheses may be
// several characters, as FLAG long and num need; any other is one flag.
func (a dictConfig) compoundRuleTokens(rule string) []ruleToken {
	var out []ruleToken
	for i := 0; i < len(rule); {
		switch c := rule[i]; {
		case c == '*' || c == '?':
			out = append(out, ruleToken{op: string(c)})
			i++
		case c == '(':
			end := strings.IndexByte(rule[i:], ')')
			if end < 0 {
				out = append(out, ruleToken{flag: rule[i+1:]})
				return out
			}
			out = append(out, ruleToken{flag: rule[i+1 : i+end]})
			i += end + 1
		default:
			n := 1
			if !a.byteFlags {
				_, n = utf8.DecodeRuneInString(rule[i:])
			}
			out = append(out, ruleToken{flag: rule[i : i+n]})
			i += n
		}
	}
	return out
}

// singleFlag reads a directive that names one flag.
func (a dictConfig) singleFlag(s string) string {
	if flags := a.parseFlags(s); len(flags) > 0 {
		return flags[0]
	}
	return s
}

// expand returns the words a `.dic` entry generates.
func (a *dictConfig) expand(entry string, out []string) ([]string, error) {
	forms, err := a.expandEntry(entry)
	if err != nil {
		return nil, err
	}
	out = out[:0]
	for _, f := range forms {
		out = append(out, f.Word)
	}
	return out, nil
}

// expandEntry returns every form a `.dic` entry generates, with its flags.
func (a *dictConfig) expandEntry(entry string) ([]entryForm, error) {
	word, keyString, found := strings.Cut(entry, "/")
	if !found {
		return []entryForm{{Word: entry}}, nil
	}
	if word == "" || keyString == "" {
		return nil, fmt.Errorf("slash char found in first or last position")
	}
	return a.expander(nil, nil).root(word, a.parseFlags(keyString)), nil
}

// expander generates a root's forms. With allowed set, only forms whose key
// is in it are kept, which confines the walk to the path toward one word.
type expander struct {
	a       *dictConfig
	allowed map[string]struct{}
	key     func(string) string
}

func (a *dictConfig) expander(allowed map[string]struct{}, key func(string) string) expander {
	if key == nil {
		key = func(s string) string { return s }
	}
	return expander{a: a, allowed: allowed, key: key}
}

// root returns the forms of a root with the given flags.
func (e expander) root(word string, flags []string) []entryForm {
	stem := entryForm{Word: word, Flags: flags, cont: flags,
		wantsAffix: hasFlag(flags, e.a.NeedAffixFlag)}
	return e.emit(stem, 0)
}

// emit returns f, unless it is only a step toward another form, and then
// the forms the affixes its flags name build on it.
func (e expander) emit(f entryForm, depth int) []entryForm {
	var out []entryForm
	if !f.virtual() {
		out = append(out, f)
	}
	if depth > maxAffixDepth {
		return out
	}
	return append(out, e.affixed(f, f.cont, depth)...)
}

// affixed builds the forms the affixes named by keys make from base.
func (e expander) affixed(base entryForm, keys []string, depth int) []entryForm {
	var out []entryForm
	prefixes := make([]affix, 0, 5)
	suffixes := make([]affix, 0, 5)
	for _, key := range keys {
		af, ok := e.a.AffixMap[key]
		if !ok {
			continue
		}
		if !af.CrossProduct {
			out = e.emitAll(e.derive(af, base), out, depth)
			continue
		}
		if af.Type == Prefix {
			prefixes = append(prefixes, af)
		} else {
			suffixes = append(suffixes, af)
		}
	}

	for _, suf := range suffixes {
		out = e.emitAll(e.derive(suf, base), out, depth)
	}
	for _, pre := range prefixes {
		prefixed := e.derive(pre, base)
		out = e.emitAll(prefixed, out, depth)

		// now do cross product
		for _, suf := range suffixes {
			for _, pw := range prefixed {
				out = e.emitAll(e.derive(suf, pw), out, depth)
			}
		}
	}
	return out
}

// emitAll emits each form, following its continuation flags one level down.
//
// This is the step Hunspell calls twofold affixation: `SFX 1 0 t/34,22 e`
// says that after the rule builds its form, classes 34 and 22 apply to that.
func (e expander) emitAll(forms []entryForm, out []entryForm, depth int) []entryForm {
	for _, f := range forms {
		out = append(out, e.emit(f, depth+1)...)
	}
	return out
}

// derive applies the affix's rules to base, one form per rule that matches.
func (e expander) derive(af affix, base entryForm) []entryForm {
	a := e.a
	var out []entryForm
	for i := range af.Rules {
		// The condition is tested last: building the form and checking it
		// against allowed is cheaper than the regular expression.
		r := &af.Rules[i]
		f := base
		f.cont = nil
		if r.Cont != "" {
			f.cont = a.parseFlags(r.Cont)
			f.Flags = union(base.Flags, f.cont)
		}
		if hasFlag(f.cont, a.NeedAffixFlag) {
			f.wantsAffix = true
		} else {
			f.plainAffix = true
		}
		if hasFlag(f.cont, a.CircumfixFlag) {
			if base.open != nil && *base.open != af.Type {
				f.open = nil // paired
			} else {
				kind := af.Type
				f.open = &kind
			}
		}
		if af.Type == Prefix {
			stripped := base.Word
			if r.Strip != "" && strings.HasPrefix(stripped, r.Strip) {
				stripped = stripped[len(r.Strip):]
			}
			f.Word = r.AffixText + stripped
			f.prefix = r
		} else {
			stripped := base.Word
			if r.Strip != "" && strings.HasSuffix(stripped, r.Strip) {
				stripped = stripped[:len(stripped)-len(r.Strip)]
			}
			f.Word = stripped + r.AffixText
			f.suffix = r
		}
		if e.allowed != nil {
			if _, ok := e.allowed[e.key(f.Word)]; !ok {
				continue
			}
		}
		if r.matcher != nil && !r.matcher.MatchString(base.Word) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// maxAffixDepth bounds how many times a continuation class may be followed.
//
// Hunspell's default is twofold affixation -- one continuation -- and this
// allows one more for dictionaries that lean on longer chains. A bound is what
// makes this safe at all: nothing stops an .aff file from having a class
// continue to itself, and following that faithfully would not terminate.
const maxAffixDepth = 2

// segment is what a form may do in a compound.
type segment struct {
	begin, middle, end bool
	upper              bool     // FORCEUCASE: a compound ending here is capitalized
	keep               bool     // KEEPCASE: not part of a case-folded compound
	affixed            bool     // built with an affix, which a `0` pattern excludes
	flags              []string // for the flags a CHECKCOMPOUNDPATTERN names
}

// merge combines what two forms of the same word may do.
func (s segment) merge(o segment) segment {
	return segment{
		begin: s.begin || o.begin, middle: s.middle || o.middle, end: s.end || o.end,
		upper:   s.upper || o.upper,
		keep:    s.keep || o.keep,
		affixed: s.affixed && o.affixed,
		flags:   union(s.flags, o.flags),
	}
}

// compoundUse reports whether f may be a compound segment, and where.
func (a dictConfig) compoundUse(f entryForm) (segment, bool) {
	if hasFlag(f.Flags, a.CompoundForbidFlag) {
		return segment{}, false
	}
	anywhere := hasFlag(f.Flags, a.CompoundFlag)
	seg := segment{
		begin:   anywhere || hasFlag(f.Flags, a.CompoundBegin),
		middle:  anywhere || hasFlag(f.Flags, a.CompoundMiddle),
		end:     anywhere || hasFlag(f.Flags, a.CompoundEnd),
		upper:   hasFlag(f.Flags, a.ForceUCaseFlag),
		affixed: f.prefix != nil || f.suffix != nil,
		flags:   f.Flags,
	}
	// An affix keeps its form at the compound's edge unless it permits more.
	if f.prefix != nil && !hasFlag(a.parseFlags(f.prefix.Cont), a.CompoundPermitFlag) {
		seg.middle, seg.end = false, false
	}
	if f.suffix != nil {
		cont := a.parseFlags(f.suffix.Cont)
		if !hasFlag(cont, a.CompoundPermitFlag) {
			seg.begin, seg.middle = false, false
		}
		// A suffix only for compounds, a Fuge-s, ends a compound only if
		// it says so itself.
		if hasFlag(cont, a.CompoundOnly) &&
			!hasFlag(cont, a.CompoundFlag) && !hasFlag(cont, a.CompoundEnd) {
			seg.end = false
		}
	}
	return seg, seg.begin || seg.middle || seg.end
}

// compoundPattern is one CHECKCOMPOUNDPATTERN line: a boundary that is
// forbidden as written, and may be written as repl instead.
type compoundPattern struct {
	end, begin         string // what the left segment ends with and the right begins with
	endFlag, beginFlag string // flags each must carry, if any
	stemOnly           bool   // `0`: the left segment is an unaffixed stem
	repl               string
}

// conditionPattern turns an affix condition into a regular expression. A
// hyphen inside a Hunspell class is a literal, never a range.
func conditionPattern(cond string) string {
	var b strings.Builder
	inClass := false
	for _, r := range cond {
		switch {
		case r == '[':
			inClass = true
		case r == ']':
			inClass = false
		case r == '-' && inClass:
			b.WriteString(`\-`)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// parsePatternSide reads `chars[/flag]`, returning the characters, the
// flag, and whether `0` asked for an unaffixed stem.
func parsePatternSide(s string) (string, string, bool) {
	chars, flag, _ := strings.Cut(s, "/")
	if chars == "0" {
		return "", flag, true
	}
	return chars, flag, false
}

// hasFlag reports whether flag is set and among flags.
func hasFlag(flags []string, flag string) bool {
	return flag != "" && stringIn(flag, flags)
}

func stringIn(s string, list []string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isCrossProduct(val string) (bool, error) {
	switch val {
	case "Y":
		return true, nil
	case "N":
		return false, nil
	}
	return false, fmt.Errorf("CrossProduct is not Y or N: got %q", val)
}

// newDictConfig reads an Hunspell AFF file
func newDictConfig(file io.Reader) (*dictConfig, error) { //nolint:funlen
	aff := dictConfig{
		Flag:        "ASCII",
		AffixMap:    make(map[string]affix),
		compoundMap: make(map[string][]string),
		CompoundMin: defaultCompoundMin,
	}
	sawBreakCount := false
	sawAliasCount := false
	aff.byteFlags = true

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		switch parts[0] {
		case "SET":
			// Decoded before parsing; see decodeDictionary. A UTF-8 file's
			// flags are bytes unless FLAG says otherwise.
			if len(parts) >= 2 {
				upper := strings.ToUpper(parts[1])
				aff.byteFlags = upper == "UTF-8" || upper == "UTF8"
			}
		case "TRY":
			if len(parts) < 2 {
				return nil, fmt.Errorf("TRY stanza had %d fields, expected 2", len(parts))
			}
			aff.TryChars = parts[1]
		case "ICONV":
			// if only 2 fields, then its the first stanza that just provides a count
			//  we don't care, as we dynamically allocate
			if len(parts) == 2 {
				continue
			} else if len(parts) < 3 {
				return nil, fmt.Errorf("ICONV stanza had %d fields, expected 2", len(parts))
			}
			aff.IconvReplacements = append(aff.IconvReplacements, parts[1], parts[2])
		case "REP":
			if len(parts) == 2 {
				continue
			} else if len(parts) < 3 {
				return nil, fmt.Errorf("REP stanza had %d fields, expected 2", len(parts))
			}
			aff.Replacements = append(aff.Replacements, [2]string{parts[1], parts[2]})
		case "COMPOUNDMIN":
			if len(parts) < 2 {
				return nil, fmt.Errorf("COMPOUNDMIN stanza had %d fields, expected 2", len(parts))
			}
			// Parsed at a fixed width rather than with Atoi, whose `int` is
			// the target's word size: on a 32-bit build that made the value
			// where a number stops being representable -- and so the line
			// between a clamped COMPOUNDMIN and a rejected one -- depend on
			// the architecture. See #1159.
			val, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("COMPOUNDMIN stanza had %q expected number", parts[1])
			}
			// Hunspell ignores a value outside this range and uses its default,
			// rather than refusing the dictionary; a `.aff` is data we are given,
			// so an absurd length is not worth failing over. Bounding it here is
			// also what keeps the value safe to use as a length below.
			aff.CompoundMin = defaultCompoundMin
			if val >= 1 && val <= maxCompoundMin {
				aff.CompoundMin = int(val)
			}
		case "ONLYINCOMPOUND":
			if len(parts) < 2 {
				return nil, fmt.Errorf("ONLYINCOMPOUND stanza had %d fields, expected 2", len(parts))
			}
			aff.CompoundOnly = aff.singleFlag(parts[1])
		case "COMPOUNDRULE":
			if len(parts) < 2 {
				return nil, fmt.Errorf("COMPOUNDRULE stanza had %d fields, expected 2", len(parts))
			}
			val, err := strconv.ParseInt(parts[1], 10, 64)
			if err == nil {
				// A count read from the file, so it only preallocates -- the
				// slice grows on its own if the count was low, and a wild one
				// cannot ask for an enormous allocation.
				aff.CompoundRule = make([]string, 0, int(min(max(val, 0), maxCompoundRules)))
			} else {
				aff.CompoundRule = append(aff.CompoundRule, parts[1])
				for _, tok := range aff.compoundRuleTokens(parts[1]) {
					if _, ok := aff.compoundMap[tok.flag]; tok.flag != "" && !ok {
						aff.compoundMap[tok.flag] = []string{}
					}
				}
			}
		case "NOSUGGEST":
			if len(parts) < 2 {
				return nil, fmt.Errorf("NOSUGGEST stanza had %d fields, expected 2", len(parts))
			}
			aff.NoSuggestFlag = aff.singleFlag(parts[1])
		case "FORBIDDENWORD":
			if len(parts) >= 2 {
				aff.ForbiddenFlag = aff.singleFlag(parts[1])
			}
		case "NEEDAFFIX", "PSEUDOROOT":
			if len(parts) >= 2 {
				aff.NeedAffixFlag = aff.singleFlag(parts[1])
			}
		case "KEEPCASE":
			if len(parts) >= 2 {
				aff.KeepCaseFlag = aff.singleFlag(parts[1])
			}
		case "CIRCUMFIX":
			if len(parts) >= 2 {
				aff.CircumfixFlag = aff.singleFlag(parts[1])
			}
		case "CHECKCOMPOUNDDUP":
			aff.CheckCompoundDup = true
		case "CHECKCOMPOUNDTRIPLE":
			aff.CheckCompoundTriple = true
		case "SIMPLIFIEDTRIPLE":
			aff.SimplifiedTriple = true
		case "CHECKCOMPOUNDCASE":
			aff.CheckCompoundCase = true
		case "CHECKCOMPOUNDREP":
			aff.CheckCompoundRep = true
		case "COMPOUNDWORDMAX":
			if len(parts) >= 2 {
				if val, err := strconv.Atoi(parts[1]); err == nil && val > 0 {
					aff.CompoundWordMax = val
				}
			}
		case "COMPOUNDSYLLABLE":
			if len(parts) >= 3 {
				if val, err := strconv.Atoi(parts[1]); err == nil && val > 0 {
					aff.CompoundSyllable = val
					aff.CompoundVowels = parts[2]
				}
			}
		case "CHECKCOMPOUNDPATTERN":
			// The first line is a count.
			if len(parts) == 2 && allDigits(parts[1]) {
				continue
			}
			if len(parts) < 3 {
				return nil, fmt.Errorf("CHECKCOMPOUNDPATTERN stanza had %d fields, expected 3", len(parts))
			}
			var p compoundPattern
			p.end, p.endFlag, p.stemOnly = parsePatternSide(parts[1])
			p.begin, p.beginFlag, _ = parsePatternSide(parts[2])
			if len(parts) > 3 {
				p.repl = parts[3]
			}
			aff.CompoundPatterns = append(aff.CompoundPatterns, p)
		case "AF":
			if len(parts) < 2 {
				return nil, fmt.Errorf("AF stanza had %d fields, expected 2", len(parts))
			}
			// The first AF line is a count, which only preallocates.
			if !sawAliasCount && allDigits(parts[1]) {
				sawAliasCount = true
				continue
			}
			aff.Aliases = append(aff.Aliases, parts[1])
		case "COMPOUNDFLAG":
			if len(parts) >= 2 {
				aff.CompoundFlag = aff.singleFlag(parts[1])
			}
		case "COMPOUNDBEGIN":
			if len(parts) >= 2 {
				aff.CompoundBegin = aff.singleFlag(parts[1])
			}
		case "COMPOUNDMIDDLE":
			if len(parts) >= 2 {
				aff.CompoundMiddle = aff.singleFlag(parts[1])
			}
		case "COMPOUNDEND":
			if len(parts) >= 2 {
				aff.CompoundEnd = aff.singleFlag(parts[1])
			}
		case "COMPOUNDPERMITFLAG":
			if len(parts) >= 2 {
				aff.CompoundPermitFlag = aff.singleFlag(parts[1])
			}
		case "COMPOUNDFORBIDFLAG":
			if len(parts) >= 2 {
				aff.CompoundForbidFlag = aff.singleFlag(parts[1])
			}
		case "FORCEUCASE":
			if len(parts) >= 2 {
				aff.ForceUCaseFlag = aff.singleFlag(parts[1])
			}
		case "WORDCHARS":
			if len(parts) < 2 {
				return nil, fmt.Errorf("WORDCHAR stanza had %d fields, expected 2", len(parts))
			}
			aff.WordChars = parts[1]
		case "BREAK":
			if len(parts) < 2 {
				return nil, fmt.Errorf("BREAK stanza had %d fields, expected 2", len(parts))
			}
			// The first BREAK line is a count, which only preallocates; the
			// rest are patterns. See #1165.
			aff.BreakDeclared = true
			if !sawBreakCount && allDigits(parts[1]) {
				sawBreakCount = true
				continue
			}
			if len(aff.Break) < maxBreakRules {
				aff.Break = append(aff.Break, parts[1])
			}
		case "FLAG":
			if len(parts) < 2 {
				return nil, fmt.Errorf("FLAG stanza had %d, expected 1", len(parts))
			}
			aff.Flag = parts[1]
		case "PFX", "SFX":
			atype := Prefix
			if parts[0] == "SFX" {
				atype = Suffix
			}

			sections := len(parts)
			// A header line is `PFX/SFX flag Y|N count`; a rule line is
			// `PFX/SFX flag strip affix [condition]`. They can both have four
			// fields -- some dictionaries (e.g. OpenTaal's Dutch) omit the
			// rule's condition -- so distinguish by the cross-product flag
			// rather than by field count alone. See #776.
			isHeader := sections == 4 &&
				(parts[2] == "Y" || parts[2] == "N") && allDigits(parts[3])
			switch {
			case isHeader:
				cross, err := isCrossProduct(parts[2])
				if err != nil {
					return nil, err
				}
				// this is a new Affix!
				aff.AffixMap[aff.singleFlag(parts[1])] = affix{
					Type:         atype,
					CrossProduct: cross,
				}
			case sections >= 4:
				flag := aff.singleFlag(parts[1])
				a, ok := aff.AffixMap[flag]
				if !ok {
					return nil, fmt.Errorf("got rules for flag %q but no definition", flag)
				}

				strip := ""
				if parts[2] != "0" {
					strip = parts[2]
				}

				// The condition is optional; default to "." (matches anything)
				// when a dictionary omits it. See #776.
				cond := "."
				if sections > 4 {
					cond = parts[4]
				}

				var matcher *regexp.Regexp
				var err error
				if cond != "." {
					pat := conditionPattern(cond)
					if a.Type == Prefix {
						pat = "^" + pat
					} else {
						pat += "$"
					}
					matcher, err = regexp.Compile(pat)
					if err != nil {
						return nil, fmt.Errorf("unable to compile %s", pat)
					}
				}

				// See #499.
				//
				// TODO: Is this safe to do in all cases?
				affixText, cont := parts[3], ""
				if text, flags, found := strings.Cut(affixText, "/"); found {
					// Split off the affix's own continuation flags, e.g. the
					// "/34,22" in `SFX 1 0 t/34,22 e`. Left in place they would
					// be appended to the generated word ("stavet/34,22"), so
					// the real form ("stavet") is never recognized. See #1065.
					//
					// They are kept rather than dropped: the flags name further
					// classes that apply to the form this rule produces, which
					// is how Hunspell builds a word like `stavets` from
					// `stave` in two steps. See expand.
					affixText, cont = text, flags
				}
				if affixText == "0" {
					// A zero affix, which may still carry flags: `0/UPX`.
					affixText = ""
				}

				a.Rules = append(a.Rules, rule{
					Strip:     strip,
					AffixText: affixText,
					Cont:      cont,
					Pattern:   cond,
					matcher:   matcher,
				})
				aff.AffixMap[flag] = a
			}
		default:
			// Hunspell ignores lines that don't start with a directive; a
			// directive it knows and this reader does not is recorded.
			name := parts[0]
			if !strings.HasPrefix(name, "#") && !stringIn(name, aff.Ignored) {
				aff.Ignored = append(aff.Ignored, name)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	aff.buildIndex()
	return &aff, nil
}
