package main

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	_killMinDelay  = 20 * time.Millisecond
	_killDelaySpan = 180 * time.Millisecond
	_signalExit    = -1
)

func killWriter(self, dir string) ([]string, error) {
	delay, err := killDelay()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(self, "-child", "-dir", dir)
	cmd.Stderr = os.Stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("writer stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start writer: %w", err)
	}

	var (
		acks     bytes.Buffer
		drainErr error
		wg       sync.WaitGroup
	)
	wg.Go(func() {
		_, drainErr = io.Copy(&acks, pipe)
	})

	time.Sleep(delay)
	if err := cmd.Process.Kill(); err != nil {
		return nil, fmt.Errorf("kill writer: %w", err)
	}
	wg.Wait()
	if err := waitKilled(cmd); err != nil {
		return nil, err
	}
	if drainErr != nil {
		return nil, fmt.Errorf("drain writer stdout: %w", drainErr)
	}

	return ackedKeys(acks.String()), nil
}

func killDelay() (time.Duration, error) {
	span, err := rand.Int(rand.Reader, big.NewInt(int64(_killDelaySpan)))
	if err != nil {
		return 0, fmt.Errorf("draw a kill delay: %w", err)
	}

	return _killMinDelay + time.Duration(span.Int64()), nil
}

func waitKilled(cmd *exec.Cmd) error {
	err := cmd.Wait()
	if err == nil {
		return errors.New("the writer exited on its own")
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == _signalExit {
		return nil
	}

	return fmt.Errorf("the writer exited before the kill: %w", err)
}

func ackedKeys(acks string) []string {
	var keys []string
	rest := acks
	for {
		key, tail, whole := strings.Cut(rest, "\n")
		if !whole {
			return keys
		}
		keys = append(keys, key)
		rest = tail
	}
}
