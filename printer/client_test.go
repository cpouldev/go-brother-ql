package printer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// Scripted transport exercises the real session and framing, without USB hardware.
type scriptTransport struct {
	mu         sync.Mutex
	input      chan []byte
	pending    []byte
	written    bytes.Buffer
	writeFn    func(context.Context, []byte) (int, error)
	status     bool
	closed     bool
	readEvents chan struct{}
}

func newScript() *scriptTransport {
	return &scriptTransport{input: make(chan []byte, 20), status: true}
}
func (s *scriptTransport) Read(ctx context.Context, p []byte) (int, error) {
	if s.readEvents != nil {
		s.readEvents <- struct{}{}
	}
	if len(s.pending) == 0 {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case s.pending = <-s.input:
			if s.pending == nil {
				return 0, io.EOF
			}
		}
	}
	n := copy(p, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}

func TestCoolingPausesPartialWrites(t *testing.T) {
	s := newScript()
	s.readEvents = make(chan struct{}, 10)
	s.input <- statusFrame(0, 0, 0)
	firstWritten := make(chan struct{})
	secondAttempt := make(chan struct{})
	writes := 0
	s.writeFn = func(ctx context.Context, p []byte) (int, error) {
		if bytes.Equal(p, []byte{0x1b, 0x69, 0x53}) {
			return len(p), nil
		}
		writes++
		if writes == 1 {
			s.input <- statusFrame(StatusNotification, 1, CoolingStarted)
			// The third read starts after preflight and processing cooling-start.
			for range 3 {
				select {
				case <-s.readEvents:
				case <-ctx.Done():
					return 0, ctx.Err()
				}
			}
			close(firstWritten)
			return 1, nil
		}
		close(secondAttempt)
		s.input <- statusFrame(StatusCompleted, 0, 0)
		return len(p), nil
	}
	c := NewClient(s)
	defer c.Close()
	type answer struct {
		result Result
		err    error
	}
	done := make(chan answer, 1)
	go func() {
		r, e := c.Print(timeout(t), bytes.NewBufferString("abcdef"), PrintOptions{Wait: true})
		done <- answer{r, e}
	}()
	<-firstWritten
	wroteDuringCooling := false
	select {
	case <-secondAttempt:
		wroteDuringCooling = true
	case <-time.After(25 * time.Millisecond):
	}
	s.input <- statusFrame(StatusNotification, 1, CoolingFinished)
	a := <-done
	if wroteDuringCooling {
		t.Fatal("continued transmitting after cooling-start notification")
	}
	if a.err != nil || a.result.Outcome != "printed" || a.result.BytesSent != 6 {
		t.Fatalf("result=%+v error=%v", a.result, a.err)
	}
}

func TestPrinterErrorCancelsBlockedWrite(t *testing.T) {
	s := newScript()
	s.input <- statusFrame(0, 0, 0)
	s.writeFn = func(ctx context.Context, p []byte) (int, error) {
		if bytes.Equal(p, []byte{0x1b, 0x69, 0x53}) {
			return len(p), nil
		}
		b := statusFrame(StatusError, 1, 0)
		b[9] = 16
		s.input <- b
		<-ctx.Done()
		return 2, ctx.Err()
	}
	c := NewClient(s)
	defer c.Close()
	r, err := c.Print(timeout(t), bytes.NewBufferString("abcdef"), PrintOptions{Wait: true})
	var statusErr *StatusErrorDetail
	if !errors.As(err, &statusErr) || r.Outcome != "failed" || r.BytesSent != 2 {
		t.Fatalf("result=%+v error=%v", r, err)
	}
}

func TestIdleDoesNotConfirmPrinting(t *testing.T) {
	s := newScript()
	s.input <- statusFrame(0, 0, 0)
	s.writeFn = func(ctx context.Context, p []byte) (int, error) {
		if string(p) == "job" {
			s.input <- statusFrame(StatusPhaseChange, 0, 0)
			s.input <- nil
		}
		return len(p), nil
	}
	c := NewClient(s)
	defer c.Close()
	r, err := c.Print(timeout(t), bytes.NewBufferString("job"), PrintOptions{Wait: true})
	if err == nil || r.Outcome != "unknown" || r.PagesPrinted != 0 {
		t.Fatalf("result=%+v error=%v", r, err)
	}
}

func TestTruncatedStatus(t *testing.T) {
	s := newScript()
	s.input <- statusFrame(0, 0, 0)[:12]
	s.input <- nil
	c := NewClient(s)
	defer c.Close()
	if _, err := c.Status(timeout(t)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error=%v", err)
	}
}

func TestInterruptedStatusPoisonsConnection(t *testing.T) {
	s := newScript()
	s.input <- statusFrame(0, 0, 0)[:4]
	c := NewClient(s)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c.Status(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("status error=%v", err)
	}
	before := len(s.output())
	_, err := c.Print(timeout(t), bytes.NewBufferString("new job"), PrintOptions{})
	if !errors.Is(err, ErrUnusable) || len(s.output()) != before {
		t.Fatalf("error=%v writes=%x", err, s.output())
	}
}

func TestUnconfirmedPrintPoisonsConnection(t *testing.T) {
	s := newScript()
	s.input <- statusFrame(0, 0, 0)
	c := NewClient(s)
	defer c.Close()
	if r, err := c.Print(timeout(t), bytes.NewBufferString("job"), PrintOptions{}); err != nil || r.Outcome != "sent" {
		t.Fatalf("result=%+v error=%v", r, err)
	}
	before := len(s.output())
	if _, err := c.Status(timeout(t)); !errors.Is(err, ErrUnusable) {
		t.Fatalf("status error=%v", err)
	}
	if len(s.output()) != before {
		t.Fatal("unconfirmed connection was reused")
	}
}

func TestQueuedJobCancellation(t *testing.T) {
	s := newScript()
	s.status = false
	entered := make(chan struct{})
	release := make(chan struct{})
	s.writeFn = func(ctx context.Context, p []byte) (int, error) {
		close(entered)
		select {
		case <-release:
			return len(p), nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	c := NewClient(s)
	defer c.Close()
	done := make(chan error, 1)
	go func() { _, err := c.Print(timeout(t), bytes.NewBufferString("first"), PrintOptions{}); done <- err }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := c.Print(ctx, bytes.NewBufferString("second"), PrintOptions{})
	close(release)
	firstErr := <-done
	if !errors.Is(err, context.Canceled) || r.BytesSent != 0 || firstErr != nil {
		t.Fatalf("result=%+v error=%v first error=%v", r, err, firstErr)
	}
}
func (s *scriptTransport) Write(ctx context.Context, p []byte) (int, error) {
	if s.writeFn != nil {
		return s.writeFn(ctx, p)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.written.Write(p)
}
func (s *scriptTransport) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}
func (s *scriptTransport) SupportsStatus() bool { return s.status }
func (s *scriptTransport) output() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return bytes.Clone(s.written.Bytes())
}
func timeout(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestStatusFramesFragmented(t *testing.T) {
	s := newScript()
	frame := statusFrame(0, 0, 0)
	s.input <- frame[:1]
	s.input <- frame[1:19]
	s.input <- frame[19:]
	c := NewClient(s)
	defer c.Close()
	got, err := c.Status(timeout(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.MediaWidth != 62 || !bytes.Equal(s.output(), []byte{0x1b, 0x69, 0x53}) {
		t.Fatalf("status=%+v writes=%x", got, s.output())
	}
}

func TestPrintCountsCoalescedCompletions(t *testing.T) {
	s := newScript()
	s.input <- statusFrame(0, 0, 0)
	s.writeFn = func(ctx context.Context, p []byte) (int, error) {
		s.mu.Lock()
		s.written.Write(p)
		s.mu.Unlock()
		if bytes.Equal(p, []byte("job")) {
			s.input <- append(statusFrame(1, 1, 0), statusFrame(1, 0, 0)...)
		}
		return len(p), nil
	}
	c := NewClient(s)
	defer c.Close()
	got, err := c.Print(timeout(t), bytes.NewBufferString("job"), PrintOptions{Wait: true, Pages: 2, Media: &Media{Width: 62, Type: 0x0a}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != "printed" || got.BytesSent != 3 || got.PagesPrinted != 2 {
		t.Fatalf("result=%+v", got)
	}
}

func TestPrintRejectsMediaBeforeJob(t *testing.T) {
	s := newScript()
	s.input <- statusFrame(0, 0, 0)
	c := NewClient(s)
	defer c.Close()
	got, err := c.Print(timeout(t), bytes.NewBufferString("job"), PrintOptions{Media: &Media{Width: 29, Type: 0x0a}})
	if !errors.Is(err, ErrMediaMismatch) || got.BytesSent != 0 || bytes.Contains(s.output(), []byte("job")) {
		t.Fatalf("result=%+v error=%v writes=%x", got, err, s.output())
	}
}

func TestPrintRejectsNoMedia(t *testing.T) {
	s := newScript()
	frame := statusFrame(0, 0, 0)
	frame[10], frame[11] = 0, 0
	s.input <- frame
	c := NewClient(s)
	defer c.Close()
	r, err := c.Print(timeout(t), bytes.NewBufferString("job"), PrintOptions{})
	if err == nil || r.BytesSent != 0 {
		t.Fatalf("result=%+v error=%v", r, err)
	}
}

func TestPrintRejectsUnknownMediaType(t *testing.T) {
	s := newScript()
	frame := statusFrame(0, 0, 0)
	frame[11] = 0x8a
	s.input <- frame
	c := NewClient(s)
	defer c.Close()
	r, err := c.Print(timeout(t), bytes.NewBufferString("job"), PrintOptions{Media: &Media{Width: 62, Type: 0x0a}})
	if !errors.Is(err, ErrMediaMismatch) || r.BytesSent != 0 {
		t.Fatalf("result=%+v error=%v", r, err)
	}
}

type sourceErrorReader struct {
	data []byte
	err  error
}

func (r *sourceErrorReader) Read(p []byte) (int, error) { return copy(p, r.data), r.err }
func TestPrintPreservesSourceError(t *testing.T) {
	s := newScript()
	s.status = false
	c := NewClient(s)
	defer c.Close()
	broken := errors.New("source is truncated")
	r, err := c.Print(timeout(t), &sourceErrorReader{data: []byte("partial"), err: broken}, PrintOptions{})
	if !errors.Is(err, broken) || r.BytesSent != 7 || r.Outcome != "unknown" {
		t.Fatalf("result=%+v error=%v", r, err)
	}
}

func TestPrintPartialWriteNotReplayed(t *testing.T) {
	s := newScript()
	s.status = false
	calls := 0
	broken := errors.New("disconnected")
	s.writeFn = func(context.Context, []byte) (int, error) { calls++; return 2, broken }
	c := NewClient(s)
	defer c.Close()
	got, err := c.Print(timeout(t), bytes.NewBufferString("abcdef"), PrintOptions{})
	if !errors.Is(err, broken) || got.BytesSent != 2 || got.Outcome != "unknown" || calls != 1 {
		t.Fatalf("result=%+v err=%v calls=%d", got, err, calls)
	}
}

func TestPrintShortWriteContinues(t *testing.T) {
	s := newScript()
	s.status = false
	s.writeFn = func(ctx context.Context, p []byte) (int, error) {
		if len(p) > 2 {
			p = p[:2]
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.written.Write(p)
	}
	c := NewClient(s)
	defer c.Close()
	got, err := c.Print(timeout(t), bytes.NewBufferString("abcdef"), PrintOptions{})
	if err != nil || got.BytesSent != 6 || got.Outcome != "sent" || string(s.output()) != "abcdef" {
		t.Fatalf("result=%+v err=%v bytes=%q", got, err, s.output())
	}
}

func TestPrintUnsupportedWaitWritesNothing(t *testing.T) {
	s := newScript()
	s.status = false
	c := NewClient(s)
	defer c.Close()
	got, err := c.Print(timeout(t), bytes.NewBufferString("job"), PrintOptions{Wait: true})
	if !errors.Is(err, ErrStatusUnsupported) || got.BytesSent != 0 || len(s.output()) != 0 {
		t.Fatalf("result=%+v err=%v", got, err)
	}
}

func TestPrintMissingCompletionIsUnknown(t *testing.T) {
	s := newScript()
	s.input <- statusFrame(0, 0, 0)
	s.writeFn = func(ctx context.Context, p []byte) (int, error) {
		if string(p) == "job" {
			s.input <- nil
		}
		return len(p), nil
	}
	c := NewClient(s)
	defer c.Close()
	got, err := c.Print(timeout(t), bytes.NewBufferString("job"), PrintOptions{Wait: true})
	if !errors.Is(err, io.EOF) || got.Outcome != "unknown" || got.BytesSent != 3 {
		t.Fatalf("result=%+v err=%v", got, err)
	}
}

func TestPrintRejectsPrinterError(t *testing.T) {
	s := newScript()
	b := statusFrame(0, 0, 0)
	b[9] = 16
	s.input <- b
	c := NewClient(s)
	defer c.Close()
	got, err := c.Print(timeout(t), bytes.NewBufferString("job"), PrintOptions{Wait: true})
	if err == nil || got.Outcome != "failed" || got.BytesSent != 0 {
		t.Fatalf("result=%+v err=%v", got, err)
	}
}

func TestCloseCancelsPendingStatus(t *testing.T) {
	s := newScript()
	c := NewClient(s)
	started := make(chan struct{})
	s.writeFn = func(ctx context.Context, p []byte) (int, error) { close(started); return len(p), nil }
	done := make(chan error, 1)
	go func() { _, err := c.Status(context.Background()); done <- err }()
	<-started
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("missing cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("close left status blocked")
	}
}

func TestPrintReportsStatusBeforeTerminalError(t *testing.T) {
	s := newScript()
	s.input <- statusFrame(StatusReply, 0, 0)
	s.writeFn = func(ctx context.Context, data []byte) (int, error) {
		if bytes.Equal(data, []byte{0x1b, 0x69, 0x53}) {
			return len(data), nil
		}
		s.input <- statusFrame(StatusNotification, 1, CoolingStarted)
		s.input <- statusFrame(StatusNotification, 1, CoolingFinished)
		s.input <- statusFrame(StatusCompleted, 0, 0)
		fault := statusFrame(StatusError, 1, 0)
		fault[9] = 16
		s.input <- fault
		<-ctx.Done()
		return 2, ctx.Err()
	}
	c := NewClient(s)
	defer c.Close()
	var statuses []Status
	r, err := c.Print(timeout(t), bytes.NewBufferString("job"), PrintOptions{
		Wait: true, Pages: 2, OnStatus: func(s Status) { statuses = append(statuses, s) },
	})
	if err == nil || r.BytesSent != 2 || r.PagesPrinted != 1 {
		t.Fatalf("result=%+v error=%v", r, err)
	}
	if len(statuses) != 5 {
		t.Fatalf("status history has %d frames, want all 5 including cooling before failure", len(statuses))
	}
	if statuses[1].Notification != CoolingStarted || statuses[2].Notification != CoolingFinished || statuses[4].Raw[9] != 16 {
		t.Fatalf("status sequence: %+v", statuses)
	}
}
