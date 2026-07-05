package forwarder

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/tbxark/at-message-forward/internal/config"
)

// fakeTransport is a stand-in transport for exercising the session/reconnect
// paths without real hardware. This is exactly the seam an integration test or
// a local mock would plug into.
type fakeTransport struct {
	opens int
	link  io.ReadWriteCloser
	name  string
	err   error
}

func (f *fakeTransport) Open(context.Context) (io.ReadWriteCloser, string, error) {
	f.opens++
	return f.link, f.name, f.err
}

func TestRunModemSessionWrapsOpenError(t *testing.T) {
	wantErr := errors.New("boom")
	tr := &fakeTransport{err: wantErr}
	err := runModemSession(context.Background(), config.Config{}, tr, &reconnectableExecutor{}, nil)
	if tr.opens != 1 {
		t.Fatalf("opens = %d, want 1", tr.opens)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestReconnectLoopRetriesAfterClosedSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runs := 0
	waits := 0
	errClosed := errors.New("closed")
	err := runReconnectLoopWithOptions(ctx, reconnectLoopOptions{
		initialBackoff: time.Second,
		maxBackoff:     4 * time.Second,
		runSession: func(context.Context) error {
			runs++
			if runs == 2 {
				cancel()
			}
			return errClosed
		},
		wait: func(context.Context, time.Duration) error {
			waits++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runReconnectLoopWithOptions returned error: %v", err)
	}
	if runs != 2 {
		t.Fatalf("runs = %d, want 2", runs)
	}
	if waits != 1 {
		t.Fatalf("waits = %d, want 1", waits)
	}
}

func TestReconnectLoopRespectsContextCancellationDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runs := 0
	err := runReconnectLoopWithOptions(ctx, reconnectLoopOptions{
		initialBackoff: time.Second,
		maxBackoff:     4 * time.Second,
		runSession: func(context.Context) error {
			runs++
			return errSerialSessionClosed
		},
		wait: func(context.Context, time.Duration) error {
			cancel()
			return context.Canceled
		},
	})
	if err != nil {
		t.Fatalf("runReconnectLoopWithOptions returned error: %v", err)
	}
	if runs != 1 {
		t.Fatalf("runs = %d, want 1", runs)
	}
}

func TestNextBackoffCapsAtMax(t *testing.T) {
	if got := nextBackoff(2*time.Second, 30*time.Second); got != 4*time.Second {
		t.Fatalf("nextBackoff = %s, want 4s", got)
	}
	if got := nextBackoff(20*time.Second, 30*time.Second); got != 30*time.Second {
		t.Fatalf("nextBackoff = %s, want 30s", got)
	}
	if got := nextBackoff(30*time.Second, 30*time.Second); got != 30*time.Second {
		t.Fatalf("nextBackoff = %s, want 30s", got)
	}
}

func TestReconnectLoopResetsBackoffAfterConnectedSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := []error{errors.New("open failed"), errors.New("init failed"), errSerialSessionClosed, errors.New("open failed again")}
	var waits []time.Duration
	err := runReconnectLoopWithOptions(ctx, reconnectLoopOptions{
		initialBackoff: time.Second,
		maxBackoff:     4 * time.Second,
		runSession: func(context.Context) error {
			err := errs[0]
			errs = errs[1:]
			if len(errs) == 0 {
				cancel()
			}
			return err
		},
		wait: func(_ context.Context, d time.Duration) error {
			waits = append(waits, d)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runReconnectLoopWithOptions returned error: %v", err)
	}
	want := []time.Duration{time.Second, 2 * time.Second, time.Second}
	if len(waits) != len(want) {
		t.Fatalf("waits = %v, want %v", waits, want)
	}
	for i := range want {
		if waits[i] != want[i] {
			t.Fatalf("waits = %v, want %v", waits, want)
		}
	}
}
