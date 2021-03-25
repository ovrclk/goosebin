package server

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"html/template"
	"net/http"
	"os"
	"strconv"
	"time"
)

type pasteKey struct {
	sum []byte
}

func (pk pasteKey) redisKey() string {
	return fmt.Sprintf("paste/%s", base64.RawURLEncoding.EncodeToString(pk.sum))
}

func (pk pasteKey) path() string {
	return fmt.Sprintf("/paste/%s", base64.RawURLEncoding.EncodeToString(pk.sum))
}

func keyForPaste(value string) pasteKey {
	h := sha256.New()
	fmt.Fprint(h, value)
	sum := h.Sum(nil)

	sum = sum[0:16]
	return pasteKey{sum: sum}
}

func keyFromPath(value string) (pasteKey, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return pasteKey{}, err
	}
	return pasteKey{sum: data}, nil
}

func (rh *requestHandler) createPaste(rw http.ResponseWriter, req *http.Request) {
	rw.Header().Add("Cache-Control", "no-store, max-age=0")
	paste := req.FormValue("paste")

	vars := make(map[string]interface{})

	if len(paste) > rh.pasteSizeLimit {
		vars["title"] = "Paste failed"
		vars["why"] = fmt.Sprintf("Paste is too large, it must be less than %d bytes.", rh.pasteSizeLimit)
		rh.serveTemplate(rw, req, "pasteFailed", vars, http.StatusBadRequest)
		return
	}

	k := keyForPaste(paste)
	val, err := rh.client.SetNX(req.Context(), k.redisKey(), paste, rh.pasteTtl).Result()
	if err != nil {
		fmt.Printf("%s %s error on redis setnx: %v\n", req.Method, req.URL.Path, err)
		return
	}

	if !val {
		vars["title"] = "Paste exists"
		vars["pastePath"] = k.path()
		rh.serveTemplate(rw, req, "pasteExists", vars, http.StatusBadRequest)
		return
	}

	vars["title"] = "Paste created"
	rw.WriteHeader(http.StatusOK)
	vars["pastePath"] = k.path()
	rh.serveTemplate(rw, req, "createPaste", vars, http.StatusOK)
}

func (rh *requestHandler) showPaste(rw http.ResponseWriter, req *http.Request) {
	vars := make(map[string]interface{})
	requestVars := mux.Vars(req)
	k, err := keyFromPath(requestVars["key"])
	if err != nil {
		fmt.Printf("%s %s invalid base64 key: %v\n", req.Method, req.URL.Path, err)
		rw.WriteHeader(http.StatusNotFound)
		return
	}
	vars["title"] = "Paste"

	val, err := rh.client.Get(req.Context(), k.redisKey()).Result()
	if err == redis.Nil {
		rw.Header().Add("Cache-Control", "no-store, max-age=0")
		fmt.Printf("%s %s key does not exist\n", req.Method, req.URL.Path)
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		fmt.Printf("%s %s error getting key from redis: %v\n", req.Method, req.URL.Path, err)
		return
	}


	vars["paste"] = val

	rw.Header().Add("Cache-Control", "max-age=86400")
	rh.serveTemplate(rw, req, "showPaste", vars, http.StatusOK)
}

func (rh *requestHandler) newPaste(rw http.ResponseWriter, req *http.Request) {
	vars := make(map[string]interface{})
	vars["title"] = "Create Paste"

	rw.WriteHeader(http.StatusOK)
	rw.Header().Add("Cache-Control", "max-age=86400")
	rh.serveTemplate(rw, req, "newPaste", vars, http.StatusOK)
}


func (rh *requestHandler) showHome(rw http.ResponseWriter, req *http.Request) {
	vars := make(map[string]interface{})
	vars["title"] = "Goosebin"
	rw.Header().Add("Cache-Control", "max-age=86400")
	rh.serveTemplate(rw, req, "home", vars, http.StatusOK)
}

type requestHandler struct {
	t *template.Template
	client *redis.Client
	pasteTtl time.Duration
	pasteSizeLimit int
}

//go:embed templates/*.tmpl
var htmlTemplates embed.FS

var errPasteTtlTooLow = errors.New("paste TTL cannot be less than 1 second")
var errPasteSizeLimitTooLow = errors.New("paste size limit cannot be less than 128 bytes")

func newRequestHandler() (*requestHandler, error) {
	result :=  &requestHandler{
	}

	pasteTtlStr := os.Getenv("PASTE_TTL_SECONDS")
	if len(pasteTtlStr) == 0{
		result.pasteTtl = time.Hour * 24 * 7
	} else {
		ttlSeconds, err := strconv.Atoi(pasteTtlStr)
		if err != nil {
			return nil, err
		}

		if ttlSeconds < 1 {
			return nil, errPasteTtlTooLow
		}

		result.pasteTtl = time.Second * time.Duration(ttlSeconds)
	}

	pasteSizeLimitStr := os.Getenv("PASTE_SIZE_LIMIT")
	if len(pasteTtlStr) == 0 {
		result.pasteSizeLimit = 65535
	} else {
		var err error
		result.pasteSizeLimit, err = strconv.Atoi(pasteSizeLimitStr)
		if err != nil {
			return nil, err
		}

		if result.pasteSizeLimit < 128 {
			return nil, errPasteSizeLimitTooLow
		}
	}

	var err error
	redisHost := os.Getenv("REDIS_HOST")

	if len(redisHost) == 0 {
		redisHost = "localhost"
	}

	redisPort := os.Getenv("REDIS_PORT")
	if len(redisPort) == 0 {
		redisPort = "6379"
	}

	var redisPoolSize int
	redisPoolSizeStr := os.Getenv("REDIS_POOL_SIZE")
	if len(redisPoolSizeStr) == 0 {
	  redisPoolSize = 32
	} else {
		redisPoolSize, err = strconv.Atoi(redisPoolSizeStr)
		if err != nil {
			return nil, err
		}
	}

	redisOptions := &redis.Options{
		Network:            "tcp",
		Addr:               fmt.Sprintf("%s:%s", redisHost, redisPort),
		MaxRetries:         -1,
		MinIdleConns:       1,
		PoolSize: redisPoolSize,
	}

	result.client = redis.NewClient(redisOptions)
	result.t, err = template.New("foo").ParseFS(htmlTemplates,"templates/*.tmpl")
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (rh *requestHandler) serveTemplate(rw http.ResponseWriter, req *http.Request, name string, vars map[string]interface{}, status int) {
	rw.Header().Add("Content-Type", "text/html; charset=UTF-8")
	rw.WriteHeader(status)
	err := rh.t.ExecuteTemplate(rw, name, vars)
	if err != nil {
		fmt.Printf("%s %s error rendering %q: %v\n", req.Method, req.URL.Path, name, err)
	} else {
		fmt.Printf("%s %s rendered %q\n", req.Method, req.URL.Path, name)
	}
}

func GetRouter() (*mux.Router, error){
	r := mux.NewRouter()
	rh, err := newRequestHandler()
	if err != nil {
		return nil, err
	}
	r.HandleFunc("/", rh.showHome).Methods("GET")
	r.HandleFunc("/create-paste", rh.createPaste).Methods("POST")
	r.HandleFunc("/create-paste", rh.newPaste).Methods("GET")
	r.HandleFunc("/paste/{key}", rh.showPaste).Methods("GET")

	return r, nil
}