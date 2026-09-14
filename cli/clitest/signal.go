package clitest

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

type FakeSignalNotifier struct {
	sync.Mutex
	t       *testing.T
	ctx     context.Context
	cancel  context.CancelFunc
	signals []os.Signal
	stopped bool
}

func NewFakeSignalNotifier(t *testing.T) *FakeSignalNotifier {
	fsn := &FakeSignalNotifier{t: t}
	return fsn
}

func (f *FakeSignalNotifier) Stop() {
	f.Lock()
	defer f.Unlock()
	f.stopped = true
	if f.cancel == nil {
		f.t.Error("stopped before started")
		return
	}
	f.cancel()
}

func (f *FakeSignalNotifier) NotifyContext(parent context.Context, signals ...os.Signal) (ctx context.Context, stop context.CancelFunc) {
	f.Lock()
	defer f.Unlock()
	f.signals = signals
	f.ctx, f.cancel = context.WithCancel(parent)
	return f.ctx, f.Stop
}

func (f *FakeSignalNotifier) Notify() {
	f.Lock()
	defer f.Unlock()
	if f.cancel == nil {
		f.t.Error("notified before started")
		return
	}
	f.cancel()
}

func (f *FakeSignalNotifier) AssertStopped() {
	f.Lock()
	defer f.Unlock()
	assert.True(f.t, f.stopped)
}
