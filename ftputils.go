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
	XmlFile string
	Match   string
}

// Cache singleton.
var cache = NewCache("/tmp/CACHE", 100)

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

// Save buffer content to XML file and return file path
func saveXmlResult(buf *bytes.Buffer, name string) string {
	f, err := ioutil.TempFile("", path.Base(name))
	checkError(err)
	defer f.Close()
	_, err = io.Copy(f, buf)
	checkError(err)
	return f.Name()
}

func processXml(ftpFile string, entry *zip.File, results chan Result, searchParams *SearchParams) {
	xmlFile, err := entry.Open()
	checkError(err)
	defer xmlFile.Close()

	buf := bytes.NewBuffer(nil)
	_, err = io.Copy(buf, xmlFile)
	checkError(err)

	xmlContent := string(buf.Bytes())
	uXmlContent := strings.ToUpper(xmlContent)
	for _, pattern := range searchParams.Patterns {
		uPattern := strings.ToUpper(pattern)
		if strings.Contains(uXmlContent, uPattern) {
			results <- Result{
				ZipPath: ftpFile,
				XmlName: entry.Name,
				XmlFile: saveXmlResult(buf, entry.Name),
				Match:   pattern,
			}
			break
		}
	}
}

func processZip(
	ftpFile string,
	results chan Result,
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
				processXml(ftpFile, entry, results, searchParams)
				wg.Done()
			}(entry)
		}
	}
	wg.Wait()
}

// Search over FTP->ZIP->XML files by given params and put results to
// returned channel.
func Search(searchParams *SearchParams) chan Result {
	conn, err := ftp.Connect("ftp.zakupki.gov.ru:21")
	checkError(err)
	err = conn.Login("free", "free")
	checkError(err)

	var wg sync.WaitGroup
	results := make(chan Result)
	connLock := &sync.Mutex{}

	dir := searchParams.Directory
	for _, ftpFile := range getFilesList(dir, searchParams, conn) {
		wg.Add(1)
		go func(ftpFile string) {
			processZip(ftpFile, results, conn, searchParams, connLock)
			wg.Done()
		}(ftpFile)
	}

	go func() {
		wg.Wait()
		close(results)
		conn.Quit()
	}()
	return results
}
