package router

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestPrintRequiredProductionRoutes(t *testing.T) {
	if os.Getenv("PRINT_REQUIRED_ROUTES") != "1" {
		t.Skip("set PRINT_REQUIRED_ROUTES=1 to print the required route inventory")
	}

	handler, err := NewRouterWithDependencies(testRouterConfig(), testRouterDependencies())
	if err != nil {
		t.Fatal(err)
	}
	routes, ok := handler.(chi.Routes)
	if !ok {
		t.Fatalf("router %T does not expose Chi routes", handler)
	}

	var mounted []string
	if err := chi.Walk(routes, func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted = append(mounted, method+" "+path)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(mounted)
	for _, route := range mounted {
		fmt.Println(route)
	}
}
