package main

import (
	"context"
	"net/http"
	"time"

	"github.com/JaKu01/minitrace"
)

func innerInnerFunc(ctx context.Context) string {
	span, ctx := minitrace.Start(ctx)
	defer span.End()
	time.Sleep(50 * time.Millisecond)

	return "Hello from innerInnerFunc"
}

func innerFunc(ctx context.Context) string {
	span, ctx := minitrace.Start(ctx)
	defer span.End()
	time.Sleep(100 * time.Millisecond)

	return "Calling innerInnerFunc: " + innerInnerFunc(ctx)
}

func TraceMe() string {
	span, ctx := minitrace.Start(context.Background())
	defer span.End()
	time.Sleep(10 * time.Millisecond)

	return "Calling innerFunc " + innerFunc(ctx)
}

func main() {

	mux := http.NewServeMux()
	mux.HandleFunc("GET /example", func(w http.ResponseWriter, r *http.Request) {

		w.Write([]byte(TraceMe()))
	})

	http.ListenAndServe(":8000", mux)
}
