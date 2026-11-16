// Command kairosflux-import-registry imports QuantBrew experiment records as versioned data.
// It does not schedule or execute research jobs.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ChronoBrew/KairosFlux/internal/jobctl"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	fs := flag.NewFlagSet("kairosflux-import-registry", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:9000", "KairosFlux v2 server address")
	timeout := fs.Duration("timeout", 5*time.Second, "request timeout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: kairosflux-import-registry [-addr host:port] [-timeout duration] <registry.jsonl>")
		return 2
	}
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open registry: %v\n", err)
		return 1
	}
	defer f.Close()
	store := jobctl.NewV2Store(*addr, *timeout)
	defer store.Close()
	result, err := (&jobctl.RegistryImporter{Store: store}).ImportReader(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "import failed: %v\n", err)
		return 1
	}
	fmt.Printf("lines=%d imported=%d unchanged=%d errors=%d\n", result.TotalLines, result.Imported, result.Unchanged, len(result.Errors))
	for _, e := range result.Errors {
		fmt.Fprintf(os.Stderr, "line %d: %s\n", e.Line, e.Reason)
	}
	if len(result.Errors) > 0 {
		return 1
	}
	return 0
}
