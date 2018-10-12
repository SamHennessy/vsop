package vsop

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"
)

type Builder struct {
	dir       string
	binary    string
	errors    string
	useGodep  bool
	wd        string
	buildArgs []string
}

func NewBuilder(dir string, bin string, useGodep bool, wd string, buildArgs []string) *Builder {
	if len(bin) == 0 {
		bin = "bin"
	}

	// does not work on Windows without the ".exe" extension
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(bin, ".exe") { // check if it already has the .exe extension
			bin += ".exe"
		}
	}

	return &Builder{dir: dir, binary: bin, useGodep: useGodep, wd: wd, buildArgs: buildArgs}
}

func (b *Builder) Binary() string {
	return b.binary
}

func (b *Builder) Errors() string {
	return b.errors
}

func (b *Builder) Build() error {
	args := append([]string{"go", "build", "-o", filepath.Join(b.wd, b.binary)}, b.buildArgs...)

	var command *exec.Cmd
	if b.useGodep {
		args = append([]string{"godep"}, args...)
	}
	command = exec.Command(args[0], args[1:]...)

	command.Dir = b.dir

	output, err := command.CombinedOutput()

	if command.ProcessState.Success() {
		b.errors = ""
	} else {
		b.errors = string(output)
	}

	if len(b.errors) > 0 {
		return fmt.Errorf(b.errors)
	}

	return err
}

// DepEnsure runs dep ensure in the working directory
func (b *Builder) DepEnsure() error {
	var command *exec.Cmd
	command = exec.Command("dep", "ensure")
	output, err := command.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, "builder dep ensure")
	}

	if command.ProcessState.Success() {
		b.errors = ""
	} else {
		b.errors = string(output)
	}

	if len(b.errors) > 0 {
		return fmt.Errorf(b.errors)
	}
	return err
}
