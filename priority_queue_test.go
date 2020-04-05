package ttlcache

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPriorityQueuePush(t *testing.T) {
	queue := newPriorityQueue()
	for i := 0; i < 10; i++ {
		queue.push(newItem(fmt.Sprintf("key_%d", i), "Data", -1))
	}
	assert.Equal(t, queue.Len(), 10, "Expected queue to have 10 elements")
}

func TestPriorityQueuePop(t *testing.T) {
	queue := newPriorityQueue()
	for i := 0; i < 10; i++ {
		queue.push(newItem(fmt.Sprintf("key_%d", i), "Data", -1))
	}
	for i := 0; i < 5; i++ {
		item := queue.pop()
		assert.Equal(t, fmt.Sprintf("%T", item), "*ttlcache.Item", "Expected 'Item' to be a '*ttlcache.Item'")
	}
	assert.Equal(t, queue.Len(), 5, "Expected queue to have 5 elements")
	for i := 0; i < 5; i++ {
		item := queue.pop()
		assert.Equal(t, fmt.Sprintf("%T", item), "*ttlcache.Item", "Expected 'Item' to be a '*ttlcache.Item'")
	}
	assert.Equal(t, queue.Len(), 0, "Expected queue to have 0 elements")

	item := queue.pop()
	assert.Nil(t, item, "*ttlcache.Item", "Expected 'Item' to be nil")
}

func TestPriorityQueueCheckOrder(t *testing.T) {
	queue := newPriorityQueue()
	for i := 10; i > 0; i-- {
		queue.push(newItem(fmt.Sprintf("key_%d", i), "Data", time.Duration(i)*time.Second))
	}
	for i := 1; i <= 10; i++ {
		item := queue.pop()
		assert.Equal(t, item.key, fmt.Sprintf("key_%d", i), "error")
	}
}

func TestPriorityQueueRemove(t *testing.T) {
	queue := newPriorityQueue()
	items := make(map[string]*Item)
	var itemRemove *Item
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("key_%d", i)
		items[key] = newItem(key, "Data", time.Duration(i)*time.Second)
		queue.push(items[key])

		if i == 2 {
			itemRemove = items[key]
		}
	}
	assert.Equal(t, queue.Len(), 5, "Expected queue to have 5 elements")
	queue.remove(itemRemove)
	assert.Equal(t, queue.Len(), 4, "Expected queue to have 4 elements")

	for {
		item := queue.pop()
		if item == nil {
			break
		}
		assert.NotEqual(t, itemRemove.key, item.key, "This element was not supose to be in the queue")
	}

	assert.Equal(t, queue.Len(), 0, "The queue is supose to be with 0 items")
}

func TestPriorityQueueUpdate(t *testing.T) {
	queue := newPriorityQueue()
	item := newItem("key", "Data", 1*time.Second)
	queue.push(item)
	assert.Equal(t, queue.Len(), 1, "The queue is supose to be with 1 Item")

	item.key = "newKey"
	queue.update(item)
	newItem := queue.pop()
	assert.Equal(t, newItem.key, "newKey", "The Item key didn't change")
	assert.Equal(t, queue.Len(), 0, "The queue is supose to be with 0 items")
}
