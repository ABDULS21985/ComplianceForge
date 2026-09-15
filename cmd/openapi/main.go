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
	if _, err := temporary.Write(data); err != nil {
		return errors.Join(fmt.Errorf("write temporary OpenAPI file: %w", err), closeAndRemove(temporary, temporaryName))
	}
	if err := temporary.Chmod(0o644); err != nil {
		return errors.Join(fmt.Errorf("set generated OpenAPI permissions: %w", err), closeAndRemove(temporary, temporaryName))
	}
	if err := temporary.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync temporary OpenAPI file: %w", err), closeAndRemove(temporary, temporaryName))
	}
	if err := temporary.Close(); err != nil {
		return errors.Join(fmt.Errorf("close temporary OpenAPI file: %w", err), removeTemporary(temporaryName))
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		return errors.Join(fmt.Errorf("replace generated OpenAPI file: %w", err), removeTemporary(temporaryName))
	}
	return nil
}

func closeAndRemove(file *os.File, filename string) error {
	return errors.Join(wrapFileError("close temporary OpenAPI file", file.Close()), removeTemporary(filename))
}

func removeTemporary(filename string) error {
	err := os.Remove(filename)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("remove temporary OpenAPI file: %w", err)
}

func wrapFileError(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "openapi:", err)
	os.Exit(1)
}
