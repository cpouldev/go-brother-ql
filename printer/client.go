package printer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Transport must allow a Read and Write concurrently. Both operations must
// return promptly on context cancellation and accurately report partial I/O.
// A Client serializes complete jobs on its transport and owns its lifetime.
type Transport interface {
	Read(context.Context, []byte) (int, error)
	Write(context.Context, []byte) (int, error)
	Close() error
	SupportsStatus() bool
}

// Media describes the expected stock for a preflight check. Width and Length are
// millimetres; Length is zero for continuous media. Type is 0x0a for continuous
// or 0x0b for die-cut media; the QL-800 series values 0x4a and 0x4b are also accepted.
type Media struct {
	Width  int  `json:"width"`
	Length int  `json:"length"`
	Type   byte `json:"type"`
}

// PrintOptions controls completion confirmation and expected-media checks.
type PrintOptions struct {
	// Wait requires one completion response per page. An idle status alone is
	// insufficient. The default is false. Transports without status support
	// reject Wait before sending any job data.
	Wait bool
	// Pages is the actual job page count when waiting for completion. Zero
	// defaults to one; negative values are invalid.
	Pages int
	// Media is checked before sending on bidirectional transports. TCP cannot
	// preflight media. Both old and new Brother media-type encodings are accepted.
	// Nil skips the expected-media match, but readiness and media presence are
	// still checked on transports that support status.
	Media *Media
}

// Result describes delivery and any observed printer completion. Print returns
// it even on error; a failed or unknown job may have produced physical output.
type Result struct {
	// Outcome is "sent" after delivery without confirmed completion, or
	// "printed" after the requested number of completion responses. "failed"
	// means failure before delivery or an explicit printer error; it does not
	// imply that no pages printed. "unknown" means some bytes were sent before
	// another error, such as a timeout or lost connection.
	Outcome string `json:"outcome"`
	// BytesSent counts job bytes accepted by the transport, excluding status
	// requests. It does not prove that those bytes reached the print head.
	BytesSent int64 `json:"bytes_sent"`
	// PagesPrinted counts completion responses observed during the job.
	PagesPrinted int     `json:"pages_printed"`
	LastStatus   *Status `json:"last_status,omitempty"`
}

// Client owns a transport and serializes Print and Status operations on it.
// Construct it with Open or NewClient; the zero value is not usable. A Client
// must not be copied after first use. Close it when finished, including after
// ErrUnusable, and create a new client to resume communication.
type Client struct {
	transport Transport
	gate      chan struct{}
	life      context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
	closeErr  error
	unusable  error
	// Only the holder of gate accesses framing; a Print receiver is joined
	// before gate is released.
	pending []byte
	readErr error
}

// NewClient takes ownership of t without performing printer I/O. The transport
// must satisfy the concurrency and cancellation requirements of Transport.
func NewClient(t Transport) *Client {
	life, cancel := context.WithCancel(context.Background())
	c := &Client{transport: t, gate: make(chan struct{}, 1), life: life, cancel: cancel}
	c.gate <- struct{}{}
	return c
}

func (c *Client) acquire(ctx context.Context) (context.Context, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if c.life.Err() != nil {
		return nil, nil, ErrClosed
	}
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-c.life.Done():
		return nil, nil, ErrClosed
	case <-c.gate:
	}
	if c.life.Err() != nil {
		c.gate <- struct{}{}
		return nil, nil, ErrClosed
	}
	if c.unusable != nil {
		c.gate <- struct{}{}
		return nil, nil, fmt.Errorf("%w: previous operation: %v", ErrUnusable, c.unusable)
	}
	op, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(c.life, cancel)
	return op, func() { stop(); cancel(); c.gate <- struct{}{} }, nil
}

// Close cancels active I/O, waits for its cleanup, and closes the transport.
// Repeated calls return the same result. A blocked print source can delay Close
// because an arbitrary io.Reader cannot be canceled by the client.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.cancel()
		<-c.gate
		if c.transport != nil {
			c.closeErr = c.transport.Close()
		}
		c.gate <- struct{}{}
	})
	return c.closeErr
}

func (c *Client) readStatus(ctx context.Context) (Status, error) {
	empty := 0
	for len(c.pending) < 32 {
		if c.readErr != nil {
			err := c.readErr
			if len(c.pending) > 0 && errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return Status{}, err
		}
		if err := ctx.Err(); err != nil {
			return Status{}, err
		}
		// A USB transfer buffer must be a multiple of the endpoint packet size.
		var b [4096]byte
		n, err := c.transport.Read(ctx, b[:])
		if n < 0 || n > len(b) {
			return Status{}, errors.New("transport returned an invalid read count")
		}
		c.pending = append(c.pending, b[:n]...)
		// Cancellation ends this read, but must not poison the next operation.
		if err != nil && ctx.Err() == nil {
			c.readErr = err
		}
		if n == 0 {
			if err != nil {
				if len(c.pending) > 0 && errors.Is(err, io.EOF) {
					return Status{}, io.ErrUnexpectedEOF
				}
				return Status{}, err
			}
			empty++
			if empty >= 100 {
				return Status{}, io.ErrNoProgress
			}
		}
	}
	s, err := ParseStatus(c.pending[:32])
	c.pending = c.pending[32:]
	return s, err
}

func writeAll(ctx context.Context, t Transport, b []byte) (int, error) {
	return writeAllReady(ctx, t, b, nil)
}

func writeAllReady(ctx context.Context, t Transport, b []byte, ready func(context.Context) error) (int, error) {
	total := 0
	for len(b) > 0 {
		if ready != nil {
			if err := ready(ctx); err != nil {
				return total, err
			}
		}
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := t.Write(ctx, b)
		if n < 0 || n > len(b) {
			return total, errors.New("transport returned an invalid write count")
		}
		total += n
		b = b[n:]
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, io.ErrNoProgress
		}
	}
	return total, nil
}

func (c *Client) status(ctx context.Context) (status Status, resultErr error) {
	defer func() {
		if resultErr != nil && !errors.Is(resultErr, ErrStatusUnsupported) {
			c.unusable = resultErr
		}
	}()
	if c.transport == nil {
		return Status{}, errors.New("nil printer transport")
	}
	if !c.transport.SupportsStatus() {
		return Status{}, ErrStatusUnsupported
	}
	if _, err := writeAll(ctx, c.transport, []byte{0x1b, 0x69, 0x53}); err != nil {
		return Status{}, fmt.Errorf("request status: %w", err)
	}
	for {
		s, err := c.readStatus(ctx)
		if err != nil {
			return s, fmt.Errorf("read status: %w", err)
		}
		if err = s.Err(); err != nil {
			return s, err
		}
		// Ignore notifications left over from the previous job. The reply is a
		// stream-order barrier before observing this job's completion messages.
		if s.Type == StatusReply {
			return s, nil
		}
	}
}

// Status requests the printer's current status, reporting both exchange errors
// and printer errors. Transports without status support return
// ErrStatusUnsupported. A failed status exchange makes the client unusable,
// except when status is unsupported; close it and open a new client.
func (c *Client) Status(ctx context.Context) (Status, error) {
	op, release, err := c.acquire(ctx)
	if err != nil {
		return Status{}, err
	}
	defer release()
	return c.status(op)
}

type printState struct {
	mu      sync.Mutex
	result  Result
	cooling bool
	err     error
	changed chan struct{}
}

func (s *printState) signal() { close(s.changed); s.changed = make(chan struct{}) }
func (s *printState) snapshot() (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.result, s.err
}
func (s *printState) ready(ctx context.Context) error {
	for {
		s.mu.Lock()
		cooling, err, ch := s.cooling, s.err, s.changed
		s.mu.Unlock()
		if err != nil {
			return err
		}
		if !cooling {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// Print sends already-encoded raster data. Validate/encode the entire job before
// calling Print. A source error after transmission can leave a partial job.
// The input reader must itself be finite or interruptible; a context cannot
// cancel an arbitrary io.Reader. Use a context deadline for printer I/O.
// On an unknown outcome, inspect the printer before deciding to retry.
// Interrupted exchanges and unconfirmed bidirectional jobs make this client
// unusable: close it and open a new connection before another operation. This
// prevents stale status messages or incomplete raster commands contaminating
// a later job. Reopening does not establish whether a previous job printed.
func (c *Client) Print(ctx context.Context, r io.Reader, opt PrintOptions) (Result, error) {
	result := Result{Outcome: "failed"}
	op, release, err := c.acquire(ctx)
	if err != nil {
		return result, err
	}
	defer release()
	if r == nil || c.transport == nil {
		return result, errors.New("nil print source or transport")
	}
	if opt.Pages < 0 {
		return result, errors.New("page count cannot be negative")
	}
	if opt.Pages == 0 {
		opt.Pages = 1
	}
	statusEnabled := c.transport.SupportsStatus()
	if opt.Wait && !statusEnabled {
		return result, ErrStatusUnsupported
	}
	if statusEnabled {
		s, err := c.status(op)
		result.LastStatus = &s
		if err != nil {
			return result, err
		}
		if s.Phase != 0 {
			return result, ErrBusy
		}
		if s.MediaType == 0 {
			return result, errors.New("printer has no media loaded")
		}
		if m := opt.Media; m != nil && (m.Width != s.MediaWidth || m.Length != s.MediaLength || (m.Type&^0x40) != (s.MediaType&^0x40)) {
			return result, fmt.Errorf("%w: loaded %dx%d type 0x%02x, requested %dx%d type 0x%02x", ErrMediaMismatch, s.MediaWidth, s.MediaLength, s.MediaType, m.Width, m.Length, m.Type)
		}
	}
	job, cancel := context.WithCancel(op)
	state := &printState{result: result, changed: make(chan struct{})}
	done := make(chan struct{})
	if statusEnabled {
		go func() {
			defer close(done)
			for {
				s, err := c.readStatus(job)
				if job.Err() != nil {
					return
				}
				state.mu.Lock()
				if err == nil {
					state.result.LastStatus = &s
					err = s.Err()
					if err == nil && s.Type == StatusCompleted {
						state.result.PagesPrinted++
					}
					if s.Type == StatusNotification {
						switch s.Notification {
						case CoolingStarted:
							state.cooling = true
						case CoolingFinished:
							state.cooling = false
						}
					}
				}
				if err != nil {
					state.err = err
				}
				complete := opt.Wait && state.result.PagesPrinted >= opt.Pages
				state.signal()
				state.mu.Unlock()
				if err != nil {
					cancel()
					return
				}
				if complete {
					return
				}
			}
		}()
	} else {
		close(done)
	}
	defer func() { cancel(); <-done }()
	var sent int64
	var sendErr error
	buf := make([]byte, 16*1024)
	empty := 0
	for {
		if sendErr = state.ready(job); sendErr != nil {
			break
		}
		n, readErr := r.Read(buf)
		if n < 0 || n > len(buf) {
			sendErr = errors.New("source returned an invalid read count")
			break
		}
		if n > 0 {
			empty = 0
			var wrote int
			wrote, sendErr = writeAllReady(job, c.transport, buf[:n], state.ready)
			sent += int64(wrote)
			if sendErr != nil {
				break
			}
		} else if readErr == nil {
			empty++
			if empty >= 100 {
				sendErr = io.ErrNoProgress
				break
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				sendErr = readErr
			}
			break
		}
	}
	if sent == 0 && sendErr == nil {
		sendErr = errors.New("empty print job")
	}
	if sendErr == nil && opt.Wait {
		for {
			state.mu.Lock()
			complete := state.result.PagesPrinted >= opt.Pages
			err := state.err
			changed := state.changed
			state.mu.Unlock()
			if err != nil {
				sendErr = err
				break
			}
			if complete {
				break
			}
			select {
			case <-job.Done():
				sendErr = job.Err()
			case <-changed:
			}
			if sendErr != nil {
				break
			}
		}
	}
	cancel()
	<-done
	result, statusErr := state.snapshot()
	result.BytesSent = sent
	if statusErr != nil {
		sendErr = statusErr
	}
	if sendErr != nil {
		c.unusable = sendErr
		var printerErr *StatusErrorDetail
		if sent > 0 && !errors.As(sendErr, &printerErr) {
			result.Outcome = "unknown"
		}
		return result, fmt.Errorf("print after sending %d bytes: %w", sent, sendErr)
	}
	result.Outcome = "sent"
	if opt.Wait {
		result.Outcome = "printed"
	} else if statusEnabled {
		c.unusable = errors.New("previous job was sent without waiting for completion")
	}
	return result, nil
}
