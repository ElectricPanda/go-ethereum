// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"fmt"
	// "strconv"
	"sync/atomic"
	"time"
	"unsafe"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type tSet struct {
	level int
	table *tFile
}

type version struct {
	id int64 // unique monotonous increasing version id
	s  *session

	levels []tFiles

	// Level that should be compacted next and its compaction score.
	// Score < 1 means compaction is not strictly needed. These fields
	// are initialized by computeCompaction()
	cLevel int
	cScore float64

	cSeek unsafe.Pointer

	closing  bool
	ref      int
	released bool
}

// newVersion creates a new version with an unique monotonous increasing id.
func newVersion(s *session) *version {
	id := atomic.AddInt64(&s.ntVersionId, 1)
	nv := &version{s: s, id: id - 1}
	return nv
}

func (v *version) incref() {
	if v.released {
		panic("already released")
	}

	v.ref++
	if v.ref == 1 {
		select {
		case v.s.refCh <- &vTask{vid: v.id, files: v.levels, created: time.Now()}:
			// We can use v.levels directly here since it is immutable.
		case <-v.s.closeC:
			v.s.log("reference loop already exist")
		}
	}
}

func (v *version) releaseNB() {
	v.ref--
	if v.ref > 0 {
		return
	} else if v.ref < 0 {
		panic("negative version ref")
	}
	select {
	case v.s.relCh <- &vTask{vid: v.id, files: v.levels, created: time.Now()}:
		// We can use v.levels directly here since it is immutable.
	case <-v.s.closeC:
		v.s.log("reference loop already exist")
	}

	v.released = true
}

func (v *version) release() {
	v.s.vmu.Lock()
	v.releaseNB()
	v.s.vmu.Unlock()
}

// write leveldb table log to the file (jmlee)
func writeTableLog(log string) {
	// f, err := os.OpenFile("./experiment/impt_leveldb_table_log.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	f, err := os.OpenFile("/home/minseokk1m/TH-PriorityQueueEnhance/logs/impt_leveldb_table_log.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("ERR:", err)
		// log.Info("ERR", "err", err)
	}
	fmt.Fprintln(f, log)
	f.Close()
}

func (v *version) walkOverlapping(aux tFiles, ikey internalKey, f func(level int, t *tFile) bool, lf func(level int) bool) {
	ukey := ikey.ukey()

	// Aux level.
	// aux = nil로 들어오니까 무시하자
	if aux != nil {
		for i, t := range aux {
			// fmt.Println("Aux: " + strconv.Itoa(i))
			_ = i
			if t.overlaps(v.s.icmp, ukey, ukey) {
				if !f(-1, t) {
					return
				}
			}
		}

		if lf != nil && !lf(-1) {
			return
		}
	}

	// Walk tables level-by-level.
	// fmt.Println(v.levels)
	for level, tables := range v.levels {
		// fmt.Println("------------------------------------------------------")
		// fmt.Println("[level: " + strconv.Itoa(level) + ", tablelen: " + strconv.Itoa(len(tables)) + "]")
		// writeTableLog("level: " + strconv.Itoa(level) + ", tablelen: " + strconv.Itoa(len(tables)))	// write leveldb table log (jmlee)

		if len(tables) == 0 {
			continue
		}

		// for i, t := range tables {
		// 	fmt.Println("table " + strconv.Itoa(i))
		// 	fmt.Println("fd: " + t.fd.String())
		// 	fmt.Println("size: " + strconv.FormatInt(t.size, 10))
		// 	fmt.Println("range: (" + string(t.imin) + ", " + string(t.imax) + ")")
		// }

		if level == 0 {
			// Level-0 files may overlap each other. Find all files that
			// overlap ukey.
			for _, t := range tables {
				if t.overlaps(v.s.icmp, ukey, ukey) {
					if !f(level, t) {
						// f 함수의 결과가 false면 탐색을 끝냄
						return
					}
				}
			}
		} else {
			if i := tables.searchMax(v.s.icmp, ikey); i < len(tables) {
				t := tables[i]
				if v.s.icmp.uCompare(ukey, t.imin.ukey()) >= 0 {
					if !f(level, t) {
						return
					}
				}
			}
		}

		if lf != nil && !lf(level) {
			// level0에서 원하던 값을 찾았었으면 이제 찾은 것으로 확정 짓고 끝냄
			return
		}
		// 못찾았으면 다음 level 탐색하러 감
	}
}

// db.Get()에서 값을 찾으려 하는데 memory에서 못찾으면 disk에서 찾기 위해 이 함수를 부르게 됨
// aux = nil, ro = nil, noValue = false를 넣어 호출
func (v *version) get(aux tFiles, ikey internalKey, ro *opt.ReadOptions, noValue bool) (value []byte, tcomp bool, err error) {
	// 만약 version이 닫혔으면? 닫히는 중이면? 아무튼 못찾았다고 하나봄, 넘어가고
	if v.closing {
		return nil, false, ErrClosed
	}

	// 찾고자하는 key를 db.Get()에서 internalKey로 바꾸고 그걸 또 ukey라는 형태로 바꿈
	ukey := ikey.ukey()
	sampleSeeks := !v.s.o.GetDisableSeeksCompaction()

	var (
		tset  *tSet
		tseek bool

		// Level-0.
		zfound bool
		zseq   uint64
		zkt    keyType
		zval   []byte
	)

	err = ErrNotFound

	// Since entries never hop across level, finding key/value
	// in smaller level make later levels irrelevant.
	// aux = nil로 들어오니까 무시하자
	// var foundTFile *tFile
	// var foundLevel int

	v.walkOverlapping(aux, ikey, func(level int, t *tFile) bool {
		// 이게 walkOverlapping 함수에서 f 함수인데 뭐하는 함수냐면
		// 해당 level에 있는 table 하나에서 찾고자 하는 값을 서치하는 코드임
		// 이 함수를 모든 레벨에 있는 모든 table 마다 실행시키는 식으로 iterate 하면서 찾는거임
		if sampleSeeks && level >= 0 && !tseek {
			if tset == nil {
				tset = &tSet{level, t}
			} else {
				tseek = true
			}
		}

		var (
			fikey, fval []byte
			ferr        error
		)
		if noValue {
			fikey, ferr = v.s.tops.findKey(t, ikey, ro)
		} else {
			// noValue = false 라서 이게 실행됨
			// tops는 tOps 타입인데 table operations 를 뜻함
			// tops.find() 함수 설명 보면 이렇게 나옴
			// Finds key/value pair whose key is greater than or equal to the given key.
			if t != nil {
				hexKey := fmt.Sprintf("0x%x", ukey)
				common.WriteToMap(hexKey, level, t.fd.Num)
			}
			fikey, fval, ferr = v.s.tops.find(t, ikey, ro)
		}

		switch ferr {
		case nil:
			// v.s.tops.find() 함수에서 해당 ikey보다 더 크거나 같은 key/value pair를 return 한다는데
			// 이게 ferr 안일어나고 성공했으면 이거 가지고서 밑에서 한번 찾아보는듯
			// table에서 찾으려는 ikey 보다 더 큰값이 있다는건 해당 range 안에 들어있다는 거니까
			// 밑에서 한번 찾아보려고 하나봄
		case ErrNotFound:
			// walkOverlapping 함수에서 보면 f가 false 면 함수를 종료하던데
			// 이렇게 못찾았다고 ErrNotFound가 나왔을 때 true를 return 하니까
			// 여긴 없구나, 다른 곳으로 찾으러가자 이런 의미로 true를 return 시키는거 같음
			return true
		default:
			// 문제가 있으면 f를 false로 만들어서
			// walkOverlapping 함수를 종료시켜버림
			// 에러가 났으니까 찾고 자시고 간에 그냥 문제 있다고 종료시키는거 같음
			err = ferr
			return false
		}

		// 자 위에서 v.s.tops.find()에서 가져온 정보를 바탕으로 한번 보자
		// 
		if fukey, fseq, fkt, fkerr := parseInternalKey(fikey); fkerr == nil {
			// ukey가 내가 찾고자 하는 key값이고
			// fukey가 find()함수를 통해 가져온 값인데 이걸 비교해보고 같은지 아닌지를 보는듯
			// 둘이 같다면 원하던 값을 찾아냈다는 거지
			// (ukey <= fukey 니까)
			if v.s.icmp.uCompare(ukey, fukey) == 0 {
				// Level <= 0 may overlaps each-other.
				if level <= 0 {
					// 찾아낸게 level0에서라면 잠시 저장해두고서 일단 넘어가는듯
					// level0에서 찾은 경우에는 다 sort 되어 있는게 아니고 뭔가 overlapping 이 있을 수 있기에
					// 이런 식으로 하는건가? 
					if fseq >= zseq {
						zfound = true
						zseq = fseq
						zkt = fkt
						zval = fval
					}
				} else {
					// level0에서 찾았을 때와는 다르게
					// level1 이상에서 원하던 값을 찾았다면 찾은 것으로 확정지어버리는 듯
					switch fkt {
					case keyTypeVal:
						// fmt.Println("@@FIND THE VALUE!: at level", level) // 여긴 level1 이상이 나옴
						// logLevelInfo(level, ukey)
						value = fval
						err = nil
						// common.LevelDBNodeFrom = level
						// foundTFile = t // Store the table where the key was found
						// foundLevel = level // Store the level where the key was found
					case keyTypeDel:
					default:
						panic("leveldb: invalid internalKey type")
					}
					// 찾았던 오류가 났던간에 아무튼 walkOverlapping 함수를 종료시켜 탐색 끝냄
					return false
				}
			}
		} else {
			// parseInternalKey 함수가 에러가 났으면 fikey가 뭔가 이상한거니까
			// 그냥 false 를 return 해서 walkOverlapping 함수를 종료시켜 탐색 끝냄
			err = fkerr
			return false
		}

		// fukey가 찾고자 하던 ukey 보다 큰 경우 / level0에서 찾던 값이 있엇다면 
		// walkOverlapping를 통한 탐색을 계속 진행시킴
		return true

	}, func(level int) bool {
		if zfound {
			switch zkt {
			case keyTypeVal:
				// fmt.Println("@FIND THE VALUE!: at level", level) // 여긴 level0가 나옴
				// logLevelInfo(level, ukey)
				value = zval
				err = nil
				// common.LevelDBNodeFrom = level
			case keyTypeDel:
			default:
				panic("leveldb: invalid internalKey type")
			}
			// 원하는 값을 찾았던 / 뭔가의 에러가 있어 못찾았던 간에 
			// return false 를 통해 walkOverlapping 함수를 종료시켜버림
			return false
		}

		// level0에서 원하던 값을 찾은게 아니라면 true를 return해 walkOverlapping 함수를 통해 계속 탐색시킴
		return true
	})

	// if foundTFile != nil {
	// 	hexKey := fmt.Sprintf("0x%x", ukey)
	// 	common.WriteToMap(hexKey, foundLevel, foundTFile.fd.Num)
	// }

	if tseek && tset.table.consumeSeek() <= 0 {
		tcomp = atomic.CompareAndSwapPointer(&v.cSeek, nil, unsafe.Pointer(tset))
	}

	return
}

func (v *version) sampleSeek(ikey internalKey) (tcomp bool) {
	var tset *tSet

	v.walkOverlapping(nil, ikey, func(level int, t *tFile) bool {
		if tset == nil {
			tset = &tSet{level, t}
			return true
		}
		if tset.table.consumeSeek() <= 0 {
			tcomp = atomic.CompareAndSwapPointer(&v.cSeek, nil, unsafe.Pointer(tset))
		}
		return false
	}, nil)

	return
}

func (v *version) getIterators(slice *util.Range, ro *opt.ReadOptions) (its []iterator.Iterator) {
	strict := opt.GetStrict(v.s.o.Options, ro, opt.StrictReader)
	for level, tables := range v.levels {
		if level == 0 {
			// Merge all level zero files together since they may overlap.
			for _, t := range tables {
				its = append(its, v.s.tops.newIterator(t, slice, ro))
			}
		} else if len(tables) != 0 {
			its = append(its, iterator.NewIndexedIterator(tables.newIndexIterator(v.s.tops, v.s.icmp, slice, ro), strict))
		}
	}
	return
}

func (v *version) newStaging() *versionStaging {
	return &versionStaging{base: v}
}

// Spawn a new version based on this version.
func (v *version) spawn(r *sessionRecord, trivial bool) *version {
	staging := v.newStaging()
	staging.commit(r)
	return staging.finish(trivial)
}

func (v *version) fillRecord(r *sessionRecord) {
	for level, tables := range v.levels {
		for _, t := range tables {
			r.addTableFile(level, t)
		}
	}
}

func (v *version) tLen(level int) int {
	if level < len(v.levels) {
		return len(v.levels[level])
	}
	return 0
}

func (v *version) offsetOf(ikey internalKey) (n int64, err error) {
	for level, tables := range v.levels {
		for _, t := range tables {
			if v.s.icmp.Compare(t.imax, ikey) <= 0 {
				// Entire file is before "ikey", so just add the file size
				n += t.size
			} else if v.s.icmp.Compare(t.imin, ikey) > 0 {
				// Entire file is after "ikey", so ignore
				if level > 0 {
					// Files other than level 0 are sorted by meta->min, so
					// no further files in this level will contain data for
					// "ikey".
					break
				}
			} else {
				// "ikey" falls in the range for this table. Add the
				// approximate offset of "ikey" within the table.
				if m, err := v.s.tops.offsetOf(t, ikey); err == nil {
					n += m
				} else {
					return 0, err
				}
			}
		}
	}

	return
}

func (v *version) pickMemdbLevel(umin, umax []byte, maxLevel int) (level int) {
	if maxLevel > 0 {
		if len(v.levels) == 0 {
			return maxLevel
		}
		if !v.levels[0].overlaps(v.s.icmp, umin, umax, true) {
			var overlaps tFiles
			for ; level < maxLevel; level++ {
				if pLevel := level + 1; pLevel >= len(v.levels) {
					return maxLevel
				} else if v.levels[pLevel].overlaps(v.s.icmp, umin, umax, false) {
					break
				}
				if gpLevel := level + 2; gpLevel < len(v.levels) {
					overlaps = v.levels[gpLevel].getOverlaps(overlaps, v.s.icmp, umin, umax, false)
					if overlaps.size() > int64(v.s.o.GetCompactionGPOverlaps(level)) {
						break
					}
				}
			}
		}
	}
	return
}

func (v *version) computeCompaction() {
	// Precomputed best level for next compaction
	bestLevel := int(-1)
	bestScore := float64(-1)

	statFiles := make([]int, len(v.levels))
	statSizes := make([]string, len(v.levels))
	statScore := make([]string, len(v.levels))
	statTotSize := int64(0)

	for level, tables := range v.levels {
		var score float64
		size := tables.size()
		if level == 0 {
			// We treat level-0 specially by bounding the number of files
			// instead of number of bytes for two reasons:
			//
			// (1) With larger write-buffer sizes, it is nice not to do too
			// many level-0 compaction.
			//
			// (2) The files in level-0 are merged on every read and
			// therefore we wish to avoid too many files when the individual
			// file size is small (perhaps because of a small write-buffer
			// setting, or very high compression ratios, or lots of
			// overwrites/deletions).
			score = float64(len(tables)) / float64(v.s.o.GetCompactionL0Trigger())
		} else {
			score = float64(size) / float64(v.s.o.GetCompactionTotalSize(level))
		}

		if score > bestScore {
			bestLevel = level
			bestScore = score
		}

		statFiles[level] = len(tables)
		statSizes[level] = shortenb(int(size))
		statScore[level] = fmt.Sprintf("%.2f", score)
		statTotSize += size
	}

	v.cLevel = bestLevel
	v.cScore = bestScore

	v.s.logf("version@stat F·%v S·%s%v Sc·%v", statFiles, shortenb(int(statTotSize)), statSizes, statScore)
}

func (v *version) needCompaction() bool {
	return v.cScore >= 1 || atomic.LoadPointer(&v.cSeek) != nil
}

type tablesScratch struct {
	added   map[int64]atRecord
	deleted map[int64]struct{}
}

type versionStaging struct {
	base   *version
	levels []tablesScratch
}

func (p *versionStaging) getScratch(level int) *tablesScratch {
	if level >= len(p.levels) {
		newLevels := make([]tablesScratch, level+1)
		copy(newLevels, p.levels)
		p.levels = newLevels
	}
	return &(p.levels[level])
}

func (p *versionStaging) commit(r *sessionRecord) {
	// Deleted tables.
	for _, r := range r.deletedTables {
		scratch := p.getScratch(r.level)
		if r.level < len(p.base.levels) && len(p.base.levels[r.level]) > 0 {
			if scratch.deleted == nil {
				scratch.deleted = make(map[int64]struct{})
			}
			scratch.deleted[r.num] = struct{}{}
		}
		if scratch.added != nil {
			delete(scratch.added, r.num)
		}
	}

	// New tables.
	for _, r := range r.addedTables {
		scratch := p.getScratch(r.level)
		if scratch.added == nil {
			scratch.added = make(map[int64]atRecord)
		}
		scratch.added[r.num] = r
		if scratch.deleted != nil {
			delete(scratch.deleted, r.num)
		}
	}
}

func (p *versionStaging) finish(trivial bool) *version {
	// Build new version.
	nv := newVersion(p.base.s)
	numLevel := len(p.levels)
	if len(p.base.levels) > numLevel {
		numLevel = len(p.base.levels)
	}
	nv.levels = make([]tFiles, numLevel)
	for level := 0; level < numLevel; level++ {
		var baseTabels tFiles
		if level < len(p.base.levels) {
			baseTabels = p.base.levels[level]
		}

		if level < len(p.levels) {
			scratch := p.levels[level]

			// Short circuit if there is no change at all.
			if len(scratch.added) == 0 && len(scratch.deleted) == 0 {
				nv.levels[level] = baseTabels
				continue
			}

			var nt tFiles
			// Prealloc list if possible.
			if n := len(baseTabels) + len(scratch.added) - len(scratch.deleted); n > 0 {
				nt = make(tFiles, 0, n)
			}

			// Base tables.
			for _, t := range baseTabels {
				if _, ok := scratch.deleted[t.fd.Num]; ok {
					continue
				}
				if _, ok := scratch.added[t.fd.Num]; ok {
					continue
				}
				nt = append(nt, t)
			}

			// Avoid resort if only files in this level are deleted
			if len(scratch.added) == 0 {
				nv.levels[level] = nt
				continue
			}

			// For normal table compaction, one compaction will only involve two levels
			// of files. And the new files generated after merging the source level and
			// source+1 level related files can be inserted as a whole into source+1 level
			// without any overlap with the other source+1 files.
			//
			// When the amount of data maintained by leveldb is large, the number of files
			// per level will be very large. While qsort is very inefficient for sorting
			// already ordered arrays. Therefore, for the normal table compaction, we use
			// binary search here to find the insert index to insert a batch of new added
			// files directly instead of using qsort.
			if trivial && len(scratch.added) > 0 {
				added := make(tFiles, 0, len(scratch.added))
				for _, r := range scratch.added {
					added = append(added, tableFileFromRecord(r))
				}
				if level == 0 {
					added.sortByNum() // Sorts tables by file number in descending order.
					index := nt.searchNumLess(added[len(added)-1].fd.Num)
					nt = append(nt[:index], append(added, nt[index:]...)...)
				} else {
					added.sortByKey(p.base.s.icmp) // Sorts tables by key in ascending order.
					_, amax := added.getRange(p.base.s.icmp)
					index := nt.searchMin(p.base.s.icmp, amax)
					nt = append(nt[:index], append(added, nt[index:]...)...)
				}
				nv.levels[level] = nt
				continue
			}

			// New tables.
			for _, r := range scratch.added {
				nt = append(nt, tableFileFromRecord(r))
			}

			if len(nt) != 0 {
				// Sort tables.
				if level == 0 {
					nt.sortByNum()
				} else {
					nt.sortByKey(p.base.s.icmp)
				}

				nv.levels[level] = nt
			}
		} else {
			nv.levels[level] = baseTabels
		}
	}

	// Trim levels.
	n := len(nv.levels)
	for ; n > 0 && nv.levels[n-1] == nil; n-- {
	}
	nv.levels = nv.levels[:n]

	// Compute compaction score for new version.
	nv.computeCompaction()

	return nv
}

type versionReleaser struct {
	v    *version
	once bool
}

func (vr *versionReleaser) Release() {
	v := vr.v
	v.s.vmu.Lock()
	if !vr.once {
		v.releaseNB()
		vr.once = true
	}
	v.s.vmu.Unlock()
}
