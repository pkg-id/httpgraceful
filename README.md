# httpgraceful

A wrapper for `http.Server` that enables graceful shutdown handling.

## Installation

```shell
go get github.com/pkg-id/httpgraceful
```

## Example

```go
package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/pkg-id/httpgraceful"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "Hello, World")
	})

	server := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", 8080),
		Handler: mux,
	}

	gs := httpgraceful.WrapServer(server)
	if err := gs.ListenAndServe(); err != nil {
		panic(err)
	}
}
```

When you wrap your server with `httpgraceful.WrapServer`, the `ListenAndServe` method will keep the server running until it receives either the `os.Interrupt` signal or the `syscall.SIGTERM` signal. 
At that point, the graceful shutdown process will be initiated, waiting for all active connections to be closed. If there are still active connections after the wait timeout is exceeded, the server will be force closed. 
The default wait timeout is 5 seconds. 

However, you can customize the timeout and the signal as shown in the example below:

```go
gs := httpgraceful.WrapServer(
	server, 
	httpgraceful.WithWaitTimeout(100*time.Millisecond), 
	httpgraceful.WithSignal(syscall.SIGTERM)
)
```
