package gateway

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGatewayFoundationImportBoundaries(t *testing.T) {
	forbiddenImports := []string{
		"github.com/Wei-Shaw/sub2api/internal/domain",
		"github.com/Wei-Shaw/sub2api/internal/service",
		"github.com/Wei-Shaw/sub2api/internal/repository",
		"github.com/Wei-Shaw/sub2api/internal/handler",
		"github.com/Wei-Shaw/sub2api/internal/server",
		"github.com/gin-gonic/gin",
	}

	for _, packageDir := range []string{"domain", "core"} {
		err := filepath.WalkDir(packageDir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}

			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse imports for %s: %v", path, err)
			}

			for _, importSpec := range file.Imports {
				importPath := strings.Trim(importSpec.Path.Value, `"`)
				for _, forbiddenImport := range forbiddenImports {
					if importPath == forbiddenImport || strings.HasPrefix(importPath, forbiddenImport+"/") {
						t.Errorf("%s imports forbidden package %q", path, importPath)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk package dir %s: %v", packageDir, err)
		}
	}
}
