package cfg_g

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// sharedNameReaders are the files outside the one writer that may name a
// shared file, and why. A new entry is a new reader or writer of shared
// state: if it writes, it must go through SaveConfigFile instead.
var sharedNameReaders = map[string]string{
	"cfg_g/helpers.go":     "the loader and SaveConfigFile",
	"cfg_g/config.go":      "the loader",
	"cfg_g/links.go":       "ReadLinks reads config.json for Tab",
	"cfg_g/accountname.go": "the legacy per-account list names, moved by MigrateDirSanitization",
	"cfg_g/sharedwrite.go": "SharedFiles",
	"cfg_g/canonical.go":   "which shared files are one set",
	"cfg_g/save.go":        "SaveConfigFile",
	"cfg_g/merge3.go":      "the accounts of config.json merge by name",
	"cli/cli.go":           "help text",
	"cli/helpers.go":       "completion reads config.json",
	"cli/migrate.go":       "account migrate copies config.json to another host",
}

// writePrimitives are the calls that put bytes in a file.
var writePrimitives = []string{"os.WriteFile", "os.Create", "os.OpenFile", "dirlock.AtomicWrite", "atomicWriteFile"}

// configPathHints in a write's path argument mean it writes into the
// configuration: a path made from the loader's own names for it.
var configPathHints = []string{"configPath", "ConfigPath", "resolved.Path", "AccountDir("}

type goFile struct {
	rel  string
	file *ast.File
	fset *token.FileSet
}

// repoGoFiles parses every non-test Go file of the module.
func repoGoFiles(t *testing.T) []goFile {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	var files []goFile
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata" || d.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		rel, _ := filepath.Rel(root, path)
		files = append(files, goFile{rel: filepath.ToSlash(rel), file: f, fset: fset})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 100 {
		t.Fatalf("found only %d Go files under %s: the walk is not seeing the repository", len(files), root)
	}
	return files
}

func exprText(fset *token.FileSet, src string, e ast.Expr) string {
	start, end := fset.Position(e.Pos()).Offset, fset.Position(e.End()).Offset
	return src[start:end]
}

func callName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		if pkg, ok := fn.X.(*ast.Ident); ok {
			return pkg.Name + "." + fn.Sel.Name
		}
	}
	return ""
}

// namesSharedFile reports whether a string literal is a shared file's name,
// alone or as the tail of a legacy per-account name ("%s_rules.json").
func namesSharedFile(lit string) bool {
	for _, name := range SharedFiles {
		if lit == name || strings.HasSuffix(lit, "_"+name) {
			return true
		}
	}
	return false
}

// sharedWriteProblems is every place in gf, whose source is src, that names
// a shared file without being a known reader, or writes one directly.
func sharedWriteProblems(gf goFile, src string) []string {
	var problems []string
	var enclosing string
	ast.Inspect(gf.file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			enclosing = n.Name.Name
		case *ast.BasicLit:
			problems = append(problems, sharedNameProblems(gf, n)...)
		case *ast.CallExpr:
			problems = append(problems, writeProblems(gf, src, enclosing, n)...)
		}
		return true
	})
	return problems
}

func sharedNameProblems(gf goFile, lit *ast.BasicLit) []string {
	if lit.Kind != token.STRING {
		return nil
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil || !namesSharedFile(s) {
		return nil
	}
	if _, ok := sharedNameReaders[gf.rel]; ok {
		return nil
	}
	return []string{fmt.Sprintf("%s: names the shared file %q; write it through cfg_g.SaveConfigFile, and if this only reads it, add the file to sharedNameReaders",
		gf.fset.Position(lit.Pos()), s)}
}

func writeProblems(gf goFile, src, enclosing string, call *ast.CallExpr) []string {
	name := callName(call)
	if !slices.Contains(writePrimitives, name) || len(call.Args) == 0 {
		return nil
	}
	pos := gf.fset.Position(call.Pos())
	if strings.HasPrefix(gf.rel, "cfg_g/") {
		if allowedCfgWrite(name, enclosing) {
			return nil
		}
		return []string{fmt.Sprintf("%s: %s in %s writes a file directly; a shared file is written by writeShared", pos, name, enclosing)}
	}
	arg := exprText(gf.fset, src, call.Args[0])
	for _, hint := range configPathHints {
		if strings.Contains(arg, hint) {
			return []string{fmt.Sprintf("%s: %s(%s) writes into the configuration directly; use cfg_g.SaveConfigFile", pos, name, arg)}
		}
	}
	return nil
}

// allowedCfgWrite is the one writer, and the files cfg_g writes that are not
// JSON shared state: the account rename's marker, and an account's
// signature.txt (WriteSignature), which is plain text with its own --dry-run
// guard. Whether a signature travels with a sync export is sync step 3's call.
func allowedCfgWrite(name, enclosing string) bool {
	return name == "atomicWriteFile" && (enclosing == "writeShared" || enclosing == "BeginRename" || enclosing == "WriteSignature")
}

// Every write of config.json and the per-account files goes through
// writeShared: nothing else in cfg_g writes a file directly, no file outside
// a known few names a shared file, and no write anywhere takes its path from
// the configuration's own names for it.
func TestSharedStateHasOneWriter(t *testing.T) {
	for _, gf := range repoGoFiles(t) {
		data, err := os.ReadFile(filepath.Join("..", gf.rel))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range sharedWriteProblems(gf, string(data)) {
			t.Error(p)
		}
	}
}

// The check sees what it is meant to see: a direct write into the
// configuration, a stray shared file name, and a direct write inside cfg_g
// are each flagged, and the one writer is not.
func TestSharedStateCheckFlagsADirectWrite(t *testing.T) {
	src := `package x
func f(configPath string) { _ = os.WriteFile(configPath, nil, 0o600) }
func g() { _ = "rules.json" }
func writeShared() { _ = atomicWriteFile(p, nil, 0) }
func other() { _ = atomicWriteFile(p, nil, 0) }`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]int{
		"cli/x.go":   2, // the write into configPath, and the name
		"cfg_g/x.go": 3, // the name, os.WriteFile, and atomicWriteFile outside writeShared
	}
	for rel, want := range cases {
		got := sharedWriteProblems(goFile{rel: rel, file: f, fset: fset}, src)
		if len(got) != want {
			t.Errorf("%s: %d problems, want %d:\n%s", rel, len(got), want, strings.Join(got, "\n"))
		}
	}
}
