package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"runtime"
	"time"

	"github.com/square/p2/pkg/hooks"
	"github.com/square/p2/pkg/intent"
	"github.com/square/p2/pkg/kp"
	"github.com/square/p2/pkg/pods"
	"github.com/square/p2/pkg/util"
)

func main() {
	// 1. Generate pod for preparer in this code version (`rake artifact:prepare`)
	// 2. Locate manifests for preparer pod, premade consul pod
	// 3. Execute bootstrap with premade consul pod and preparer pod
	// 4. Deploy hello pod manifest by pushing to intent store
	// 5. Verify that hello is running (listen to syslog? verify Runit PIDs? Both?)
	tempdir, err := ioutil.TempDir("", "single-node-check")
	log.Printf("Putting test manifests in %s\n", tempdir)
	if err != nil {
		log.Fatalln("Could not create temp directory, bailing")
	}
	preparerManifest, err := generatePreparerPod(tempdir)
	if err != nil {
		log.Fatalf("Could not generate preparer pod: %s\n", err)
	}
	consulManifest, err := getConsulManifest(tempdir)
	if err != nil {
		log.Fatalf("Could not generate consul pod: %s\n", err)
	}
	fmt.Println("Executing bootstrap")
	err = executeBootstrap(preparerManifest, consulManifest)
	if err != nil {
		log.Fatalf("Could not execute bootstrap: %s", err)
	}
	// _, err = user.CreateUser("hello", pods.PodPath("hello"))
	// if err != nil && err != user.AlreadyExists {
	// 	log.Fatalf("Could not create user: %s", err)
	// }
	err = scheduleUserCreationHook(tempdir)
	if err != nil {
		log.Fatalf("Couldn't schedule the user creation hook: %s", err)
	}
	err = postHelloManifest(tempdir)
	if err != nil {
		log.Fatalf("Could not generate hello pod: %s\n", err)
	}
	err = verifyHelloRunning()
	if err != nil {
		log.Fatalf("Couldn't get hello running: %s", err)
	}
}

func generatePreparerPod(workdir string) (string, error) {
	// build the artifact from HEAD
	err := exec.Command("go", "build", "github.com/square/p2/bin/p2-preparer").Run()
	if err != nil {
		return "", util.Errorf("Couldn't build preparer: %s", err)
	}
	wd, _ := os.Getwd()
	hostname, err := os.Hostname()
	if err != nil {
		return "", util.Errorf("Couldn't get hostname: %s", err)
	}
	// the test number forces the pod manifest to change every test run.
	testNumber := fmt.Sprintf("test=%d", rand.Intn(2000000000))
	cmd := exec.Command("p2-bin2pod", "--work-dir", workdir, "--id", "p2-preparer", "--config", fmt.Sprintf("node_name=%s", hostname), "--config", testNumber, wd+"/p2-preparer")
	return executeBin2Pod(cmd)
}

func scheduleUserCreationHook(tmpdir string) error {
	createUserPath := path.Join(tmpdir, "create_user")
	script := `#!/usr/bin/env bash
set -e
useradd $POD_ID -d $POD_HOME
`
	err := ioutil.WriteFile(createUserPath, []byte(script), 0744)
	if err != nil {
		return err
	}

	cmd := exec.Command("p2-bin2pod", "--work-dir", tmpdir, createUserPath)
	manifestPath, err := executeBin2Pod(cmd)
	if err != nil {
		return err
	}
	manifest, err := pods.PodManifestFromPath(manifestPath)
	if err != nil {
		return err
	}
	store := kp.NewStore(kp.Options{})
	_, err = store.SetPod(kp.HookPath(hooks.BEFORE_INSTALL, "create_user"), *manifest)
	if err != nil {
		return err
	}
	return nil
}

func executeBin2Pod(cmd *exec.Cmd) (string, error) {
	out := bytes.Buffer{}
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return "", util.Errorf("p2-bin2pod failed: %s", err)
	}
	var bin2podres map[string]string
	err = json.Unmarshal(out.Bytes(), &bin2podres)
	if err != nil {
		return "", err
	}
	return bin2podres["manifest_path"], nil
}

func getConsulManifest(dir string) (string, error) {
	consulTar := fmt.Sprintf("file://%s", util.From(runtime.Caller(0)).ExpandPath("../hoisted-consul_abc123.tar.gz"))
	manifest := &pods.PodManifest{}
	manifest.Id = "intent"
	stanza := pods.LaunchableStanza{
		LaunchableId:   "consul",
		LaunchableType: "hoist",
		Location:       consulTar,
	}
	manifest.LaunchableStanzas = map[string]pods.LaunchableStanza{
		"consul": stanza,
	}
	consulPath := path.Join(dir, "consul.yaml")
	f, err := os.OpenFile(consulPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return "", err
	}
	err = manifest.Write(f)
	if err != nil {
		return "", err
	}
	return consulPath, f.Close()
}

func executeBootstrap(preparerManifest, consulManifest string) error {
	cmd := exec.Command("rake", "install")
	cmd.Stderr = os.Stdout
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("Could not install newest bootstrap: %s", err)
	}
	bootstr := exec.Command("p2-bootstrap", "--consul-pod", consulManifest, "--agent-pod", preparerManifest)
	bootstr.Stdout = os.Stdout
	bootstr.Stderr = os.Stdout
	return bootstr.Run()
}

func postHelloManifest(dir string) error {
	hello := fmt.Sprintf("file://%s", util.From(runtime.Caller(0)).ExpandPath("../hoisted-hello_def456.tar.gz"))
	manifest := &pods.PodManifest{}
	manifest.Id = "hello"
	stanza := pods.LaunchableStanza{
		LaunchableId:   "hello",
		LaunchableType: "hoist",
		Location:       hello,
	}
	manifest.LaunchableStanzas = map[string]pods.LaunchableStanza{
		"hello": stanza,
	}

	store, err := intent.LookupStore(intent.Options{})
	if err != nil {
		return err
	}
	hostname, _ := os.Hostname()
	_, err = store.SetPod(hostname, *manifest)
	return err
}

func verifyHelloRunning() error {
	helloPidAppeared := make(chan struct{})
	quit := make(chan struct{})
	defer close(quit)
	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			res := exec.Command("sudo", "sv", "stat", "/var/service/hello__hello__launch").Run()
			if res == nil {
				select {
				case <-quit:
					fmt.Println("got a valid stat after timeout")
				case helloPidAppeared <- struct{}{}:
				}
				return
			} else {
				select {
				case <-quit:
					return
				default:
				}
			}
		}
	}()
	select {
	case <-time.After(20 * time.Second):
		var helloTail, preparerTail bytes.Buffer
		helloT := exec.Command("tail", "/var/service/hello__hello/log/main/current")
		helloT.Stdout = &helloTail
		helloT.Run()
		preparerT := exec.Command("tail", "/var/service/preparer__preparer/log/main/current")
		preparerT.Stdout = &preparerTail
		preparerT.Run()
		return fmt.Errorf("Couldn't start hello after 15 seconds: \n\n hello tail: \n%s\n\n preparer tail: \n%s", helloTail.String(), preparerTail.String())
	case <-helloPidAppeared:
		return nil
	}
}
