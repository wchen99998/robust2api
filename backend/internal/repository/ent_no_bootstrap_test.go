//go:build unit

package repository

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestInitEnt_DoesNotCallBootstrapFunctions verifies that InitEnt no longer references
// migration functions, ensuring bootstrap separation.
func TestInitEnt_DoesNotCallBootstrapFunctions(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "ent.go", nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse ent.go: %v", err)
	}

	banned := []string{
		"applyMigrationsFS",
		"ApplyMigrations",
		"ensureBootstrapSecrets",
		"EnsureSimpleModeDefaultGroups",
		"EnsureSimpleModeAdminConcurrency",
		"ensureSimpleModeDefaultGroups",
		"ensureSimpleModeAdminConcurrency",
	}

	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			var name string
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				name = fn.Name
			case *ast.SelectorExpr:
				name = fn.Sel.Name
			}
			for _, b := range banned {
				if name == b {
					t.Errorf("InitEnt must not call %s — bootstrap mutation belongs in the bootstrap binary", b)
				}
			}
		}
		return true
	})
}

// TestInitEnt_DoesNotImportMigrations verifies the migrations package is no longer imported.
func TestInitEnt_DoesNotImportMigrations(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "ent.go", nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse ent.go: %v", err)
	}

	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if strings.HasSuffix(path, "/migrations") {
			t.Errorf("ent.go must not import the migrations package — bootstrap owns migration execution")
		}
	}
}
