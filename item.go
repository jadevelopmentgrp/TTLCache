package ttlcache

import (
	"time"
)

const (
	// ItemNotExpire Will avoid the Item being expired by TTL, but can still be exired by callback etc.
	ItemNotExpire time.Duration = -1
	// ItemExpireWithGlobalTTL will use the global TTL when set.
	ItemExpireWithGlobalTTL time.Duration = 0
)

func newItem(key string, data interface{}, ttl time.Duration) *Item {
	item := &Item{
		Data: data,
		TTL:  ttl,
		key:  key,
	}
	// since nobody is aware yet of this Item, it's safe to touch without lock here
	item.touch()
	return item
}

type Item struct {
	key        string
	Data       interface{}
	TTL        time.Duration
	ExpireAt   time.Time
	queueIndex int
}

// Reset the Item expiration time
func (item *Item) touch() {
	if item.TTL > 0 {
		item.ExpireAt = time.Now().Add(item.TTL)
	}
}

// Verify if the Item is expired
func (item *Item) expired() bool {
	if item.TTL <= 0 {
		return false
	}
	return item.ExpireAt.Before(time.Now())
}
