package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func main() {
	uid, err := numericEnv("PUID", 1000)
	if err != nil {
		fatal(err)
	}
	gid, err := numericEnv("PGID", 1000)
	if err != nil {
		fatal(err)
	}

	dataDir := os.Getenv("OPENSYNC_DATA_DIR")
	if dataDir == "" {
		dataDir = "/app/data"
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		fatal(fmt.Errorf("failed to create %s: %w", dataDir, err))
	}
	needChown, err := shouldChownRecursive(dataDir, uid, gid)
	if err != nil {
		fatal(err)
	}
	if needChown {
		if err := chownRecursive(dataDir, uid, gid); err != nil {
			fatal(fmt.Errorf("failed to change ownership of %s to %d:%d: %w", dataDir, uid, gid, err))
		}
	}

	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"./opensync"}
	} else if strings.HasPrefix(args[0], "-") || args[0] == "reset-password" {
		args = append([]string{"./opensync"}, args...)
	}

	if uid != 0 || gid != 0 {
		if err := syscall.Setgroups([]int{gid}); err != nil {
			fatal(fmt.Errorf("failed to set groups: %w", err))
		}
		if err := syscall.Setgid(gid); err != nil {
			fatal(fmt.Errorf("failed to set gid %d: %w", gid, err))
		}
		if err := syscall.Setuid(uid); err != nil {
			fatal(fmt.Errorf("failed to set uid %d: %w", uid, err))
		}
	}

	binary, err := executablePath(args[0])
	if err != nil {
		fatal(err)
	}
	if err := syscall.Exec(binary, args, os.Environ()); err != nil {
		fatal(fmt.Errorf("failed to execute %s: %w", binary, err))
	}
}

func numericEnv(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	if strings.ContainsAny(value, "+-") {
		return 0, fmt.Errorf("%s must be a numeric value", name)
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a numeric value", name)
	}
	return parsed, nil
}

func chownRecursive(root string, uid int, gid int) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		return os.Lchown(path, uid, gid)
	})
}

func shouldChownRecursive(root string, uid int, gid int) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("OPENSYNC_CHOWN"))) {
	case "always", "true", "1":
		return true, nil
	case "never", "false", "0":
		return false, nil
	}

	info, err := os.Lstat(root)
	if err != nil {
		return false, fmt.Errorf("failed to inspect %s ownership: %w", root, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return true, nil
	}
	return int(stat.Uid) != uid || int(stat.Gid) != gid, nil
}

func executablePath(name string) (string, error) {
	if strings.ContainsRune(name, os.PathSeparator) {
		return name, nil
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	if _, err := os.Stat("./" + name); err == nil {
		return "./" + name, nil
	}
	return "", errors.New("executable not found: " + name)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
