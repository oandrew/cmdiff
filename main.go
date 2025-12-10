package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"
)

func LastIndex[T comparable](s []T, t T) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == t {
			return i
		}
	}
	return -1
}

func ReplaceCmdTmpl(s []string, param string) []string {
	res := slices.Clone(s)
	for i := range res {
		res[i] = strings.ReplaceAll(res[i], "{}", param)
	}
	if len(res) == 1 {
		return []string{"/bin/bash", "-c", res[0]}
	} else {
		return res
	}
}

func PathForParam(dir, param string) string {
	return filepath.Join(dir, url.PathEscape(param))
}

func getTmpRoot() (string, error) {
	tmpRoot := "/tmp"
	if _, err := os.Stat("/dev/shm"); err == nil {
		tmpRoot = "/dev/shm"
	}
	dir, err := os.MkdirTemp(tmpRoot, "cmdiff-*")
	if err != nil {
		return "", err
	}
	return dir, nil
}

func run(diffCmdTmpl []string, cmdTmpl []string, params []string) error {
	dir, err := getTmpRoot()
	if err != nil {
		return fmt.Errorf("Failed to create tmp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	var wg errgroup.Group
	for p := range slices.Values(params) {
		cmd := ReplaceCmdTmpl(cmdTmpl, p)
		wg.Go(func() error {
			w, err := os.Create(PathForParam(dir, p))
			if err != nil {
				return err
			}
			defer w.Close()
			c := exec.Command(cmd[0], cmd[1:]...)
			c.Stdout = w
			if err := c.Run(); err != nil {
				return fmt.Errorf("param '%s' failed: %w", p, err)
			}
			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		return err
	}

	if len(params) == 1 {
		f, err := os.Open(PathForParam(dir, params[0]))
		if err != nil {
			return fmt.Errorf("Failed to open output file: %w", err)
		}
		defer f.Close()
		f.WriteTo(os.Stdout)
	} else {
		for i := range len(params) - 1 {
			diffCmd := append(slices.Clone(diffCmdTmpl), PathForParam(dir, params[i]), PathForParam(dir, params[i+1]))
			c := exec.Command(diffCmd[0], diffCmd[1:]...)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				if exitError, ok := err.(*exec.ExitError); !ok || exitError.ExitCode() > 1 {
					return fmt.Errorf("diff failed: %w", err)
				}
			}
		}
	}
	return nil
}

func main() {
	args := os.Args[1:]

	sepIdx := LastIndex(args, ":::")
	if sepIdx == -1 {
		log.Fatal("::: not found")
	}

	diffCmd := []string{"diff"}
	if diffCmdEnv, ok := os.LookupEnv("CMDIFF_DIFF"); ok {
		diffCmd = []string{"/bin/bash", "-c", diffCmdEnv + " " + `"$@"`, "--"}
	}
	cmdTmpl := args[:sepIdx]
	if idx := slices.Index(cmdTmpl, "--"); idx != -1 {
		diffCmd = cmdTmpl[:idx]
		cmdTmpl = cmdTmpl[idx+1:]
	}

	params := args[sepIdx+1:]

	if len(diffCmd) == 0 {
		log.Fatal("diff cmd not specified")
	}

	if err := run(diffCmd, cmdTmpl, params); err != nil {
		log.Fatal(err)
	}

}
