package main

import (
	"hash/fnv"
	"sync"
)

type Shard[T uint32 | uint64] struct {
	*sync.RWMutex
	data map[T]*GnetPeer
}

type ShardMap[T uint32 | uint64] struct {
	Count int
	Shards []*Shard[T]
}

func NewShardMap[T uint32 | uint64](count int) *ShardMap[T] {
	shardMap := &ShardMap[T]{
		Count: count,
		Shards: make([]*Shard[T], count),
	}
	for i := range shardMap.Shards {
		shardMap.Shards[i] = &Shard[T]{
			RWMutex: &sync.RWMutex{},
			data: map[T]*GnetPeer{},
		}
	}
	return shardMap
}

func (shardmap *ShardMap[T]) GetShardKey(key T) uint32 {
	hasher := fnv.New32()
	switch any(key).(type) {
	case uint32:
		hasher.Write([]byte{byte(key>>24), byte(key>>16), byte(key>>8), byte(key)})
	case uint64:
		hasher.Write([]byte{byte(key>>40), byte(key>>32), byte(key>>24), byte(key>>16), byte(key>>8), byte(key)})
	}
	return hasher.Sum32() % uint32(shardmap.Count)
}

func (shardMap *ShardMap[T]) Set(key T, peer *GnetPeer) {
	shard := shardMap.Shards[shardMap.GetShardKey(key)]
	shard.Lock()
	shard.data[key] = peer
	shard.Unlock()
}

func (shardMap *ShardMap[T]) Get(key T) (*GnetPeer, bool) {
	shard := shardMap.Shards[shardMap.GetShardKey(key)]
	shard.RLock()
	peer, ok := shard.data[key]
	shard.RUnlock()
	return peer, ok
}

/* func (shardMap ShardMap[T]) Delete(key T) {
	shard := shardMap.Shards[shardMap.GetShardKey(key)]
	shard.Lock()
	delete(shard.data, key)
	shard.Unlock()
} */

func (shardMap *ShardMap[T]) GetM(key T) *GnetPeer {
	peer, _ := shardMap.Get(key)
	return peer
}