package lint

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vale-cli/vale/v3/internal/core"
	"github.com/vale-cli/vale/v3/internal/system"
)

func (l *Linter) lintDITA(file *core.File) error {
	var out bytes.Buffer
	var htmlFile string

	dita := system.Which([]string{"dita", "dita.bat"})
	if dita == "" {
		return core.NewE100("lintDITA", errors.New("dita not found"))
	}

	tempDir, err := os.MkdirTemp("", "dita-")
	defer os.RemoveAll(tempDir)

	if err != nil {
		return core.NewE201FromPosition(err.Error(), file.Path, 1)
	}

	// The toolkit reads the file from disk, so ignore patterns are applied
	// to a copy beside it, where its conrefs still resolve.
	input := file.Path
	if s, terr := l.Transform(file); terr != nil {
		return terr
	} else if s != file.Content {
		copied, cerr := os.CreateTemp(filepath.Dir(file.Path), ".vale-*.dita")
		if cerr != nil {
			return core.NewE100(file.Path, cerr)
		}
		defer os.Remove(copied.Name())
		if _, cerr = copied.WriteString(s); cerr != nil {
			return core.NewE100(file.Path, cerr)
		}
		if cerr = copied.Close(); cerr != nil {
			return core.NewE100(file.Path, cerr)
		}
		input = copied.Name()
	}

	// FIXME: The `dita` command is *slow* (~4s per file)!
	cmd := exec.Command(dita, []string{
		"-i",
		input,
		"-f",
		"html5",
		"-o",
		tempDir,
		"--nav-toc=none",
		"--outer.control=quiet", // allows DITA files to reference external files, like in conrefs.
	}...)
	cmd.Stderr = &out

	if err = cmd.Run(); err != nil {
		// dita's own diagnostics are far more useful than the exit status,
		// but it doesn't always write any
		if msg := strings.TrimSpace(out.String()); msg != "" {
			return core.NewE100(file.Path, errors.New(msg))
		}
		return core.NewE100(file.Path, err)
	}

	targetFileName := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input)) + ".html"
	_ = filepath.WalkDir(tempDir, func(fp string, de os.DirEntry, _ error) error {
		// Find .html file, also looking in subdirectories in case an
		// "outer" file was referenced in the DITA file, which is allowed
		// because of the outer.control option of the dita command.
		if de.Name() == targetFileName {
			htmlFile = fp
		}
		return nil
	})

	data, err := os.ReadFile(htmlFile)
	if err != nil {
		return core.NewE100(htmlFile, err)
	}

	// NOTE: We have to remove the `<head>` tag to avoid
	// introducing new content into the HTML.
	head1 := bytes.Index(data, []byte("<head>"))
	head2 := bytes.Index(data, []byte("</head>"))

	if head1 >= 0 && head2 >= 0 {
		data = append(data[:head1], data[head2:]...)
	}

	return l.lintHTMLTokens(file, data, 0)
}
