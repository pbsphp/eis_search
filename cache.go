package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"sync"
	"time"
)

// Cache index file name
const indexName = "index.json"

// Cache entry for one row.
// Using "Used" property for LRU purposes.
type CacheRow struct {
	LocalName string
	Used      int64
}

// Json mapping struct.
// This is what the file contains.
type CacheJsonStruct struct {
	Rows map[string]CacheRow
}

// Public cache object struct.
type Cache struct {
	Data      *CacheJsonStruct
	lock      *sync.Mutex
	directory string
	capacity  int
}

// Create new Cache instance.
// Directory - absolute path to cache directory (/tmp for example).
// Capacity - max rows (files) in cache.
func NewCache(directory string, capacity int) *Cache {
	if capacity <= 0 {
		panic("NewCache capacity must be >= 0")
	}

	// Create cache directory if it does not exist.
	_, err := os.Stat(directory)
	if os.IsNotExist(err) {
		err = os.MkdirAll(directory, 0744)
		checkError(err)
	} else {
		checkError(err)
	}

	var jsonMapping CacheJsonStruct
	indexFile := path.Join(directory, indexName)
	_, err = os.Stat(indexFile)
	if os.IsNotExist(err) {
		jsonMapping = CacheJsonStruct{
			Rows: make(map[string]CacheRow),
		}
	} else if err == nil {
		dat, err := ioutil.ReadFile(indexFile)
		checkError(err)
		err = json.Unmarshal(dat, &jsonMapping)
		checkError(err)
	} else {
		checkError(err)
	}
	return &Cache{
		Data:      &jsonMapping,
		lock:      &sync.Mutex{},
		directory: directory,
		capacity:  capacity,
	}
}

// Find `ftpPath' file, copy it to temporary file and return it's path.
// Instead of returning cached file path, we copy it, because cached
// file can be unlinked by LRU mechanism before it was read and closed.
// Return "" if file does not exist.
// Caller code should unlink returned file after work is done.
func (cache *Cache) Get(ftpPath string) string {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	var result string
	row, ok := cache.Data.Rows[ftpPath]
	if ok {
		row.Used = time.Now().Unix()
		cache.flush()
		tempFile, err := ioutil.TempFile("", row.LocalName)
		checkError(err)
		defer tempFile.Close()
		cachedPath := path.Join(cache.directory, row.LocalName)
		err = copyFile(tempFile.Name(), cachedPath)
		checkError(err)
		result = tempFile.Name()
	}
	return result
}

// Store file in cache.
// FtpPath - absolute path to FTP file.
// LocPath - absolute path to local file to store.
func (cache *Cache) Store(ftpPath string, locPath string) {
	if cache.Has(ftpPath) {
		return
	}

	cache.lock.Lock()
	defer cache.lock.Unlock()

	// Find and delete least recently used if needed
	if len(cache.Data.Rows) >= cache.capacity {
		var minKey string
		var minUsed int64
		for key, val := range cache.Data.Rows {
			if minKey == "" || val.Used < minUsed {
				minKey, minUsed = key, val.Used
			}
		}
		delRow := cache.Data.Rows[minKey]
		os.Remove(path.Join(cache.directory, delRow.LocalName))
		delete(cache.Data.Rows, minKey)
	}

	storeFileName := fmt.Sprintf(
		"%d_%s",
		time.Now().UnixNano(),
		path.Base(locPath),
	)
	storeFilePath := path.Join(cache.directory, storeFileName)
	copyFile(storeFilePath, locPath)

	cache.Data.Rows[ftpPath] = CacheRow{
		LocalName: storeFileName,
		Used:      time.Now().Unix(),
	}
	cache.flush()
}

func (cache *Cache) Has(ftpPath string) bool {
	_, exists := cache.Data.Rows[ftpPath]
	return exists
}

// Flush data to disk
func (cache *Cache) flush() {
	data, err := json.Marshal(*cache.Data)
	checkError(err)
	dbFile := path.Join(cache.directory, indexName)
	err = ioutil.WriteFile(dbFile, data, 0644)
	checkError(err)
}

// Copy file.
// Current cache model doesn't allow in-place read of cached files
// because goroutines can try to read from files deleted by LRU.
// So Get and Store actually copies files. It's reasonably fast.
// TODO: Simple lightweight transactions or locks model.
func copyFile(dstPath, srcPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}
	return nil
}
