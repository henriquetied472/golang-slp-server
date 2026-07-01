package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
)

func RunMonitor(ctx context.Context, udp *SLPServer, port int) {
	handler := http.NewServeMux()
	server := &http.Server{
		Addr: ":"+fmt.Sprint(port),
		Handler: handler,
	}

	handler.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(200)
			size := udp.GetClientSize()
			w.Write([]byte(fmt.Sprintf("{ \"online\": %d, \"version\": \"1.0.0-ALPHA\" }", size)))
		}
	})

	go func() {
		go func(){
			<-ctx.Done()
			server.Close()
		}()

		slog.Info("Monitor: monitor listening")
		if err := server.ListenAndServe(); err == http.ErrServerClosed {
			slog.Info("Monitor: monitor closed")
		}
	}()
}
