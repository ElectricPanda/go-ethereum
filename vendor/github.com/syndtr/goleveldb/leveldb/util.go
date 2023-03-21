// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"encoding/hex"

	"github.com/syndtr/goleveldb/leveldb/storage"
)

var LogFilePath = "/home/minseokk1m/TH-PriorityQueueEnhance/logs/th_enhance_which_level_1.txt"

func shorten(str string) string {
	if len(str) <= 8 {
		return str
	}
	return str[:3] + ".." + str[len(str)-3:]
}

var bunits = [...]string{"", "Ki", "Mi", "Gi", "Ti"}

func shortenb(bytes int) string {
	i := 0
	for ; bytes > 1024 && i < 4; i++ {
		bytes /= 1024
	}
	return fmt.Sprintf("%d%sB", bytes, bunits[i])
}

func sshortenb(bytes int) string {
	if bytes == 0 {
		return "~"
	}
	sign := "+"
	if bytes < 0 {
		sign = "-"
		bytes *= -1
	}
	i := 0
	for ; bytes > 1024 && i < 4; i++ {
		bytes /= 1024
	}
	return fmt.Sprintf("%s%d%sB", sign, bytes, bunits[i])
}

func sint(x int) string {
	if x == 0 {
		return "~"
	}
	sign := "+"
	if x < 0 {
		sign = "-"
		x *= -1
	}
	return fmt.Sprintf("%s%d", sign, x)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type fdSorter []storage.FileDesc

func (p fdSorter) Len() int {
	return len(p)
}

func (p fdSorter) Less(i, j int) bool {
	return p[i].Num < p[j].Num
}

func (p fdSorter) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func sortFds(fds []storage.FileDesc) {
	sort.Sort(fdSorter(fds))
}

func ensureBuffer(b []byte, n int) []byte {
	if cap(b) < n {
		return make([]byte, n)
	}
	return b[:n]
}

// logLevelInfo logs which level the value is found at (jmlee)
func logLevelInfo(level int, key [] byte) {
	return
	// append or write logData to file
	f, err := os.OpenFile(LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("ERR:", err)
		// log.Info("ERR", "err", err)
	}

	temp := hex.EncodeToString(key)
	if(len(temp) != 64) { 
		f.Close() 
	} else {
	logData0 := "Found key: " + hex.EncodeToString(key)
	logData1 := "Fount at level:  " + strconv.Itoa(level)
	fmt.Fprintln(f, logData0, logData1)
	f.Close()
	}
}
