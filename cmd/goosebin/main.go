package main

import (
	"net/http"
	"os"
	"time"
)
import "github.com/ovrclk/goosebin/internal/server"

func main() {
	router, err := server.GetRouter()
	if err != nil {
		panic(err)
	}

	srv := &http.Server{
		Handler: router,
		Addr: "0.0.0.0:8000",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	srv.ListenAndServe()
	os.Exit(0)
}