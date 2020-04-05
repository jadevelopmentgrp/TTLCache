package ttlcache

import (
	"sync"
	"time"
)

// CheckExpireCallback is used as a callback for an external check on Item expiration
type checkExpireCallback func(key string, value interface{}) bool

// ExpireCallback is used as a callback on Item expiration or when notifying of an Item new to the cache
type expireCallback func(key string, value interface{})

// Cache is a synchronized map of items that can auto-expire once stale
type Cache struct {
	mutex                  sync.Mutex
	ttl                    time.Duration
	items                  map[string]*Item
	expireCallback         expireCallback
	checkExpireCallback    checkExpireCallback
	newItemCallback        expireCallback
	priorityQueue          *priorityQueue
	expirationNotification chan bool
	expirationTime         time.Time
	skipTTLExtension       bool
	shutdownSignal         chan (chan struct{})
	isShutDown             bool
}

func (cache *Cache) GetItem(key string) (*Item, bool, bool) {
	item, exists := cache.items[key]
	if !exists || item.expired() {
		return nil, false, false
	}

	if item.TTL >= 0 && (item.TTL > 0 || cache.ttl > 0) {
		if cache.ttl > 0 && item.TTL == 0 {
			item.TTL = cache.ttl
		}

		if !cache.skipTTLExtension {
			item.touch()
		}
		cache.priorityQueue.update(item)
	}

	expirationNotification := false
	if cache.expirationTime.After(time.Now().Add(item.TTL)) {
		expirationNotification = true
	}
	return item, exists, expirationNotification
}

func (cache *Cache) startExpirationProcessing() {
	timer := time.NewTimer(time.Hour)
	for {
		var sleepTime time.Duration
		cache.mutex.Lock()
		if cache.priorityQueue.Len() > 0 {
			sleepTime = time.Until(cache.priorityQueue.items[0].ExpireAt)
			if sleepTime < 0 && cache.priorityQueue.items[0].ExpireAt.IsZero() {
				sleepTime = time.Hour
			} else if sleepTime < 0 {
				sleepTime = time.Microsecond
			}
			if cache.ttl > 0 {
				sleepTime = min(sleepTime, cache.ttl)
			}

		} else if cache.ttl > 0 {
			sleepTime = cache.ttl
		} else {
			sleepTime = time.Hour
		}

		cache.expirationTime = time.Now().Add(sleepTime)
		cache.mutex.Unlock()

		timer.Reset(sleepTime)
		select {
		case shutdownFeedback := <-cache.shutdownSignal:
			timer.Stop()
			shutdownFeedback <- struct{}{}
			return
		case <-timer.C:
			timer.Stop()
			cache.mutex.Lock()
			if cache.priorityQueue.Len() == 0 {
				cache.mutex.Unlock()
				continue
			}

			// index will only be advanced if the current entry will not be evicted
			i := 0
			for item := cache.priorityQueue.items[i]; item.expired(); item = cache.priorityQueue.items[i] {

				if cache.checkExpireCallback != nil {
					if !cache.checkExpireCallback(item.key, item.Data) {
						item.touch()
						cache.priorityQueue.update(item)
						i++
						if i == cache.priorityQueue.Len() {
							break
						}
						continue
					}
				}

				cache.priorityQueue.remove(item)
				delete(cache.items, item.key)
				if cache.expireCallback != nil {
					go cache.expireCallback(item.key, item.Data)
				}
				if cache.priorityQueue.Len() == 0 {
					goto done
				}
			}
		done:
			cache.mutex.Unlock()

		case <-cache.expirationNotification:
			timer.Stop()
			continue
		}
	}
}

// Close calls Purge, and then stops the goroutine that does TTL checking, for a clean shutdown.
// The cache is no longer cleaning up after the first call to Close, repeated calls are safe though.
func (cache *Cache) Close() {

	cache.mutex.Lock()
	if !cache.isShutDown {
		cache.isShutDown = true
		cache.mutex.Unlock()
		feedback := make(chan struct{})
		cache.shutdownSignal <- feedback
		<-feedback
		close(cache.shutdownSignal)
	} else {
		cache.mutex.Unlock()
	}
	cache.Purge()
}

// Set is a thread-safe way to add new items to the map
func (cache *Cache) Set(key string, data interface{}) {
	cache.SetWithTTL(key, data, ItemExpireWithGlobalTTL)
}

// SetWithTTL is a thread-safe way to add new items to the map with individual TTL
func (cache *Cache) SetWithTTL(key string, data interface{}, ttl time.Duration) {
	cache.mutex.Lock()
	item, exists, _ := cache.GetItem(key)

	if exists {
		item.Data = data
		item.TTL = ttl
	} else {
		item = newItem(key, data, ttl)
		cache.items[key] = item
	}

	if item.TTL >= 0 && (item.TTL > 0 || cache.ttl > 0) {
		if cache.ttl > 0 && item.TTL == 0 {
			item.TTL = cache.ttl
		}
		item.touch()
	}

	if exists {
		cache.priorityQueue.update(item)
	} else {
		cache.priorityQueue.push(item)
	}

	cache.mutex.Unlock()
	if !exists && cache.newItemCallback != nil {
		cache.newItemCallback(key, data)
	}
	cache.expirationNotification <- true
}

// Get is a thread-safe way to lookup items
// Every lookup, also touches the Item, hence extending it's life
func (cache *Cache) Get(key string) (interface{}, bool) {
	cache.mutex.Lock()
	item, exists, triggerExpirationNotification := cache.GetItem(key)

	var dataToReturn interface{}
	if exists {
		dataToReturn = item.Data
	}
	cache.mutex.Unlock()
	if triggerExpirationNotification {
		cache.expirationNotification <- true
	}
	return dataToReturn, exists
}

func (cache *Cache) GetTTL(key string) (time.Duration, bool) {
	cache.mutex.Lock()
	item, exists, _ := cache.GetItem(key)
	cache.mutex.Unlock()

	if exists {
		return item.TTL, true
	} else {
		return 0, false
	}
}

func (cache *Cache) Remove(key string) bool {
	cache.mutex.Lock()
	object, exists := cache.items[key]
	if !exists {
		cache.mutex.Unlock()
		return false
	}
	delete(cache.items, object.key)
	cache.priorityQueue.remove(object)
	cache.mutex.Unlock()

	return true
}

// Count returns the number of items in the cache
func (cache *Cache) Count() int {
	cache.mutex.Lock()
	length := len(cache.items)
	cache.mutex.Unlock()
	return length
}

func (cache *Cache) SetTTL(ttl time.Duration) {
	cache.mutex.Lock()
	cache.ttl = ttl
	cache.mutex.Unlock()
	cache.expirationNotification <- true
}

// SetExpirationCallback sets a callback that will be called when an Item expires
func (cache *Cache) SetExpirationCallback(callback expireCallback) {
	cache.expireCallback = callback
}

// SetCheckExpirationCallback sets a callback that will be called when an Item is about to expire
// in order to allow external code to decide whether the Item expires or remains for another TTL cycle
func (cache *Cache) SetCheckExpirationCallback(callback checkExpireCallback) {
	cache.checkExpireCallback = callback
}

// SetNewItemCallback sets a callback that will be called when a new Item is added to the cache
func (cache *Cache) SetNewItemCallback(callback expireCallback) {
	cache.newItemCallback = callback
}

// SkipTtlExtensionOnHit allows the user to change the cache behaviour. When this flag is set to true it will
// no longer extend TTL of items when they are retrieved using Get, or when their expiration condition is evaluated
// using SetCheckExpirationCallback.
func (cache *Cache) SkipTtlExtensionOnHit(value bool) {
	cache.skipTTLExtension = value
}

// Purge will remove all entries
func (cache *Cache) Purge() {
	cache.mutex.Lock()
	cache.items = make(map[string]*Item)
	cache.priorityQueue = newPriorityQueue()
	cache.mutex.Unlock()
}

// NewCache is a helper to create instance of the Cache struct
func NewCache() *Cache {

	shutdownChan := make(chan chan struct{})

	cache := &Cache{
		items:                  make(map[string]*Item),
		priorityQueue:          newPriorityQueue(),
		expirationNotification: make(chan bool),
		expirationTime:         time.Now(),
		shutdownSignal:         shutdownChan,
		isShutDown:             false,
	}
	go cache.startExpirationProcessing()
	return cache
}

func min(duration time.Duration, second time.Duration) time.Duration {
	if duration < second {
		return duration
	}
	return second
}
