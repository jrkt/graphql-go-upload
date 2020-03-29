package upload

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type postedFiles func(key string) (multipart.File, *multipart.FileHeader, error)

type graphqlParams struct {
	Variables  interface{}            `json:"variables"`
	Query      interface{}            `json:"query"`
	Operations map[string]interface{} `json:"operations"`
	Map        map[string][]string    `json:"map"`
}

type fileData struct {
	Fields        interface{}
	Files         postedFiles
	MapEntryIndex string
	EntryPaths    []string
}

var (
	l sync.Mutex
)

func Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMiddlewareSupported(r) {
			next.ServeHTTP(w, r)
			return
		}

		l.Lock()
		defer l.Unlock()

		err := validateRequest(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isMiddlewareSupported(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}

	contentType := r.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if contentType == "" || mediaType != "multipart/form-data" {
		return false
	}

	return true
}

func validateRequest(w http.ResponseWriter, r *http.Request) error {
	r.ParseMultipartForm((1 << 20) * 64)

	m := r.PostFormValue("map")
	if &m == nil {
		return errors.New("missing map field parameter")
	}

	o := r.PostFormValue("operations")
	if &o == nil {
		return errors.New("missing operations field parameter")
	}

	operations := make(map[string]interface{})

	err := json.Unmarshal([]byte(o), &operations)
	if err != nil {
		return errors.New("cannot unmarshal operations: malformed query")
	}

	mapEntries := make(map[string][]string)

	err = json.Unmarshal([]byte(m), &mapEntries)
	if err != nil {
		return errors.New("cannot unmarshal map entries: malformed query")
	}

	for idx, mapEntry := range mapEntries {
		for _, entry := range mapEntry {
			entryPaths := strings.Split(entry, ".")
			fields := findFields(operations, entryPaths[:len(entryPaths)-1])

			// assign normal variable types
			if value := r.PostForm.Get(idx); value != "" {
				entryPaths := strings.Split(entry, ".")
				operations[entryPaths[0]].(map[string]interface{})[entryPaths[1]] = value
			} else {
				// assign Upload variable types
				params := fileData{
					Fields:        fields,
					Files:         r.FormFile,
					MapEntryIndex: idx,
					EntryPaths:    entryPaths,
				}

				file, handle, err := params.Files(params.MapEntryIndex)
				if err != nil {
					return fmt.Errorf("could not access multipart file: %s", err)
				}
				defer file.Close()

				file.Seek(0, 0)
				data, err := ioutil.ReadAll(file)
				if err != nil {
					return fmt.Errorf("could not read multipart file: %s", err)
				}

				extension := strings.ToLower(filepath.Ext(handle.Filename))
				filename := fmt.Sprintf("graphqlupload-*%s", extension)
				f, err := ioutil.TempFile(os.TempDir(), filename)
				if err != nil {
					return fmt.Errorf("unable to create temporary file: %s", err)
				}

				_, err = f.Write(data)
				if err != nil {
					return fmt.Errorf("could not write temporary file: %s", err)
				}

				upload := &Upload{
					MimeType: http.DetectContentType(data),
					Filename: handle.Filename,
					Filepath: f.Name(),
				}

				if op, ok := params.Fields.(map[string]interface{}); ok {
					i := len(params.EntryPaths) - 1
					arrKey := params.EntryPaths[i]
					last := params.EntryPaths[i-1]

					if arr, isArr := op[last].([]interface{}); isArr {
						key, err := strconv.Atoi(arrKey)
						if err != nil {
							return fmt.Errorf("error converting final upload key to int: %s", err)
						}
						arr[key] = upload
						continue
					}

					op[arrKey] = upload
				}
			}
		}
	}

	graphqlParams := graphqlParams{
		Variables:  operations["variables"],
		Query:      operations["query"],
		Operations: operations,
		Map:        mapEntries,
	}

	body, err := json.Marshal(graphqlParams)
	if err == nil {
		r.Body = ioutil.NopCloser(bytes.NewReader(body))
		w.Header().Set("Content-Type", "application/json")
	}

	return nil
}

func findFields(operations interface{}, entryPaths []string) map[string]interface{} {
	for i := 0; i < len(entryPaths); i++ {
		if arr, ok := operations.([]map[string]interface{}); ok {
			operations = arr[i]

			return findFields(operations, entryPaths)
		} else if op, ok := operations.(map[string]interface{}); ok && entryPaths[i] != "attachments" {
			operations = op[entryPaths[i]]
		}
	}

	return operations.(map[string]interface{})
}
