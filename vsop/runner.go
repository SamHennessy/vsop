package vsop

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/pkg/errors"
)

// Possible errors
var (
	ErrStartTimeOut = errors.New("app took too long to start and was killed")
)

type Runner struct {
	bin       string
	args      []string
	writer    io.Writer
	command   *exec.Cmd
	starttime time.Time
	log       LineLogNamespace
}

func NewRunner(bin string, logger LineLogNamespace, args ...string) *Runner {
	return &Runner{
		bin:       bin,
		args:      args,
		writer:    ioutil.Discard,
		starttime: time.Now(),
		log:       logger,
	}
}

func (r *Runner) Run() (*exec.Cmd, error) {
	if r.needsRefresh() {
		r.Kill()
	}

	if r.command == nil || r.Exited() {
		err := r.runBin()
		if err != nil {
			r.log.Err(errors.Wrap(err, "runner run"))
		}
		return r.command, err
	}
	return r.command, nil
}

func (r *Runner) Info() (os.FileInfo, error) {
	return os.Stat(r.bin)
}

// SetWriter for stdout and errout
func (r *Runner) SetWriter(writer io.Writer) {
	r.writer = writer
}

// Kill process
func (r *Runner) Kill() error {
	if r.command != nil && r.command.Process != nil {
		done := make(chan error)
		go func() {
			r.command.Wait()
			close(done)
		}()

		//Trying a "soft" kill first
		if runtime.GOOS == "windows" {
			if err := r.command.Process.Kill(); err != nil {
				return err
			}
		} else if err := r.command.Process.Signal(os.Interrupt); err != nil {
			return err
		}

		//Wait for our process to die before we return or hard kill after 3 sec
		select {
		case <-time.After(3 * time.Second):
			if err := r.command.Process.Kill(); err != nil {
				r.log.Err(errors.Wrap(err, "hard kill process"))
			}
		case <-done:
		}
		r.command = nil
	}

	return nil
}

func (r *Runner) Exited() bool {
	return r.command != nil && r.command.ProcessState != nil && r.command.ProcessState.Exited()
}

func (r *Runner) IsRunning() bool {
	return r.command != nil && !r.Exited()
}

func (r *Runner) runBin() error {
	r.command = exec.Command(r.bin, r.args...)
	r.command.Stdout = r.writer
	r.command.Stderr = r.writer

	err := r.command.Start()
	if err != nil {
		return err
	}

	r.starttime = time.Now()

	go r.command.Wait()

	return nil
}

func (r *Runner) needsRefresh() bool {
	info, err := r.Info()
	if err != nil {
		return false
	} else {
		return info.ModTime().After(r.starttime)
	}
}

func (r *Runner) Command() *exec.Cmd {
	return r.command
}
