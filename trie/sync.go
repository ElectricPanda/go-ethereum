// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package trie

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/prque"
	"github.com/ethereum/go-ethereum/ethdb"
)

var ReceiverPopFilePath = "/data/trieHashimoto/th_experiment_data_3/logs/batch1/setHead_100000_cache_10G_depth+[0:9]_pop.txt"
// var ReceiverPushFilePath = "/data/trieHashimoto/th_experiment_data_3/logs/batch1/setHead_100000_cache_10G_depth+[0:9]_push.txt"

//mskim
func logQueuePush(input string, priority int, queueSize int, requestSize int, time time.Duration){
	return
	// f, err := os.OpenFile(ReceiverPushFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	// if err != nil {
	// 	fmt.Println("ERR:", err)
	// }

	// HashData:= "PUSHED Hash: "+ input
	// PriorityData := "Priority: " + strconv.Itoa(priority)
	// queueSizeData := "Queue Size: " + strconv.Itoa(queueSize)
	// requestSizeData := "Request Size: " + strconv.Itoa(requestSize)
	// timeData := "Took time: "+ strconv.Itoa(int(time.Nanoseconds()))

	// fmt.Fprintln(f, HashData, PriorityData,  queueSizeData, requestSizeData, timeData)
	// f.Close()
}

func logQueuePushNewLine(){
	return
	// f, err := os.OpenFile(ReceiverPushFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	// if err != nil {
	// 	fmt.Println("ERR:", err)
	// }
	// fmt.Fprintln(f,"")
	// f.Close()
}

func logQueuePop(input string, priority int, queueSize int, requestSize int, time time.Duration){
	return
	f, err := os.OpenFile(ReceiverPopFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("ERR:", err)
	}

	HashData := "POPPED Hash: " + input
	PriorityData := "Priority: " + strconv.Itoa(priority)
	queueSizeData := "Queue Size: " + strconv.Itoa(queueSize)
	requestSizeData := "Request Size: " + strconv.Itoa(requestSize)
	timeData := "Took time: " + strconv.Itoa(int(time.Nanoseconds()))

	fmt.Fprintln(f, HashData, PriorityData, queueSizeData, requestSizeData, timeData)
	f.Close()
}

func logQueuePopNewLine(){
	return
	f, err := os.OpenFile(ReceiverPopFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("ERR:", err)
	}
	fmt.Fprintln(f,"")
	f.Close()
}

// overhead of correctly prefixing for fake IMPT data (nanoseconds)
var TotalPrefixingOverhead uint64 = 0

// ErrNotRequested is returned by the trie sync when it's requested to process a
// node it did not request.
var ErrNotRequested = errors.New("not requested")

// ErrAlreadyProcessed is returned by the trie sync when it's requested to process a
// node it already processed previously.
var ErrAlreadyProcessed = errors.New("already processed")

// request represents a scheduled or already in-flight state retrieval request.
type request struct {
	hash common.Hash // Hash of the node data content to retrieve
	data []byte      // Data content of the node, cached until all subtrees complete
	raw  bool        // Whether this is a raw entry (code) or a trie node

	parents []*request // Parent state nodes referencing this entry (notify all upon completion)
	depth   int        // Depth level within the trie the node is located to prioritise DFS
	deps    int        // Number of dependencies before allowed to commit this node

	callback LeafCallback // Callback to invoke if a leaf node it reached on this branch
}

// SyncResult is a simple list to return missing nodes along with their request
// hashes.
type SyncResult struct {
	Hash common.Hash // Hash of the originally unknown trie node
	Data []byte      // Data content of the retrieved node
}

// syncMemBatch is an in-memory buffer of successfully downloaded but not yet
// persisted data items.
type syncMemBatch struct {
	batch map[common.Hash][]byte // In-memory membatch of recently completed items
	order []common.Hash          // Order of completion to prevent out-of-order data loss
}

// newSyncMemBatch allocates a new memory-buffer for not-yet persisted trie nodes.
func newSyncMemBatch() *syncMemBatch {
	return &syncMemBatch{
		batch: make(map[common.Hash][]byte),
		order: make([]common.Hash, 0, 256),
	}
}

// Sync is the main state trie synchronisation scheduler, which provides yet
// unknown trie hashes to retrieve, accepts node data associated with said hashes
// and reconstructs the trie step by step until all is done.
type Sync struct {
	database ethdb.KeyValueReader     // Persistent database to check for existing entries
	membatch *syncMemBatch            // Memory buffer to avoid frequent database writes
	requests map[common.Hash]*request // Pending requests pertaining to a key hash
	queue    *prque.Prque             // Priority queue with the pending requests
	bloom    *SyncBloom               // Bloom filter for fast node existence checks

	// s.hashes[fakeHash_withoutBlockNumberPrefix] = fakeHash (this is for fast sync with fake Trie-Hashimoto data) (jmlee)
	// (this is not necessary with real Trie-Hashimoto data which has really PoW-mined trie nodes)
	hashes	map[string]common.Hash
}

// NewSync creates a new trie data download scheduler.
func NewSync(root common.Hash, database ethdb.KeyValueReader, callback LeafCallback, bloom *SyncBloom) *Sync {
	ts := &Sync{
		database: database,
		membatch: newSyncMemBatch(),
		requests: make(map[common.Hash]*request),
		queue:    prque.New(nil),
		bloom:    bloom,
		hashes: make(map[string]common.Hash),
	}
	ts.AddSubTrie(root, 0, common.Hash{}, callback)
	return ts
}

// AddSubTrie registers a new trie to the sync code, rooted at the designated parent.
func (s *Sync) AddSubTrie(root common.Hash, depth int, parent common.Hash, callback LeafCallback) {
	// Short circuit if the trie is empty or already known
	if root == emptyRoot {
		return
	}
	if _, ok := s.membatch.batch[root]; ok {
		return
	}
	if s.bloom.Contains(root[:]) {
		// Bloom filter says this might be a duplicate, double check
		blob, _ := s.database.Get(root[:])
		if local, err := decodeNode(root[:], blob); local != nil && err == nil {
			return
		}
		// False positive, bump fault meter
		bloomFaultMeter.Mark(1)
	}
	// Assemble the new sub-trie sync request
	req := &request{
		hash:     root,
		depth:    depth,
		callback: callback,
	}
	// If this sub-trie has a designated parent, link them together
	if parent != (common.Hash{}) {
		ancestor := s.requests[parent]
		if ancestor == nil {
			panic(fmt.Sprintf("sub-trie ancestor not found: %x", parent))
		}
		ancestor.deps++
		req.parents = append(req.parents, ancestor)
	}
	fmt.Println("AddSubTrie schedule()")
	s.schedule(req)
	logQueuePushNewLine()
}

// AddRawEntry schedules the direct retrieval of a state entry that should not be
// interpreted as a trie node, but rather accepted and stored into the database
// as is. This method's goal is to support misc state metadata retrievals (e.g.
// contract code).
func (s *Sync) AddRawEntry(hash common.Hash, depth int, parent common.Hash) {
	// Short circuit if the entry is empty or already known
	if hash == emptyState {
		return
	}
	if _, ok := s.membatch.batch[hash]; ok {
		return
	}
	if s.bloom.Contains(hash[:]) {
		// Bloom filter says this might be a duplicate, double check
		if ok, _ := s.database.Has(hash[:]); ok {
			return
		}
		// False positive, bump fault meter
		bloomFaultMeter.Mark(1)
	}
	// Assemble the new sub-trie sync request
	req := &request{
		hash:  hash,
		raw:   true,
		depth: depth,
	}
	// If this sub-trie has a designated parent, link them together
	if parent != (common.Hash{}) {
		ancestor := s.requests[parent]
		if ancestor == nil {
			panic(fmt.Sprintf("raw-entry ancestor not found: %x", parent))
		}
		ancestor.deps++
		req.parents = append(req.parents, ancestor)
	}
	fmt.Println("AddRawEntry schedule()")
	s.schedule(req)
}

// Missing retrieves the known missing nodes from the trie for retrieval.
func (s *Sync) Missing(max int) []common.Hash {
	// fmt.Println("Missing() max: ", max)
	var requests []common.Hash
	iterationCount := 0
	for !s.queue.Empty() && (max == 0 || len(requests) < max) {
		iterationCount++
		fmt.Printf("\t\tQUEUE POP #: %d -------------------------------------------\n", iterationCount)
		start2 := time.Now()
		// var temp = s.queue.PopItem().(common.Hash)
		var tempValue, tempPriority = s.queue.Pop()
		var temp = tempValue.(common.Hash)
		elapsed2 := time.Since(start2)
		fmt.Printf("\t\t\tQUEUE POP Hash: %s\n", temp.Hex())
		fmt.Printf("\t\t\tQUEUE POP Priority: %d\n", tempPriority)
		fmt.Printf("\t\t\tQUEUE POP TIME: %s\n", elapsed2)
		fmt.Printf("\t\t\tQUEUE POP SIZE: %d\n", s.queue.Size())
		requests = append(requests, temp)
		logQueuePop(temp.Hex(), int(tempPriority), s.queue.Size(), len(s.requests), elapsed2)
	}

	// code to send node requests in strictly descending order(a.k.a. benchmark) (mskim)
	// for (common.DescendingOrderNodeHashPopIndex < len(common.DescendingOrderNodeHash))  && (max == 0 || len(requests) < max){

	// 	PoppedNodeHash := common.HexToHash(common.DescendingOrderNodeHash[common.DescendingOrderNodeHashPopIndex])
	// 	requests = append(requests,PoppedNodeHash)
	// 	common.DescendingOrderNodeHashPopIndex++

	// 	//전처리
	// 	prefixLen := PrefixLength*2 + 2
	// 	hashWithoutPrefix := string([]rune(PoppedNodeHash.Hex())[prefixLen:])
	// 	s.hashes[hashWithoutPrefix] = PoppedNodeHash

	// 	logQueuePop(common.DescendingOrderNodeHash[common.DescendingOrderNodeHashPopIndex], 888888888, 888888888, len(s.requests), 888888888)
	// }

	logQueuePopNewLine()
	return requests
}

// Process injects a batch of retrieved trie nodes data, returning if something
// was committed to the database and also the index of an entry if processing of
// it failed.
func (s *Sync) Process(results []SyncResult) (bool, int, error) {
	// var k = 0;
	// fmt.Println("Process() 인자로 results 받았고 그 길이는: ", len(results))
	committed := false

	// fmt.Println("length of s.queue: ", s.queue.Size())
	// fmt.Println("length of s.requests: ", len(s.requests))

	for i, item := range results {
		// fmt.Println("이게 Process()안에서 몇번째 iteration이냐면: ", k)
		// k++
		// If the item was not requested, bail out
		request := s.requests[item.Hash]
		// fmt.Println("item.Hash: ", item.Hash.Hex())
		// fmt.Println("request from s.requests[item.Hash]: ", request)
		if request == nil {
			return committed, i, ErrNotRequested
		}
		if request.data != nil {
			return committed, i, ErrAlreadyProcessed
		}
		// If the item is a raw entry request, commit directly
		if request.raw {
			fmt.Println("request.raw: ")
			request.data = item.Data
			s.commit(request)
			committed = true
			continue
		}
		// Decode the node data content and update the request
		node, err := decodeNode(item.Hash[:], item.Data)
		if err != nil {
			return committed, i, err
		}
		request.data = item.Data

		// Create and schedule a request for all the children nodes
		// fmt.Println("node: ", node)
		requests, err := s.children(request, node)
		if err != nil {
			return committed, i, err
		}
		if len(requests) == 0 && request.deps == 0 {
			// fmt.Println("len(requests) == 0 (즉 노드의 children이 없는 경우) && request.deps == 0")
			s.commit(request)
			committed = true
			continue
		}
		request.deps += len(requests)
		// fmt.Println("요청하고자하는 child node의 개수: ", len(requests))
		for i, child := range requests {
			// fmt.Println("Child Number: ", i)
			fmt.Printf("\t\tCHILD HASH data of the Node, Child Number#: %d ------------------------------- \n", i)
			s.schedule(child)
		}
		logQueuePushNewLine()
	}
	return committed, 0, nil
}

// Commit flushes the data stored in the internal membatch out to persistent
// storage, returning the number of items written and any occurred error.
func (s *Sync) Commit(dbw ethdb.KeyValueWriter) (int, error) {
	fmt.Printf("\t\t\t\tCommit() from membatch to Database -------------------------------\n")

	var totalWriteTime time.Duration

	// Dump the membatch into a database dbw
	for i, key := range s.membatch.order {s
		start1 := time.Now()
		if err := dbw.Put(key[:], s.membatch.batch[key]); err != nil {
			return i, err
		}
		elapsed1 := time.Since(start1)
		fmt.Printf("\t\t\t\tDB Write time for %s'th key %x: %s\n",i , key, elapsed1)
		totalWriteTime += elapsed1
		s.bloom.Add(key[:])
	}
	written := len(s.membatch.order) // TODO(karalabe): could an order change improve write performance?
	fmt.Printf("\t\t\t\tTotal DB Write time for one Commit(): %s writing %s number of keys\n", totalWriteTime, written)

	// Drop the membatch data and return
	s.membatch = newSyncMemBatch()
	return written, nil
}

// Pending returns the number of state entries currently pending for download.
func (s *Sync) Pending() int {
	return len(s.requests)
}

// schedule inserts a new state retrieval request into the fetch queue. If there
// is already a pending request for this node, the new request will be discarded
// and only a parent reference added to the old one.
var tempCount = 0
func (s *Sync) schedule(req *request) {
	// If we're already requesting this node, add a new reference and stop
	// fmt.Println("schedule()")
	// fmt.Println("schedule(): ", req.hash.Hex())
	if old, ok := s.requests[req.hash]; ok {
		fmt.Println("inside old check")
		// os.Exit(1)
		old.parents = append(old.parents, req.parents...)
		return
	}
	prefixLen := PrefixLength*2 +2// = blockNumberLen + "0x"

	// Schedule the request for future retrieval
	// TH/GETH (when checking benchmark, we must comment out this part)
	// priority := int64(req.depth)


    // ENHANCED (when checking benchmark, we must comment out this part)
	// Prefix := (req.hash.Hex())[0:10]
	// PrefixInt64, _ := strconv.ParseInt(Prefix, 0, 64)
	// priority := PrefixInt64

	// Depth || blockNum (concat)
	hexStringDepth := strconv.FormatInt(int64(req.depth), 16)
	if len(hexStringDepth) < 2 {
		hexStringDepth = "0" + hexStringDepth
	}
	Prefix := "0x"+hexStringDepth + (req.hash.Hex())[2:10]
	PrefixInt64, _ := strconv.ParseInt(Prefix, 0, 64)
	priority := PrefixInt64

	// BlockNum || depth (concat)
	// hexStringDepth := strconv.FormatInt(int64(req.depth), 16)
	// if len(hexStringDepth) < 2 {
	// 	hexStringDepth = "0" + hexStringDepth
	// }
	// Prefix := (req.hash.Hex())[0:10]+hexStringDepth
	// PrefixInt64, _ := strconv.ParseInt(Prefix, 0, 64)
	// priority := PrefixInt64

	fmt.Printf("\t\t\tQUEUE PUSH Hash: %s\n", req.hash.Hex())
	fmt.Printf("\t\t\tQUEUE PUSH Priority: %d\n", priority)
	start1 := time.Now()
	s.queue.Push(req.hash,priority)
	elapsed1 := time.Since(start1)
	fmt.Printf("\t\t\tQUEUE PUSH TIME: %s\n", elapsed1)
	// fmt.Println("&&&&&&&&&&&&&&&&: childnode를 queue 에 넣는 데 걸린 시간 (s.queue.Push time): ", elapsed1)
	fmt.Printf("\t\t\tQUEUE PUSH SIZE: %d \n", s.queue.Size())
	// fmt.Println("Push() 후 len(s.queue): ", s.queue.Size())
	// 싱크가 처음부터 다시 되었는지 탐지하는 코드 (mskim)
	if(req.hash.Hex() == "0x007a12008815e97fc0b9d433fffe823e25db538c37685cb68a98c83bc5f2658d"){
			tempCount++
			if(tempCount==2){
				// os.Exit(1)
			}
		}


	
	// fmt.Println("Push() 후 length of s.requests BEFORE: ", len(s.requests))
	s.requests[req.hash] = req
	// fmt.Println("Push() 후 length of s.requests AFTER: ", len(s.requests))
	logQueuePush(req.hash.Hex(), int(priority), s.queue.Size(), len(s.requests), elapsed1)

	// insert hash info for syncing with fake TH data (jmlee)
	// startTime := time.Now()
	// prefixLen := PrefixLength*2 + 2 // = blockNumberLen + "0x"

	// 전처리: realHash's suffix to FakeHash mapping
	hashWithoutPrefix := string([]rune(req.hash.Hex())[prefixLen:])
	s.hashes[hashWithoutPrefix] = req.hash

	// elapsedTime := time.Since(startTime)
	// TotalPrefixingOverhead += uint64(elapsedTime.Nanoseconds())
	// fmt.Println("hashWithoutPrefix:", hashWithoutPrefix, "/ fakeHash:", req.hash.Hex())
}

// children retrieves all the missing children of a state trie entry for future
// retrieval scheduling.
func (s *Sync) children(req *request, object node) ([]*request, error) {
	// Gather all the children of the node, irrelevant whether known or not
	type child struct {
		node  node
		depth int
	}
	var children []child

	switch node := (object).(type) {
	case *shortNode:
		children = []child{{
			node:  node.Val,
			depth: req.depth + len(node.Key),
		}}
	case *fullNode:
		for i := 0; i < 17; i++ {
			if node.Children[i] != nil {
				children = append(children, child{
					node:  node.Children[i],
					depth: req.depth + 1,
				})
			}
		}
	default:
		panic(fmt.Sprintf("unknown node: %+v", node))
	}
	// Iterate over the children, and request all unknown ones
	requests := make([]*request, 0, len(children))
	for _, child := range children {
		// Notify any external watcher of a new key/value node
		if req.callback != nil {
			if node, ok := (child.node).(valueNode); ok {
				if err := req.callback(node, req.hash); err != nil {
					return nil, err
				}
			}
		}
		// If the child references another node, resolve or schedule
		if node, ok := (child.node).(hashNode); ok {
			// Try to resolve the node from the local database
			hash := common.BytesToHash(node)
			if _, ok := s.membatch.batch[hash]; ok {
				continue
			}
			if s.bloom.Contains(node) {
				// Bloom filter says this might be a duplicate, double check
				if ok, _ := s.database.Has(node); ok {
					continue
				}
				// False positive, bump fault meter
				bloomFaultMeter.Mark(1)
			}
			// Locally unknown node, schedule for retrieval
			requests = append(requests, &request{
				hash:     hash,
				parents:  []*request{req},
				depth:    child.depth,
				callback: req.callback,
			})
		}
	}
	return requests, nil
}

// commit finalizes a retrieval request and stores it into the membatch. If any
// of the referencing parent requests complete due to this commit, they are also
// committed themselves.
func (s *Sync) commit(req *request) (err error) {
	fmt.Printf("\t\tThis Node has no CHILD, hence directly entering commit() -------------------------------\n")
	// Write the node content to the membatch
	s.membatch.batch[req.hash] = req.data
	s.membatch.order = append(s.membatch.order, req.hash)

	delete(s.requests, req.hash)
	fmt.Printf("\t\t\tCOMMIT HASH: %s\n", (req.hash).Hex())
	fmt.Printf("\t\t\tCOMMIT HASH DEPTH: %d\n", int64(req.depth) )
	fmt.Printf("\t\t\tCOMMIT REQUEST SIZE: %d\n", len(s.requests))
	// fmt.Println("***AFTER: commit(), length of s.requests: ", len(s.requests))

	common.TotalCommit++
	fmt.Printf("\t\t\tCOMMIT ACCUMULATE COUNT: %d\n", common.TotalCommit)

	// fmt.Println("Total commit(Leaf node)횟수: ", common.TotalCommit, "Queue depth: ", int64(req.depth))

	// delete hash info for syncing with fake TH data (jmlee)
	startTime := time.Now()
	prefixLen := PrefixLength*2 + 2 // = blockNumberLen + "0x"
	hashWithoutPrefix := string([]rune(req.hash.Hex())[prefixLen:])
	delete(s.hashes, hashWithoutPrefix)
	elapsedTime := time.Since(startTime)
	TotalPrefixingOverhead += uint64(elapsedTime.Nanoseconds())

	// Check all parents for completion
	// fmt.Println("commit 과정에서 노드의 parent에 대해서")
	// fmt.Println("몇개 parent 남았지?: length of req.parents: ", len(req.parents))
	for _, parent := range req.parents {
		parent.deps--
		if parent.deps == 0 {
			fmt.Println("\t\t자녀가 commit됨으로 인해 parent가 더이상 reference하는게 없으면, parent도 commit 시킨다")
			if err := s.commit(parent); err != nil {
				return err
			}
		}
	}
	return nil
}

// forcely change trie node's hash value for syncing with fake TH data (jmlee)
// 후처리 : realHash-> fakeHash
func (s *Sync) PrefixingTrieNodeHash(resHash *common.Hash) {
	
	prefixLen := PrefixLength*2 + 2 // = blockNumberLen + "0x"
	subRune1 := []rune(resHash.Hex())
	subStr1 := string(subRune1[prefixLen:])

	if fakeHash, ok := s.hashes[subStr1]; ok {
		// fmt.Println("matched!")
		// fmt.Println("resHash:", resHash.Hex(), "/ fakeHash:", fakeHash.Hex())
		*resHash = fakeHash
		return
	} else {
		// 전처리가 안된 부분을 탐지하는 코드 (mskim)
		fmt.Println("PrefixingTrieNodeHash() not matched's subStr1: ", subStr1)
		fmt.Println("PrefixingTrieNodeHash() not matched's resHash: ", resHash.Hex())
		fmt.Println("PrefixingTrieNodeHash() not matched's resHash: ", fakeHash )
	}
	
}
