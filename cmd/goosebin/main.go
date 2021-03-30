package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)
import "github.com/ovrclk/goosebin/internal/server"

func main() {
	router, err := server.GetRouter()
	if err != nil {
		fmt.Printf("Could not create server:%v\n", err)
		os.Exit(1)
	}

	httpPortStr := os.Getenv("HTTP_PORT")
	if len(httpPortStr) == 0 {
		httpPortStr = "8000"
	}
	httpPort, err := strconv.Atoi(httpPortStr)
	if err != nil {
		fmt.Printf("Could not parse http port string %q:%v\n", httpPortStr, err)
		os.Exit(1)
	}
	addr := fmt.Sprintf("0.0.0.0:%d", httpPort)
	srv := &http.Server{
		Handler: router,
		Addr: addr,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	fmt.Printf("Starting HTTP server on %q\n", addr)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP)
		result := <-sig
		fmt.Printf("Got %s, stopping server\n", result)

		ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(time.Second * 30))

		// We received an interrupt signal, shut down.
		if err := srv.Shutdown(ctx); err != nil {
			// Error from closing listeners, or context timeout:
			log.Printf("HTTP server Shutdown: %v", err)
		}

	}()

	err = srv.ListenAndServe()
	wg.Wait()

	if err != nil && err != http.ErrServerClosed {
		fmt.Printf("HTTP server failed: %v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}