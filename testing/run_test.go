package testing

import (
	"os"
	"testing"
)

func TestCompileAndRunAll(t *testing.T) {
	var readDir func(dirname string) []string
	readDir = func(dirname string) []string {
		files, err := os.ReadDir(dirname)
		if err != nil {
			t.Fatal(err)
		}
		var res []string
		for _, file := range files {
			if file.IsDir() {
				subFiles := readDir(file.Name())
				if err != nil {
					t.Fatal(err)
				}
				for _, f := range subFiles {
					res = append(res, dirname+"/"+f)
				}
			} else {
				res = append(res, dirname+"/"+file.Name())
			}
		}
		return res
	}

	files := readDir("testset")
	for _, filename := range files {
		t.Run(filename, func(t *testing.T) {
			RunTest(t, false, filename)
		})
	}
}

func TestCompileAndRunOne(t *testing.T) {
	filename := "./testset/bound_decl_error.txt"
	RunTest(t, true, filename)
}
