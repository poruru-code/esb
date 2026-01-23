package compose

import (
	"context"
)

type fakeRunner struct {
	dir  string
	name string
	args []string
}

func (f *fakeRunner) Run(_ context.Context, dir, name string, args ...string) error {
	f.dir = dir
	f.name = name
	f.args = args
	return nil
}

func (f *fakeRunner) RunOutput(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	f.dir = dir
	f.name = name
	f.args = args
	return []byte(""), nil
}

func (f *fakeRunner) RunQuiet(ctx context.Context, dir, name string, args ...string) error {
	return f.Run(ctx, dir, name, args...)
}
