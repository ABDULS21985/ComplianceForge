package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	contract "github.com/complianceforge/platform/internal/openapi"
)

func main() {
	base := flag.String("base", "api/openapi/base.json", "base OpenAPI document")
	routes := flag.String("routes", "api/openapi/routes.csv", "required production route catalog")
	output := flag.String("output", "api/openapi/openapi.json", "generated OpenAPI document")
	flag.Parse()

	command := "check"
	if flag.NArg() > 0 {
		command = flag.Arg(0)
	}
	if flag.NArg() > 1 {
		fatal(errors.New("usage: openapi [flags] [generate|validate|check]"))
	}

	switch command {
	case "generate":
		generated, err := contract.Generate(*base, *routes)
		if err != nil {
			fatal(err)
		}
		if err := writeAtomic(*output, generated); err != nil {
			fatal(err)
		}
		validated, err := contract.Load(context.Background(), *output)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("generated %s with %d operations\n", *output, len(validated.Operations))
	case "validate":
		validated, err := contract.Load(context.Background(), *output)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("validated OpenAPI 3.1 contract with %d operations\n", len(validated.Operations))
	case "check":
		generated, err := contract.Generate(*base, *routes)
		if err != nil {
			fatal(err)
		}
		committed, err := os.ReadFile(*output)
		if err != nil {
			fatal(fmt.Errorf("read generated OpenAPI document: %w", err))
		}
		if !bytes.Equal(generated, committed) {
			fatal(errors.New("api/openapi/openapi.json is stale; run `go run ./cmd/openapi generate`"))
		}
		validated, err := contract.Load(context.Background(), *output)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("OpenAPI contract is current and valid with %d operations\n", len(validated.Operations))
	default:
		fatal(fmt.Errorf("unknown command %q", command))
	}
}

func writeAtomic(filename string, data []byte) error {
	directory := filepath.Dir(filename)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return fmt.Errorf("create OpenAPI directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".openapi-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary OpenAPI file: %w", err)
	}
	temporaryName := temporary.Name()
	cleanup := func() { _ = os.Remove(temporaryName) }
	defer cleanup()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary OpenAPI file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary OpenAPI file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary OpenAPI file: %w", err)
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		return fmt.Errorf("replace generated OpenAPI file: %w", err)
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "openapi:", err)
	os.Exit(1)
}
