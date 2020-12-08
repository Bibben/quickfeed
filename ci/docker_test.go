package ci_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/autograde/quickfeed/ci"
	"github.com/docker/docker/client"
)

var docker bool

func init() {
	if os.Getenv("DOCKER_TESTS") != "" {
		docker = true
	}
	cli, err := client.NewEnvClient()
	if err != nil {
		docker = false
	}
	if _, err := cli.Ping(context.Background()); err != nil {
		docker = false
	}
}

func TestDocker(t *testing.T) {
	if !docker {
		t.SkipNow()
	}

	const (
		script  = `echo -n "hello world"`
		wantOut = "hello world"
	)

	docker, err := ci.NewDockerCI()
	if err != nil {
		t.Fatalf("failed to set up docker client: %v", err)
	}
	defer docker.Close()

	out, err := docker.Run(context.Background(), &ci.Job{
		Name:     "TestDocker-" + randomString(t),
		Image:    "golang:latest",
		Commands: []string{script},
	})
	if err != nil {
		t.Fatal(err)
	}

	if out != wantOut {
		t.Errorf("docker.Run(%#v) = %#v, want %#v", script, out, wantOut)
	}
}

func TestDockerTimeout(t *testing.T) {
	if !docker {
		t.SkipNow()
	}

	const (
		script  = `echo -n "hello," && sleep 10`
		wantOut = `Container timeout. Please check for infinite loops or other slowness.`
	)

	// Note that the timeout value below is sensitive to startup time of the container.
	// If the timeout is too short, the Run() call may not reach the ContainerWait() call.
	// Hence, if this test fails, you may try to increase the timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	docker, err := ci.NewDockerCI()
	if err != nil {
		t.Fatalf("failed to set up docker client: %v", err)
	}
	defer docker.Close()

	out, err := docker.Run(ctx, &ci.Job{
		Name:     "TestDockerTimeout-" + randomString(t),
		Image:    "golang:latest",
		Commands: []string{script},
	})
	if out != wantOut {
		t.Errorf("docker.Run(%#v) = %#v, want %#v", script, out, wantOut)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("docker.Run(%#v) = %#v, want %#v", script, err.Error(), context.DeadlineExceeded.Error())
	}
	if err == nil {
		t.Errorf("docker.Run(%#v) unexpectedly returned without error", script)
	}
}

func TestDockerOpenFileDescriptors(t *testing.T) {
	// This is mainly for debugging the 'too many open file descriptors' issue
	if !docker {
		t.SkipNow()
	}

	const (
		script        = `echo -n "hello, " && sleep 2 && echo -n "world!"`
		wantOut       = "hello, world!"
		numContainers = 5
	)

	docker, err := ci.NewDockerCI()
	if err != nil {
		t.Fatalf("failed to set up docker client: %v", err)
	}
	defer docker.Close()

	errCh := make(chan error, numContainers)
	for i := 0; i < numContainers; i++ {
		go func(j int) {
			name := fmt.Sprintf("TestDockerOpenFileDescritors-%d-%s", j, randomString(t))
			out, err := docker.Run(context.Background(), &ci.Job{
				Name:     name,
				Image:    "golang:latest",
				Commands: []string{script},
			})
			if err != nil {
				errCh <- err
			}
			if out != wantOut {
				t.Errorf("docker.Run(%#v) = %#v, want %#v", script, out, wantOut)
			}
			errCh <- nil
		}(i)
	}
	afterContainersStarted := countOpenFiles(t)

	for i := 0; i < numContainers; i++ {
		err := <-errCh
		if err != nil {
			t.Fatal(err)
		}
	}
	close(errCh)
	afterContainersFinished := countOpenFiles(t)
	if afterContainersFinished > afterContainersStarted {
		t.Errorf("finished %d > started %d", afterContainersFinished, afterContainersStarted)
	}
}

func countOpenFiles(t *testing.T) int {
	t.Helper()
	out, err := exec.Command("/bin/sh", "-c", fmt.Sprintf("lsof -p %v", os.Getpid())).Output()
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Count(out, []byte("\n"))
}

func randomString(t *testing.T) string {
	t.Helper()
	randomness := make([]byte, 10)
	_, err := rand.Read(randomness)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha1.Sum(randomness))[:6]
}

func TestDockerLogLimit(t *testing.T) {
	// This is just for testing
	t.SkipNow()
	const maxLogSize = 4
	const lastSegmentSize = 5
	logReader := strings.NewReader("want only that some small last thing")
	var stdout bytes.Buffer
	n, err := io.Copy(&stdout, logReader)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("x(%d=%d): %s", n, stdout.Len(), stdout.String())
	if stdout.Len() > maxLogSize {
		all := stdout.String()
		t.Logf("%s ONLY %s", all[0:maxLogSize], all[len(all)-lastSegmentSize:])
	}
	t.Logf("x(%d=%d): %s", n, stdout.Len(), stdout.String())
}
