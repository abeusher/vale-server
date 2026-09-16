package spell

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
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
}

// derive applies the affix's rules to base, one form per rule that matches.
func (a dictConfig) derive(af affix, base entryForm) []entryForm {
	var out []entryForm
	for i := range af.Rules {
		r := &af.Rules[i]
		if r.matcher != nil && !r.matcher.MatchString(base.Word) {
			continue
		}
		f := base
		f.cont = nil
		if r.Cont != "" {
			f.cont = a.parseFlags(r.Cont)
			f.Flags = union(base.Flags, f.cont)
		}
		if af.Type == Prefix {
			f.Word = r.AffixText + base.Word
			f.prefix = r
			// TODO is does Strip apply to prefixes too?
		} else {
			stripped := base.Word
			if r.Strip != "" && strings.HasSuffix(stripped, r.Strip) {
				stripped = stripped[:len(stripped)-len(r.Strip)]
			}
			f.Word = stripped + r.AffixText
			f.suffix = r
		}
		out = append(out, f)
	}
	return out
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

	// Ignored names the directives the file used that this reader does not
	// implement, in the order they were first seen.
	Ignored []string
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
	default: // "ASCII" or "UTF-8"
		flags := make([]string, 0, len(flagStr))
		for _, r := range flagStr {
			flags = append(flags, string(r))
		}
		return flags
	}
}

// expand returns the words a `.dic` entry generates.
func (a dictConfig) expand(entry string, out []string) ([]string, error) {
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
func (a dictConfig) expandEntry(entry string) ([]entryForm, error) {
	word, keyString, found := strings.Cut(entry, "/")
	if !found {
		return []entryForm{{Word: entry}}, nil
	}
	if word == "" || keyString == "" {
		return nil, fmt.Errorf("slash char found in first or last position")
	}

	flags := a.parseFlags(keyString)
	for _, key := range flags {
		if _, ok := a.compoundMap[key]; ok {
			a.compoundMap[key] = append(a.compoundMap[key], word)
		}
	}

	stem := entryForm{Word: word, Flags: flags, cont: flags}
	return a.emit(stem, 0), nil
}

// emit returns f, unless its own flags say it needs an affix, and then the
// forms the affixes those flags name build on it.
func (a dictConfig) emit(f entryForm, depth int) []entryForm {
	var out []entryForm
	if !hasFlag(f.cont, a.NeedAffixFlag) {
		out = append(out, f)
	}
	if depth > maxAffixDepth {
		return out
	}
	return append(out, a.affixed(f, f.cont, depth)...)
}

// affixed builds the forms the affixes named by keys make from base.
func (a dictConfig) affixed(base entryForm, keys []string, depth int) []entryForm {
	var out []entryForm
	prefixes := make([]affix, 0, 5)
	suffixes := make([]affix, 0, 5)
	for _, key := range keys {
		af, ok := a.AffixMap[key]
		if !ok {
			continue
		}
		if !af.CrossProduct {
			out = a.emitAll(a.alone(a.derive(af, base)), out, depth)
			continue
		}
		if af.Type == Prefix {
			prefixes = append(prefixes, af)
		} else {
			suffixes = append(suffixes, af)
		}
	}

	for _, suf := range suffixes {
		out = a.emitAll(a.alone(a.derive(suf, base)), out, depth)
	}
	for _, pre := range prefixes {
		prefixed := a.derive(pre, base)
		out = a.emitAll(a.alone(prefixed), out, depth)

		// now do cross product
		for _, suf := range suffixes {
			for _, pw := range prefixed {
				out = a.emitAll(a.paired(pw, a.derive(suf, pw)), out, depth)
			}
		}
	}
	return out
}

// emitAll emits each form, following its continuation flags one level down.
//
// This is the step Hunspell calls twofold affixation: `SFX 1 0 t/34,22 e`
// says that after the rule builds its form, classes 34 and 22 apply to that.
func (a dictConfig) emitAll(forms []entryForm, out []entryForm, depth int) []entryForm {
	for _, f := range forms {
		out = append(out, a.emit(f, depth+1)...)
	}
	return out
}

// circumfix reports whether a form's rule carries the CIRCUMFIX flag.
func (a dictConfig) circumfix(f entryForm) bool {
	return hasFlag(f.cont, a.CircumfixFlag)
}

// alone drops the forms that need a partner, when the affix is used by itself.
func (a dictConfig) alone(forms []entryForm) []entryForm {
	if a.CircumfixFlag == "" {
		return forms
	}
	kept := make([]entryForm, 0, len(forms))
	for _, f := range forms {
		if !a.circumfix(f) {
			kept = append(kept, f)
		}
	}
	return kept
}

// paired keeps the suffixed forms of a prefixed word whose two halves agree
// on CIRCUMFIX: both carry it, or neither does.
func (a dictConfig) paired(pre entryForm, forms []entryForm) []entryForm {
	if a.CircumfixFlag == "" {
		return forms
	}
	want := a.circumfix(pre)
	kept := make([]entryForm, 0, len(forms))
	for _, f := range forms {
		if a.circumfix(f) == want {
			kept = append(kept, f)
		}
	}
	return kept
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
	upper              bool // FORCEUCASE: a compound ending here is capitalized
}

// compoundUse reports whether f may be a compound segment, and where.
func (a dictConfig) compoundUse(f entryForm) (segment, bool) {
	if hasFlag(f.Flags, a.CompoundForbidFlag) {
		return segment{}, false
	}
	anywhere := hasFlag(f.Flags, a.CompoundFlag)
	seg := segment{
		begin:  anywhere || hasFlag(f.Flags, a.CompoundBegin),
		middle: anywhere || hasFlag(f.Flags, a.CompoundMiddle),
		end:    anywhere || hasFlag(f.Flags, a.CompoundEnd),
		upper:  hasFlag(f.Flags, a.ForceUCaseFlag),
	}
	// An affix keeps its form at the compound's edge unless it permits more.
	if f.prefix != nil && !hasFlag(a.parseFlags(f.prefix.Cont), a.CompoundPermitFlag) {
		seg.middle, seg.end = false, false
	}
	if f.suffix != nil && !hasFlag(a.parseFlags(f.suffix.Cont), a.CompoundPermitFlag) {
		seg.begin, seg.middle = false, false
	}
	return seg, seg.begin || seg.middle || seg.end
}

// hasFlag reports whether flag is set and among flags.
func hasFlag(flags []string, flag string) bool {
	return flag != "" && stringIn(flag, flags)
}

// entryHas reports whether a `.dic` entry carries flag.
func (a dictConfig) entryHas(entry, flag string) bool {
	if flag == "" {
		return false
	}
	_, flags, found := strings.Cut(entry, "/")
	return found && hasFlag(a.parseFlags(flags), flag)
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

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		switch parts[0] {
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
			aff.CompoundOnly = parts[1]
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
				for _, flag := range aff.parseFlags(parts[1]) {
					if _, ok := aff.compoundMap[flag]; !ok {
						aff.compoundMap[flag] = []string{}
					}
				}
			}
		case "NOSUGGEST":
			if len(parts) < 2 {
				return nil, fmt.Errorf("NOSUGGEST stanza had %d fields, expected 2", len(parts))
			}
			aff.NoSuggestFlag = parts[1]
		case "FORBIDDENWORD":
			if len(parts) >= 2 {
				aff.ForbiddenFlag = parts[1]
			}
		case "NEEDAFFIX", "PSEUDOROOT":
			if len(parts) >= 2 {
				aff.NeedAffixFlag = parts[1]
			}
		case "KEEPCASE":
			if len(parts) >= 2 {
				aff.KeepCaseFlag = parts[1]
			}
		case "CIRCUMFIX":
			if len(parts) >= 2 {
				aff.CircumfixFlag = parts[1]
			}
		case "CHECKCOMPOUNDDUP":
			aff.CheckCompoundDup = true
		case "CHECKCOMPOUNDTRIPLE":
			aff.CheckCompoundTriple = true
		case "SIMPLIFIEDTRIPLE":
			aff.SimplifiedTriple = true
		case "CHECKCOMPOUNDCASE":
			aff.CheckCompoundCase = true
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
				aff.CompoundFlag = parts[1]
			}
		case "COMPOUNDBEGIN":
			if len(parts) >= 2 {
				aff.CompoundBegin = parts[1]
			}
		case "COMPOUNDMIDDLE":
			if len(parts) >= 2 {
				aff.CompoundMiddle = parts[1]
			}
		case "COMPOUNDEND":
			if len(parts) >= 2 {
				aff.CompoundEnd = parts[1]
			}
		case "COMPOUNDPERMITFLAG":
			if len(parts) >= 2 {
				aff.CompoundPermitFlag = parts[1]
			}
		case "COMPOUNDFORBIDFLAG":
			if len(parts) >= 2 {
				aff.CompoundForbidFlag = parts[1]
			}
		case "FORCEUCASE":
			if len(parts) >= 2 {
				aff.ForceUCaseFlag = parts[1]
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
				aff.AffixMap[parts[1]] = affix{
					Type:         atype,
					CrossProduct: cross,
				}
			case sections >= 4:
				flag := parts[1]
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
					pat := cond
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
				if affixText == "0" {
					affixText = ""
				} else if text, flags, found := strings.Cut(affixText, "/"); found {
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

	return &aff, nil
}
