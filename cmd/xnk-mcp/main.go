package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// xnk-mcp bridges stdio JSON-RPC 2.0 MCP calls from an owned `agy` session
// to the running xnk-iterate desktop app's local /api/mcp endpoint.
func main() {
	endpoint := flag.String("endpoint", "http://127.0.0.1:9292/api/mcp", "xnk-iterate MCP HTTP endpoint")
	flag.Parse()

	client := &http.Client{Timeout: 15 * time.Second}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		resp, err := client.Post(*endpoint, "application/json", bytes.NewReader(line))
		if err != nil {
			fmt.Fprintf(os.Stderr, "xnk-mcp proxy error: %v\n", err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if len(body) > 0 {
			_, _ = os.Stdout.Write(append(body, '\n'))
		}
	}
}
