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

func check(err error) {
	if err != nil {
		panic(err)
	}
}

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
func getFilesList(
	dir string,
	params *SearchParams,
	conn *ftp.ServerConn,
) ([]string, error) {
	entries, err := conn.NameList(dir)
	if err != nil {
		return []string{}, err
	}

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
			childs, err := getFilesList(absname, params, conn)
			if err != nil {
				return []string{}, err
			}

			for _, child := range childs {
				resultFiles = append(resultFiles, child)
			}
		}
	}
	return resultFiles, nil
}

// Download file from FTP and return local path.
func download(ftpPath string, conn *ftp.ServerConn, lock *sync.Mutex) (string, error) {
	// TODO: Search in cache
	lock.Lock()
	defer lock.Unlock()

	response, err := conn.Retr(ftpPath)
	if err != nil {
		return "", err
	}
	defer response.Close()

	f, err := ioutil.TempFile("", path.Base(ftpPath))
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, err = io.Copy(f, response)
	if err != nil {
		return "", err
	}

	return f.Name(), nil
}

// Save buffer content to XML file and return file path
func saveXmlResult(buf *bytes.Buffer, name string) (string, error) {
	f, err := ioutil.TempFile("", path.Base(name))
	if err != nil {
		return "", err
	}
	defer f.Close()
	io.Copy(f, buf)
	return f.Name(), nil
}

func processXml(
	ftpFile string,
	entry *zip.File,
	results chan Result,
	searchParams *SearchParams,
) error {
	xmlFile, err := entry.Open()
	if err != nil {
		return err
	}
	defer xmlFile.Close()

	buf := bytes.NewBuffer(nil)
	if _, err := io.Copy(buf, xmlFile); err != nil {
		return err
	}

	xmlContent := string(buf.Bytes())
	uXmlContent := strings.ToUpper(xmlContent)
	for _, pattern := range searchParams.Patterns {
		uPattern := strings.ToUpper(pattern)
		if strings.Contains(uXmlContent, uPattern) {
			xmlFile, err := saveXmlResult(buf, entry.Name)
			if err != nil {
				return err
			}

			results <- Result{
				ZipPath: ftpFile,
				XmlName: entry.Name,
				XmlFile: xmlFile,
				Match:   pattern,
			}
			break
		}
	}
	return nil
}

func processZip(
	ftpFile string,
	results chan Result,
	conn *ftp.ServerConn,
	searchParams *SearchParams,
	connMutex *sync.Mutex,
) error {
	localFile, err := download(ftpFile, conn, connMutex)
	if err != nil {
		return err
	}
	defer os.Remove(localFile)

	zipFile, err := zip.OpenReader(localFile)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	var wg sync.WaitGroup
	for _, entry := range zipFile.File {
		wg.Add(1)
		go func() {
			processXml(ftpFile, entry, results, searchParams)
			wg.Done()
		}()
	}
	wg.Wait()
	return nil
}

// Search over FTP->ZIP->XML files by given params and put results to
// returned channel.
func Search(searchParams *SearchParams) (chan Result, error) {
	conn, err := ftp.Connect("ftp.zakupki.gov.ru:21")
	if err != nil {
		return nil, err
	}
	if err := conn.Login("free", "free"); err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	results := make(chan Result)
	connLock := &sync.Mutex{}

	filesList, err := getFilesList(searchParams.Directory, searchParams, conn)
	if err != nil {
		return nil, err
	}
	for _, ftpFile := range filesList {
		wg.Add(1)
		go func() {
			err = processZip(ftpFile, results, conn, searchParams, connLock)
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(results)
		conn.Quit()
	}()

	return results, nil
}
