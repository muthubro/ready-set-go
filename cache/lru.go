package cache

import (
	"container/list"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// LRUCache represents an LRU Cache object
type LRUCache struct {
	mu sync.Mutex

	list  *list.List
	table map[string]*list.Element

	size uint64

	capacity uint64
}

// Value gives a basic interface for a cache value
type Value interface {
	Size() int
}

// Item represents a cache item
type Item struct {
	Key   string
	Value Value
}

type entry struct {
	key          string
	value        Value
	size         int
	timeAccessed time.Time
}

// NewLRUCache creates a new LRU Cache
func NewLRUCache(capacity uint64) *LRUCache {
	return &LRUCache{
		list:     list.New(),
		table:    make(map[string]*list.Element),
		capacity: capacity,
	}
}

// Get returns the value in the cache corresponding to the given key
func (lru *LRUCache) Get(key string) (v Value, ok bool) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	element := lru.table[key]
	if element == nil {
		return nil, false
	}
	lru.moveToFront(element)
	return element.Value.(*entry).value, true
}

// Set creates a new cache entry if it doesn't exist.
// If it exists, moves it to the front
func (lru *LRUCache) Set(key string, value Value) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	if element := lru.table[key]; element != nil {
		lru.moveToFront(element)
	} else {
		lru.addNew(key, value)
	}
}

// SetIfAbsent creates a new cache entry only if it doesn't exist.
func (lru *LRUCache) SetIfAbsent(key string, value Value) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	if element := lru.table[key]; element == nil {
		lru.addNew(key, value)
	}
}

// Delete deletes the cache entry corresponding to the key
func (lru *LRUCache) Delete(key string) bool {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	element := lru.table[key]
	if element == nil {
		return false
	}

	lru.list.Remove(element)
	delete(lru.table, key)
	lru.size -= uint64(element.Value.(*entry).size)
	return true
}

// Clear clears the cache
func (lru *LRUCache) Clear() {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	lru.list.Init()
	lru.table = make(map[string]*list.Element)
	lru.size = 0
}

// SetCapacity sets the cache capacity
func (lru *LRUCache) SetCapacity(capacity uint64) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	lru.capacity = capacity
	lru.checkCapacity()
}

// Stats returns some information about the cache
func (lru *LRUCache) Stats() (length, size, capacity uint64, oldest time.Time) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	if lastElem := lru.list.Back(); lastElem != nil {
		oldest = lastElem.Value.(*entry).timeAccessed
	}
	return uint64(lru.list.Len()), lru.size, lru.capacity, oldest
}

// StatsJSON returns information about the cache in JSON format
func (lru *LRUCache) StatsJSON() string {
	if lru == nil {
		return "{}"
	}

	length, size, capacity, oldest := lru.Stats()
	return fmt.Sprintf(
		"{\"Length\": %v, \"Size\": %v, \"Capacity\": %v, \"OldestAccess\": \"%v\"}",
		length, size, capacity, oldest,
	)
}

// Keys returns all keys in the cache
func (lru *LRUCache) Keys() []string {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	keys := make([]string, 0, lru.list.Len())
	for element := lru.list.Front(); element != nil; element = element.Next() {
		keys = append(keys, element.Value.(*entry).key)
	}
	return keys
}

// Items returns all items in the cache
func (lru *LRUCache) Items() []Item {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	items := make([]Item, 0, lru.list.Len())
	for element := lru.list.Front(); element != nil; element = element.Next() {
		v := element.Value.(*entry)
		items = append(items, Item{Key: v.key, Value: v.value})
	}
	return items
}

// SaveItems saves the cache items by transmitting to an io.Writer
func (lru *LRUCache) SaveItems(w io.Writer) error {
	items := lru.Items()
	encoder := gob.NewEncoder(w)
	return encoder.Encode(items)
}

// SaveItemsToFile saves the cache items in a file
func (lru *LRUCache) SaveItemsToFile(path string) error {
	if writer, err := os.OpenFile(
		path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644); err != nil {
		return err
	} else {
		defer writer.Close()
		return lru.SaveItems(writer)
	}
}

// LoadItems loads cache items from io.Reader
func (lru *LRUCache) LoadItems(r io.Reader) error {
	items := make([]Item, 0)
	decoder := gob.NewDecoder(r)
	if err := decoder.Decode(&items); err != nil {
		return err
	}

	lru.mu.Lock()
	defer lru.mu.Unlock()

	for _, item := range items {
		if element := lru.table[item.Key]; element != nil {
			lru.updateInplace(element, item.Value)
		} else {
			lru.addNew(item.Key, item.Value)
		}
	}

	return nil
}

// LoadItemsFromFile loads cache items from file
func (lru *LRUCache) LoadItemsFromFile(path string) error {
	if reader, err := os.Open(path); err != nil {
		return err
	} else {
		defer reader.Close()
		return lru.LoadItems(reader)
	}
}

func (lru *LRUCache) updateInplace(element *list.Element, value Value) {
	valueSize := value.Size()
	sizeDiff := valueSize - element.Value.(*entry).size

	element.Value.(*entry).value = value
	element.Value.(*entry).size = valueSize

	lru.size += uint64(sizeDiff)
	lru.moveToFront(element)
	lru.checkCapacity()
}

func (lru *LRUCache) moveToFront(element *list.Element) {
	lru.list.MoveToFront(element)
	element.Value.(*entry).timeAccessed = time.Now()
}

func (lru *LRUCache) addNew(key string, value Value) {
	newEntry := &entry{key, value, value.Size(), time.Now()}
	element := lru.list.PushFront(newEntry)

	lru.table[key] = element
	lru.size += uint64(newEntry.size)
	lru.checkCapacity()
}

func (lru *LRUCache) checkCapacity() {
	for lru.size > lru.capacity {
		delElem := lru.list.Back()
		delValue := delElem.Value.(*entry)

		lru.list.Remove(delElem)
		delete(lru.table, delValue.key)
		lru.size -= uint64(delValue.size)
	}
}
