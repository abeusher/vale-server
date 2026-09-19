package lint

import (
	"reflect"
	"testing"
	"time"

	"github.com/vale-cli/vale/v3/internal/nlp"
)

// The hooks observe a run without changing its alerts, including above the
// concurrency floor, where a RuleHook takes the serial path.
func TestHooksObserveWithoutChanging(t *testing.T) {
	withFloor(t, 0) // every block above the floor

	plain, err := demoLinter(t).LintString(sample())
	if err != nil {
		t.Fatal(err)
	}
	want := alertLines(plain)
	if len(want) == 0 {
		t.Fatal("the sample raised no alerts, so this proves nothing")
	}

	linter := demoLinter(t)
	rules := linter.Manager.Rules()

	var blocks int
	linter.BlockHook = func(nlp.Block) { blocks++ }

	timed := map[string]time.Duration{}
	linter.RuleHook = func(name string, took time.Duration) {
		if _, ok := rules[name]; !ok {
			t.Errorf("RuleHook saw %q, which is not a loaded rule", name)
		}
		timed[name] += took
	}

	hooked, err := linter.LintString(sample())
	if err != nil {
		t.Fatal(err)
	}

	if blocks == 0 {
		t.Error("BlockHook saw no blocks")
	}
	if len(timed) == 0 {
		t.Error("RuleHook saw no rules")
	}
	for _, a := range hooked[0].Alerts {
		if _, ok := timed[linter.Manager.RuleForAlert(a.Check)]; !ok {
			t.Errorf("%s raised an alert but RuleHook never saw it", a.Check)
		}
	}
	if got := alertLines(hooked); !reflect.DeepEqual(got, want) {
		t.Errorf("hooks changed the alerts:\n got  %v\n want %v", got, want)
	}
}
