package manager

import (
	"context"
	"io"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/sirrobot01/decypharr/internal/config"
	"golang.org/x/time/rate"
)

// bandwidthController coordinates a global rate limiter (download.bandwidth_limit)
// with optional per-provider rate limiters (Debrid.BandwidthLimit). It is safe
// for concurrent use by multiple downloaders.
//
// Bytes are accounted twice on purpose: the global limiter shapes total egress
// while the per-provider limiter shapes one provider's egress; whichever is
// most constrained wins because every read calls both.
type bandwidthController struct {
	mu       sync.RWMutex
	global   *rate.Limiter
	perProv  map[string]*rate.Limiter
	limits   bandwidthLimits
	bytesIn  atomic.Int64 // observed bytes/sec gauge (rolled over by sampler)
	totalIn  atomic.Int64 // running total bytes since process start
}

type bandwidthLimits struct {
	GlobalBytes   int64
	ProviderBytes map[string]int64
}

// newBandwidthController builds a controller that reflects the current config.
// Pass nil to disable shaping. Subsequent config changes should call Reload.
func newBandwidthController(cfg *config.Config) *bandwidthController {
	c := &bandwidthController{perProv: map[string]*rate.Limiter{}}
	c.Reload(cfg)
	return c
}

// Reload swaps in fresh limiters when config changes. Existing in-flight
// downloads pick up the new rate on their next read because they look up the
// limiter on each call.
func (c *bandwidthController) Reload(cfg *config.Config) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if cfg == nil {
		c.global = nil
		c.perProv = map[string]*rate.Limiter{}
		c.limits = bandwidthLimits{}
		return
	}
	limits := bandwidthLimits{ProviderBytes: map[string]int64{}}
	if v := cfg.Download.BandwidthLimitBytes(); v > 0 {
		c.global = rate.NewLimiter(rate.Limit(v), int(burstFor(v)))
		limits.GlobalBytes = v
	} else {
		c.global = nil
	}
	c.perProv = map[string]*rate.Limiter{}
	for _, d := range cfg.Debrids {
		v, _ := config.ParseBandwidth(d.BandwidthLimit)
		if v <= 0 {
			continue
		}
		c.perProv[d.Name] = rate.NewLimiter(rate.Limit(v), int(burstFor(v)))
		limits.ProviderBytes[d.Name] = v
	}
	c.limits = limits
}

// burstFor returns a sensible burst size for the given rate (1s of traffic,
// clamped to [64KiB, 8MiB] so we don't starve small caps or buffer too much
// for huge ones).
func burstFor(bytesPerSec int64) int64 {
	const minBurst = 64 << 10
	const maxBurst = 8 << 20
	switch {
	case bytesPerSec < minBurst:
		return minBurst
	case bytesPerSec > maxBurst:
		return maxBurst
	default:
		return bytesPerSec
	}
}

// Snapshot returns a copy of the active rate caps for UI consumption.
func (c *bandwidthController) Snapshot() bandwidthLimits {
	if c == nil {
		return bandwidthLimits{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := bandwidthLimits{
		GlobalBytes:   c.limits.GlobalBytes,
		ProviderBytes: make(map[string]int64, len(c.limits.ProviderBytes)),
	}
	for k, v := range c.limits.ProviderBytes {
		out.ProviderBytes[k] = v
	}
	return out
}

// TotalBytes returns the running total bytes shaped through this controller.
func (c *bandwidthController) TotalBytes() int64 {
	if c == nil {
		return 0
	}
	return c.totalIn.Load()
}

// transportFor returns an http.RoundTripper that wraps the given base
// transport so response bodies are throttled. The returned tripper is safe to
// share across requests bound to the same provider.
func (c *bandwidthController) transportFor(base http.RoundTripper, provider string) http.RoundTripper {
	if c == nil {
		return base
	}
	if base == nil {
		base = http.DefaultTransport
	}
	return &throttledTransport{base: base, ctrl: c, provider: provider}
}

type throttledTransport struct {
	base     http.RoundTripper
	ctrl     *bandwidthController
	provider string
}

func (t *throttledTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	resp.Body = &throttledBody{
		ReadCloser: resp.Body,
		ctrl:       t.ctrl,
		provider:   t.provider,
		ctx:        req.Context(),
	}
	return resp, nil
}

type throttledBody struct {
	io.ReadCloser
	ctrl     *bandwidthController
	provider string
	ctx      context.Context
}

func (b *throttledBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 && b.ctrl != nil {
		b.ctrl.totalIn.Add(int64(n))
		b.ctrl.bytesIn.Add(int64(n))
		b.wait(n)
	}
	return n, err
}

// wait blocks until the configured limiters allow `n` bytes through. Errors
// from the rate limiter (only ctx cancellation) are silently swallowed; the
// next Read will surface the underlying transport error.
func (b *throttledBody) wait(n int) {
	if b.ctrl == nil {
		return
	}
	b.ctrl.mu.RLock()
	global := b.ctrl.global
	prov := b.ctrl.perProv[b.provider]
	b.ctrl.mu.RUnlock()

	ctx := b.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	// Cap WaitN to the limiter's burst so token-bucket WaitN never errors
	// "would exceed context deadline" for very large reads.
	for _, lim := range []*rate.Limiter{global, prov} {
		if lim == nil {
			continue
		}
		remaining := n
		burst := lim.Burst()
		if burst <= 0 {
			continue
		}
		for remaining > 0 {
			step := remaining
			if step > burst {
				step = burst
			}
			_ = lim.WaitN(ctx, step)
			remaining -= step
		}
	}
}
