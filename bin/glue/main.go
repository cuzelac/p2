package main

import (
	//"github.com/square/p2/pkg/hooks"
	//"io/ioutil"
	"bytes"
	"io"
	"os"
	"os/exec"
)

func main() {
	//hooks.RunHook('
	out := &bytes.Buffer{}

	cmd := exec.Command("./hupper.sh")
	//cmd.Stdin = nil
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.Run()

	io.Copy(os.Stdout, out)
}
