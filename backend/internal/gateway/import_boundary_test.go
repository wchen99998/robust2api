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
	strictPackageDirs := []string{
		"domain",
		"core",
		"provider",
		"planning",
		"scheduler",
	}
	forbiddenImports := []string{
		"github.com/Wei-Shaw/sub2api/ent",
		"github.com/Wei-Shaw/sub2api/internal/config",
		"github.com/Wei-Shaw/sub2api/internal/domain",
		"github.com/Wei-Shaw/sub2api/internal/service",
		"github.com/Wei-Shaw/sub2api/internal/repository",
		"github.com/Wei-Shaw/sub2api/internal/handler",
		"github.com/Wei-Shaw/sub2api/internal/pkg",
		"github.com/Wei-Shaw/sub2api/internal/server",
		"github.com/gin-gonic/gin",
	}

	for _, packageDir := range strictPackageDirs {
		packageDir := packageDir
		t.Run(packageDir, func(t *testing.T) {
			if !directoryExists(t, packageDir) {
				t.Skipf("package directory %s does not exist yet", packageDir)
			}
			assertNoForbiddenImports(t, packageDir, forbiddenImports)
		})
	}
}

func TestGatewayEdgePackageSmoke(t *testing.T) {
	for _, packageDir := range []string{"ingress", "adapters"} {
		packageDir := packageDir
		t.Run(packageDir, func(t *testing.T) {
			if !directoryExists(t, packageDir) {
				t.Skipf("edge package directory %s does not exist yet", packageDir)
			}
			assertGoFilesParse(t, packageDir)
		})
	}
}

func assertNoForbiddenImports(t *testing.T, packageDir string, forbiddenImports []string) {
	t.Helper()

	forEachGoSourceFile(t, packageDir, func(path string) {
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
	})
}

func assertGoFilesParse(t *testing.T, packageDir string) {
	t.Helper()

	forEachGoSourceFile(t, packageDir, func(path string) {
		if _, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly); err != nil {
			t.Fatalf("parse imports for %s: %v", path, err)
		}
	})
}

func forEachGoSourceFile(t *testing.T, packageDir string, visit func(path string)) {
	t.Helper()

	err := filepath.WalkDir(packageDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		visit(path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk package dir %s: %v", packageDir, err)
	}
}

func directoryExists(t *testing.T, path string) bool {
	t.Helper()

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		t.Fatalf("stat package dir %s: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("package path %s is not a directory", path)
	}
	return true
}
