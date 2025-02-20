// Package tutils provides utilities for running AIS operator tests
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package tutils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

func StreamLogs(ctx context.Context, namespace string) (err error) {
	cmd := exec.Command("kubectl", "logs", "-f",
		"--max-log-requests", "10",
		"-l", "app.kubernetes.io/component in (proxy,target)",
		"-n", namespace,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdin
	if err = cmd.Start(); err != nil {
		return
	}
	fmt.Println("AIStore logs started")

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
		close(waitCh)
	}()

	go func() {
		defer fmt.Println("AIStore logs finished")

		select {
		case <-ctx.Done():
			cmd.Process.Kill()
			return
		case err = <-waitCh:
			if err != nil {
				fmt.Fprintf(os.Stderr, "Logs streaming ended with err: %v\n", err)
			}
			return
		}
	}()

	return nil
}
