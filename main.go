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
	return res
}

func PathForParam(dir, param string) string {
	return filepath.Join(dir, url.PathEscape(param))
}

func main() {
	args := os.Args[1:]

	sepIdx := LastIndex(args, ":::")
	if sepIdx == -1 {
		log.Fatal("::: not found")
	}

	diffCmd := []string{"diff"}
	cmdTmpl := args[:sepIdx]
	if idx := slices.Index(cmdTmpl, "--"); idx != -1 {
		diffCmd = cmdTmpl[:idx]
		cmdTmpl = cmdTmpl[idx+1:]
	}
	params := args[sepIdx+1:]

	tmpRoot := "/tmp"
	if _, err := os.Stat("/dev/shm"); err == nil {
		tmpRoot = "/dev/shm"
	}

	dir, err := os.MkdirTemp(tmpRoot, "cmdiff-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	var wg errgroup.Group
	for p := range slices.Values(params) {
		// cmds = append(cmds, ReplaceCmdTmpl(cmdTmpl, p))
		cmd := ReplaceCmdTmpl(cmdTmpl, p)
		wg.Go(func() error {
			w, err := os.Create(PathForParam(dir, p))
			if err != nil {
				return err
			}
			defer w.Close()
			e := exec.Command(cmd[0], cmd[1:]...)
			e.Stdout = w
			if err := e.Run(); err != nil {
				return fmt.Errorf("%s failed: %w", p, err)
			}
			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		log.Print(err)
		return
	}

	for i := range len(params) - 1 {
		diffArgs := diffCmd[1:]
		diffArgs = append(diffArgs, PathForParam(dir, params[i]), PathForParam(dir, params[i+1]))
		d := exec.Command(diffCmd[0], diffArgs...)
		d.Stdin = os.Stdin
		d.Stdout = os.Stdout
		d.Stderr = os.Stderr

		if err := d.Start(); err != nil {
			log.Printf("failed to start diff: %s", err)
			return
		}

		d.Wait()
	}

}
