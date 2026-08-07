package dnfops

import (
	"errors"
	"strings"
	"sync"
)

// fakeRunner records invocations and returns canned responses. Use it
// to assert what commands Veilbox would run without touching the
// system. It is safe for concurrent use.
type fakeRunner struct {
	mu sync.Mutex
	// calls is the ordered list of invocations: name + args.
	calls [][]string
	// errByCmd maps "name arg0 arg1" (joined) to a canned error.
	// Missing keys succeed and return responses below.
	errByCmd map[string]error
	// responses maps the full joined command line to canned output.
	responses map[string]string
	// interactive records RunInteractive invocations.
	interactive [][]string
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		errByCmd:   map[string]error{},
		responses:  map[string]string{},
	}
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string{name}, args...))
	if err := f.errByCmd[key]; err != nil {
		return f.responses[key], err
	}
	return f.responses[key], nil
}

func (f *fakeRunner) RunInteractive(name string, args ...string) error {
	key := strings.Join(append([]string{name}, args...), " ")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.interactive = append(f.interactive, append([]string{name}, args...))
	if err := f.errByCmd[key]; err != nil {
		return err
	}
	return nil
}

// joined returns "name args..." for every recorded invocation.
func (f *fakeRunner) joined() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		out = append(out, strings.Join(c, " "))
	}
	for _, c := range f.interactive {
		out = append(out, "interactive:"+strings.Join(c, " "))
	}
	return out
}

// notInstalled simulates rpm -q reporting a package as missing: exit
// failure plus the canonical "is not installed" diagnostic.
func (f *fakeRunner) notInstalled(pkg string) {
	key := "rpm -q " + pkg
	f.responses[key] = "package " + pkg + " is not installed"
	f.errByCmd[key] = errors.New("exit status 1")
}
