package main

import (
	"archive/zip"
	"bytes"
	"github.com/jlaffaye/ftp"
	"io"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Search params.
type SearchParams struct {
	Directory string
	FromDate  string
	ToDate    string
	Patterns  []string
}

// Result returned by search function.
type Result struct {
	ZipPath string
	XmlName string
	XmlFile *bytes.Buffer
	Match   string
}

// Cache singleton.
var cache = NewCache(Settings.CachePath, Settings.CacheSize)

// Precompiled regex with zip dates.
// For isFileMatchDates().
var zipFileDatesRegexp = regexp.MustCompile(
	"(20[0-9]{6})[0-9]{2}_(20[0-9]{6})[0-9]{2}")

// Most of ZIP-files have names like this:
// contract_Tatarstan_Resp_YYYYmmdd??_YYYYmmdd??_???.zip.
// Using YYYYmmdd name parts for filtering by dates.
// If provided ZIP name doesn't contain any dates, return true anyway.
func isFileMatchDates(searchParams *SearchParams, fileName string) bool {
	result := true
	matches := zipFileDatesRegexp.FindAllStringSubmatch(fileName, -1)
	if len(matches) > 0 && len(matches[0]) == 3 {
		minDate := matches[0][1]
		maxDate := matches[0][2]

		minFilter := searchParams.FromDate
		if minFilter == "" {
			minFilter = "00000000"
		}
		maxFilter := searchParams.ToDate
		if maxFilter == "" {
			maxFilter = "99999999"
		}

		result =
			minFilter <= minDate && minDate <= maxFilter ||
				minFilter <= maxDate && maxDate <= maxFilter ||
				minDate <= minFilter && maxFilter <= maxDate
	}
	return result
}

// Return all (matching) ZIP-files fro given directory. Recursively.
func getFilesList(dir string, params *SearchParams, conn *ftp.ServerConn) []string {
	entries, err := conn.NameList(dir)
	checkError(err)

	resultFiles := make([]string, 0)
	for _, entry := range entries {
		name := path.Base(entry)
		absname := path.Join(dir, name)
		// We assume that the files have an extensions whereas
		// directories have not. Best way to check that - try to `cd'
		// into file/dir.
		if strings.Contains(name, ".") {
			if isFileMatchDates(params, name) {
				resultFiles = append(resultFiles, absname)
			}
		} else {
			for _, child := range getFilesList(absname, params, conn) {
				resultFiles = append(resultFiles, child)
			}
		}
	}
	return resultFiles
}

// Download file from FTP and return local path.
func download(ftpPath string, conn *ftp.ServerConn, lock *sync.Mutex) string {
	var result string
	cachedFile := cache.Get(ftpPath)
	if cachedFile != "" {
		result = cachedFile
	} else {
		// All operations with `conn' from separate goroutines should be protected by mutex.
		lock.Lock()
		defer lock.Unlock()

		response, err := conn.Retr(ftpPath)
		checkError(err)
		defer response.Close()

		f, err := ioutil.TempFile("", path.Base(ftpPath))
		checkError(err)
		defer f.Close()

		_, err = io.Copy(f, response)
		checkError(err)

		cache.Store(ftpPath, f.Name())
		result = f.Name()
	}

	return result
}

func processXml(ftpFile string, entry *zip.File, results *Channel, searchParams *SearchParams) {
	xmlFile, err := entry.Open()
	checkError(err)
	defer xmlFile.Close()

	buf := bytes.NewBuffer(nil)
	_, err = io.Copy(buf, xmlFile)
	checkError(err)

	foundPattern := searchPatterns(buf.Bytes(), searchParams.Patterns)
	if foundPattern != "" {
		results.Write(&Result{
			ZipPath: ftpFile,
			XmlName: entry.Name,
			XmlFile: buf,
			Match:   foundPattern,
		})
	}
}

func processZip(
	ftpFile string,
	results *Channel,
	conn *ftp.ServerConn,
	searchParams *SearchParams,
	connMutex *sync.Mutex,
) {
	localFile := download(ftpFile, conn, connMutex)
	defer os.Remove(localFile)

	zipFile, err := zip.OpenReader(localFile)
	checkError(err)
	defer zipFile.Close()

	var wg sync.WaitGroup
	for _, entry := range zipFile.File {
		if strings.HasSuffix(entry.Name, ".xml") {
			wg.Add(1)
			go func(entry *zip.File) {
				if !results.Closed {
					processXml(ftpFile, entry, results, searchParams)
				}
				wg.Done()
			}(entry)
		}
	}
	wg.Wait()
}

// Search over FTP->ZIP->XML files by given params and put results to
// returned channel.
func Search(searchParams *SearchParams) *Channel {
	conn, err := ftp.Connect("ftp.zakupki.gov.ru:21")
	checkError(err)
	err = conn.Login("free", "free")
	checkError(err)

	var wg sync.WaitGroup
	results := NewChannel()
	connLock := &sync.Mutex{}

	filesList := getFilesList(searchParams.Directory, searchParams, conn)
	// Sort by presence in cache: cached files first.
	sort.Slice(filesList, func(i, j int) bool {
		x, y := filesList[i], filesList[j]
		return cache.Has(x) && !cache.Has(y)
	})

	for _, ftpFile := range filesList {
		wg.Add(1)
		go func(ftpFile string) {
			if !results.Closed {
				processZip(ftpFile, results, conn, searchParams, connLock)
			}
			wg.Done()
		}(ftpFile)
	}

	go func() {
		wg.Wait()
		close(results.C)
		conn.Quit()
	}()
	return results
}

// Search given patterns in buffer.
// Do case insensitive search if it required.
// Return first found pattern or empty string.
func searchPatterns(buf []byte, patterns []string) string {
	var result string
	// Lazy initialization
	var uBuf []byte // uppercase buffer
	for _, pattern := range patterns {
		uPattern, lPattern := strings.ToUpper(pattern), strings.ToLower(pattern)
		if uPattern == lPattern {
			// Patterns are case insensitive.
			found := bytes.Contains(buf, []byte(pattern))
			if found {
				result = pattern
				break
			}
		} else {
			// Patterns are case sensitive. Make it lowercase before compare.
			if uBuf == nil {
				uBuf = bytes.ToLower(buf)
			}
			found := bytes.Contains(uBuf, []byte(lPattern))
			if found {
				result = pattern
				break
			}
		}
	}
	return result
}
