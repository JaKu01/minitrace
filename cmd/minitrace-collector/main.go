package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/JaKu01/minitrace/collector"
	"github.com/JaKu01/minitrace/internal/trace"
)

func innerInnerFunc(ctx context.Context) string {
	span, ctx := trace.Start(ctx)
	defer span.End()
	time.Sleep(50 * time.Millisecond)

	return "Hello from innerInnerFunc"
}

func innerFunc(ctx context.Context) string {
	span, ctx := trace.Start(ctx)
	defer span.End()
	time.Sleep(100 * time.Millisecond)

	return "Calling innerInnerFunc: " + innerInnerFunc(ctx)
}

func TraceMe() string {
	span, ctx := trace.Start(context.Background())
	defer span.End()
	time.Sleep(10 * time.Millisecond)

	return "Calling innerFunc " + innerFunc(ctx)
}

func main() {

	mux := http.NewServeMux()
	mux.HandleFunc("GET /example", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(TraceMe()))
	})

	go func() {
		col, err := collector.NewCollector([]string{"http://localhost:8080/api/traces"})
		if err != nil {
			return
		}

		time.Sleep(10 * time.Second)
		res := col.CollectTraces()

		fmt.Printf("%v\n", res)

	}()

	http.ListenAndServe(":8000", mux)
}
