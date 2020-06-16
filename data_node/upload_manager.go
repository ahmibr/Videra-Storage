package datanode

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"sync"

	"github.com/SayedAlesawy/Videra-Ingestion/orchestrator/utils/errors"
	"github.com/SayedAlesawy/Videra-Storage/config"
	"github.com/julienschmidt/httprouter"
)

// uploadManagerOnce Used to garauntee thread safety for singleton instances
var uploadManagerOnce sync.Once

// uploadManagerInstance A singleton instance of the upload manager object
var uploadManagerInstance *UploadManager

// UploadManagerInstance A function to return a singleton upload manager instance
func UploadManagerInstance() *UploadManager {
	dataNodeConfig := config.ConfigurationManagerInstance("").DataNodeConfig()

	uploadManagerOnce.Do(func() {
		uploadManager := UploadManager{
			fileBase:     make(map[string]FileInfo),
			logPrefix:    "[Upload-Manager]",
			maxChunkSize: dataNodeConfig.MaxRequestSize,
		}

		uploadManagerInstance = &uploadManager
	})

	return uploadManagerInstance
}

// Start A function to start listening
func (um *UploadManager) Start() {
	dataNodeConfig := config.ConfigurationManagerInstance("").DataNodeConfig()

	router := httprouter.New()
	router.POST("/upload", um.handleUpload)

	address := fmt.Sprintf("%s:%s", dataNodeConfig.IP, dataNodeConfig.Port)

	log.Println(um.logPrefix, fmt.Sprintf("Listening for external requests on %s", address))
	log.Fatal(http.ListenAndServe(address, router))
}

// HandleUpload is upload endpoint handler
func (um *UploadManager) handleUpload(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	reqType := r.Header.Get("Request-Type")

	switch reqType {
	case "INIT":
		um.handleInitialUpload(w, r)
	case "APPEND":
		um.handleAppendUpload(w, r)
	default:
		log.Println(um.logPrefix, r.RemoteAddr, fmt.Sprintf("request-type header value undefined - %s", reqType))
		handleRequestError(w, http.StatusBadRequest, "Request-Type header value is not undefined")
	}
}

// handleInitialUpload is a function responsible for handling the first upload request
func (um *UploadManager) handleInitialUpload(w http.ResponseWriter, r *http.Request) {
	log.Println(um.logPrefix, r.RemoteAddr, "Received INIT request")

	expectedHeaders := []string{"Filename", "Filesize"}
	err := um.validateUploadHeaders(&r.Header, expectedHeaders...)

	if err != nil {
		log.Println(um.logPrefix, r.RemoteAddr, err)
		handleRequestError(w, http.StatusBadRequest, err.Error())
		return
	}

	filesize, err := strconv.ParseInt(r.Header.Get("Filesize"), 10, 64)
	if errors.IsError(err) || filesize <= 0 {
		log.Println(um.logPrefix, r.RemoteAddr, "Error parsing file size")
		handleRequestError(w, http.StatusBadRequest, "Invalid file size")
		return
	}

	id := generateRandomString(10)
	filepath := id
	filename := r.Header.Get("Filename") // Maybe be changed later

	log.Println(um.logPrefix, r.RemoteAddr, "creating file with id", id)
	err = createFileDirectory(filepath, 0744)
	if errors.IsError(err) {
		log.Println(um.logPrefix, r.RemoteAddr, err)
		handleRequestError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	err = um.addNewFile(id, filepath, filename, filesize)
	if errors.IsError(err) {
		log.Println(um.logPrefix, r.RemoteAddr, err)
		handleRequestError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	w.Header().Set("ID", id)
	w.Header().Set("Max-Request-Size", fmt.Sprintf("%d", um.maxChunkSize))
	w.WriteHeader(http.StatusCreated)
}

// handleAppendUpload is a function responsible for handling the first upload request
func (um *UploadManager) handleAppendUpload(w http.ResponseWriter, r *http.Request) {
	log.Println(um.logPrefix, r.RemoteAddr, "Received APPEND request")
	// Content length not provided
	if r.ContentLength <= 0 {
		log.Println(um.logPrefix, r.RemoteAddr, "Content-Length header not provided")
		handleRequestError(w, http.StatusBadRequest, "Content-Length header not provided")
		return
	}

	if r.ContentLength > um.maxChunkSize {
		log.Println(um.logPrefix, r.RemoteAddr, "Request body too large")
		handleRequestError(w, http.StatusBadRequest, fmt.Sprintf("Maximum allowed content length is %d", um.maxChunkSize))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, um.maxChunkSize)

	expectedHeaders := []string{"Offset", "ID"}
	err := um.validateUploadHeaders(&r.Header, expectedHeaders...)
	if err != nil {
		log.Println(um.logPrefix, r.RemoteAddr, err)
		handleRequestError(w, http.StatusBadRequest, err.Error())
		return
	}

	id := r.Header.Get("ID")
	if !um.validateIDExistance(id) {
		log.Println(um.logPrefix, r.RemoteAddr, "ID not found")
		handleRequestError(w, http.StatusForbidden, "ID not found")
		return
	}

	contentLength := r.ContentLength
	offset, err := strconv.ParseInt(r.Header.Get("Offset"), 10, 64)
	if errors.IsError(err) || !um.validateFileOffset(id, offset, contentLength) {
		log.Println(um.logPrefix, r.RemoteAddr, "Invalid file offset", r.Header.Get("Offset"))
		w.Header().Set("Offset", fmt.Sprintf("%d", um.fileBase[id].Offset))
		handleRequestError(w, http.StatusBadRequest, "Invalid offset")
		return
	}

	fileInfo := um.fileBase[id]
	filePath := path.Join(fileInfo.Path, fileInfo.Name)
	file, err := os.OpenFile(filePath, os.O_WRONLY, 0644)
	defer file.Close()
	if errors.IsError(err) {
		log.Println(um.logPrefix, r.RemoteAddr, err)
		handleRequestError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if errors.IsError(err) {
		log.Println(um.logPrefix, r.RemoteAddr, err)
		handleRequestError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	file.WriteAt(body, offset)

	um.fileBaseMutex.Lock()
	defer um.fileBaseMutex.Unlock()
	log.Println(um.logPrefix, r.RemoteAddr, filePath, "Writing at offset", fileInfo.Offset)

	fileInfo.Offset += contentLength
	if fileInfo.Offset == fileInfo.Size {
		log.Println(um.logPrefix, r.RemoteAddr, fmt.Sprintf("File %s was uploaded successfully!", filePath))
		fileInfo.isCompleted = true

		// Name node should be notified here

		w.WriteHeader(http.StatusCreated)
	}
	um.fileBase[id] = fileInfo

}

// validateUploadHeaders is a function to check existance of parameters inside header
func (um *UploadManager) validateUploadHeaders(h *http.Header, params ...string) error {
	for _, param := range params {
		if h.Get(param) == "" {
			return errors.New(fmt.Sprintf("%s header not provided", param))
		}
	}

	return nil
}

// addNewFile is a function to add new file to storage and file base
func (um *UploadManager) addNewFile(id string, filepath string, filename string, filesize int64) error {
	um.fileBaseMutex.Lock()
	defer um.fileBaseMutex.Unlock()

	err := createFile(path.Join(filepath, filename))
	if errors.IsError(err) {
		return err
	}

	um.fileBase[id] = FileInfo{
		Name:        filename,
		Path:        filepath,
		Offset:      0,
		Size:        filesize,
		isCompleted: false,
	}

	return nil
}

func (um *UploadManager) validateIDExistance(id string) bool {
	um.fileBaseMutex.RLock()
	defer um.fileBaseMutex.RUnlock()

	_, present := um.fileBase[id]
	return present
}

func (um *UploadManager) validateFileOffset(id string, offset int64, chunkSize int64) bool {
	um.fileBaseMutex.RLock()
	defer um.fileBaseMutex.RUnlock()

	if offset < 0 {
		return false
	}

	file := um.fileBase[id]
	if file.Offset == offset && !file.isCompleted && file.Offset+chunkSize <= file.Size {
		return true
	}
	return false
}