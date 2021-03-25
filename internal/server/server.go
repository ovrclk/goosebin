package server

import (
	"embed"
	"encoding/base64"
	"errors"
	"github.com/gorilla/mux"
	"net/http"
	"html/template"
	"fmt"
	"github.com/go-redis/redis"
	"os"
	"strconv"
	"crypto/sha256"
	"time"
)

type pasteKey struct {
	sum []byte
}

func (pk pasteKey) redisKey() string {
	return fmt.Sprintf("paste/%s", base64.RawURLEncoding.EncodeToString(pk.sum))
}

func (pk pasteKey) path() string {
	return fmt.Sprintf("/paste/", base64.RawURLEncoding.EncodeToString(pk.sum))
}

func keyForPaste(value string) pasteKey {
	h := sha256.New()
	fmt.Fprint(h, value)
	sum := h.Sum(nil)

	sum = sum[0:12]
	return pasteKey{sum: sum}
}

func (rh *requestHandler) createPaste(rw http.ResponseWriter, req *http.Request) {
	paste := req.FormValue("paste")
	fmt.Printf("PASTE IS: %s\n", paste)

	k := keyForPaste(paste)
	result := rh.client.SetNX(k.redisKey(), paste, rh.pasteTtl)
	if result.Err() != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		fmt.Printf("%s %s error on redis setnx: %q\n", req.Method, req.URL.Path, result.Err())
		return
	}
	vars := make(map[string]interface{})
	
	if !result.Val() {
		rw.WriteHeader(http.StatusUnprocessableEntity)
		rh.serveTemplate(rw, req, "pasteExists", vars)
		return
	}

	rw.WriteHeader(http.StatusOK)
	vars["pastePath"] = k.path()
	rh.serveTemplate(rw, req, "createPaste", vars)
}


func (rh *requestHandler) showPaste(rw http.ResponseWriter, req *http.Request) {

}

func (rh *requestHandler) newPaste(rw http.ResponseWriter, req *http.Request) {
	rw.WriteHeader(http.StatusOK)
	rh.serveTemplate(rw, req, "newPaste", nil)
}


func (rh *requestHandler) showHome(rw http.ResponseWriter, req *http.Request) {
	vars := make(map[string]interface{})
	vars["title"] = "foobar"
	rh.serveTemplate(rw, req, "home", vars)
}

type requestHandler struct {
	t *template.Template
	client *redis.Client
	pasteTtl time.Duration
}

//go:embed templates/*.tmpl
var htmlTemplates embed.FS

var errPasteTtlTooLow = errors.New("paste TTL cannot be less than 1 second")

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

func (rh *requestHandler) serveTemplate(rw http.ResponseWriter, req *http.Request, name string, vars map[string]interface{}) {
	err := rh.t.ExecuteTemplate(rw, name, vars)
	if err != nil {
		fmt.Printf("%s %s error rendering %q: %q\n", req.Method, req.URL.Path, name, err)
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