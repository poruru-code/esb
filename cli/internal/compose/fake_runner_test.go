package compose

import (
	"context"
)

type fakeRunner struct {
	dir      string
	name     string
	args     []string
	lastCall string
}

func (f *fakeRunner) Run(_ context.Context, dir, name string, args ...string) error {
	f.dir = dir
	f.name = name
	f.args = args
	f.lastCall = "run"
	return nil
}

func (f *fakeRunner) RunOutput(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	f.dir = dir
	f.name = name
	f.args = args
	f.lastCall = "runoutput"
	return []byte(""), nil
}

func (f *fakeRunner) RunQuiet(_ context.Context, dir, name string, args ...string) error {
	f.dir = dir
	f.name = name
	f.args = args
	f.lastCall = "runquiet"
	return nil
}
