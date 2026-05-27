package arr

import (
	"sync"
	"time"
)

const validateCacheTTL = 5 * time.Minute

type validateCacheEntry struct {
	ok      bool
	expires time.Time
}

var validateCache sync.Map

// ValidateCached runs Validate at most once per host+token per TTL window.
func ValidateCached(a *Arr) bool {
	if a == nil {
		return false
	}
	key := a.Host + "|" + a.Token
	if key == "|" {
		return a.Validate() == nil
	}
	if v, ok := validateCache.Load(key); ok {
		e := v.(validateCacheEntry)
		if time.Now().Before(e.expires) {
			return e.ok
		}
		validateCache.Delete(key)
	}
	ok := a.Validate() == nil
	validateCache.Store(key, validateCacheEntry{ok: ok, expires: time.Now().Add(validateCacheTTL)})
	return ok
}
