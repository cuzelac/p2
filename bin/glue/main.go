package main

import (
	"github.com/square/p2/pkg/hooks"
	"github.com/square/p2/pkg/logging"

	"io/ioutil"
	"path/filepath"

	"os"
)

func main() {
	tmpfile := writeBash("nohup sleep 100 &") // HANG
	//tmpfile := writeBash("nohup sleep 100 >/dev/null 2>/dev/null &") // NO HANG
	defer os.Remove(tmpfile.Name())

	fullPath, _ := filepath.Abs(tmpfile.Name())

	hooks.RunHook(fullPath, "nohup-hook", []string{"a", "b"}, logging.TestLogger())
}

func writeBash(line string) *os.File {
	tmpfile, _ := ioutil.TempFile(".", "runner.")

	tmpfile.Write([]byte("#!/bin/bash\n"))
	tmpfile.Write([]byte(line))
	tmpfile.Write([]byte("\n"))
	tmpfile.Close()

	os.Chmod(tmpfile.Name(), 0744)

	return tmpfile
}
