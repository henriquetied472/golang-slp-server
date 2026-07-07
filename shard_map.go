package main

import (
	"hash/fnv"
	"sync"
)

type Shard struct {
	*sync.RWMutex
	data map[uint32]*Peer
}

type ShardMap struct {
	Count int
	Shards []*Shard
}

func NewShardMap(count int) *ShardMap {
	shardMap := &ShardMap{
		Count: count,
		Shards: make([]*Shard, count),
	}
	for i := range shardMap.Shards {
		shardMap.Shards[i] = &Shard{
			RWMutex: &sync.RWMutex{},
			data: map[uint32]*Peer{},
		}
	}
	return shardMap
}

func (shardmap *ShardMap) GetShardKey(key uint32) uint32 {
	hasher := fnv.New32()
	hasher.Write([]byte{byte(key>>24), byte(key>>16), byte(key>>8), byte(key)})
	return hasher.Sum32() % uint32(shardmap.Count)
}

func (shardMap *ShardMap) Set(key uint32, peer *Peer) {
	shard := shardMap.Shards[shardMap.GetShardKey(key)]
	shard.Lock()
	shard.data[key] = peer
	shard.Unlock()
}

func (shardMap *ShardMap) Get(key uint32) (*Peer, bool) {
	shard := shardMap.Shards[shardMap.GetShardKey(key)]
	shard.RLock()
	peer, ok := shard.data[key]
	shard.RUnlock()
	return peer, ok
}