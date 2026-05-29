//go:build linux

package rootless

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/rootless-containers/rootlesskit/v2/pkg/child"
	"github.com/rootless-containers/rootlesskit/v2/pkg/parent"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const (
	// envPipeFD is the env key rootlesskit uses to pass the sync pipe FD.
	envPipeFD = "_YAP_ROOTLESSKIT_PIPE_FD"
	// envChildActivation is the env key for the child activation signal.
	envChildActivation = "_YAP_ROOTLESSKIT_CHILD_USE_ACTIVATION"
	// envStateDir is the env key for propagating the state directory.
	envStateDir = "_YAP_ROOTLESSKIT_STATE_DIR"
	// envExecMode signals that this re-exec should pivot into rootfs and exec the target.
	envExecMode = "_YAP_ROOTLESSKIT_EXEC"
	// envChildRootfs is the rootfs path passed to the child.
	envChildRootfs = "_YAP_ROOTLESSKIT_ROOTFS"
	// envChildWorkDir is the workspace path passed to the child.
	envChildWorkDir = "_YAP_ROOTLESSKIT_WORKDIR"
	// envChildArgs is the serialised command args passed to the child (argSep-separated).
	envChildArgs = "_YAP_ROOTLESSKIT_ARGS"
)

// MaybeRunAsChild checks whether the current process was re-executed as the
// rootlesskit child. If so, it completes the child initialisation and runs the
// target command inside the new user namespace, then exits.
//
// Call this as early as possible in main(), before cobra runs.
func MaybeRunAsChild() {
	// rootlesskit child re-exec: complete namespace setup.
	if os.Getenv(envPipeFD) != "" {
		if err := runChild(); err != nil {
			fmt.Fprintf(os.Stderr, "yap rootless child: %v\n", err)
			os.Exit(1)
		}

		os.Exit(0)
	}

	// exec-mode re-exec: pivot into rootfs and exec the target command.
	if os.Getenv(envExecMode) != "" {
		if err := runExec(); err != nil {
			fmt.Fprintf(os.Stderr, "yap rootless exec: %v\n", err)
			os.Exit(1)
		}

		os.Exit(0) // unreachable if syscall.Exec succeeds
	}
}

// runChild is called inside the new user namespace (child side of rootlesskit).
// It configures the child namespace and sets TargetCmd to re-exec this binary
// in exec-mode, which will pivot into the rootfs.
func runChild() error {
	// TargetCmd: re-exec this binary with envExecMode set so it pivots into rootfs.
	targetCmd := []string{"/proc/self/exe"}

	childOpt := child.Opt{
		PipeFDEnvKey:              envPipeFD,
		RunActivationHelperEnvKey: envChildActivation,
		ChildUseActivationEnvKey:  envChildActivation,
		StateDirEnvKey:            envStateDir,
		TargetCmd:                 targetCmd,
		MountProcfs:               true,
		Propagation:               "rprivate",
	}

	return child.Child(childOpt)
}

// runExec pivots into the rootfs and execs the target command.
// Called when envExecMode is set (second re-exec inside the user namespace).
func runExec() error {
	rootfs := os.Getenv(envChildRootfs)
	workDir := os.Getenv(envChildWorkDir)
	argsRaw := os.Getenv(envChildArgs)

	if rootfs == "" {
		return errors.New(errors.ErrTypeConfiguration, "missing rootfs environment variable").
			WithOperation("runExec")
	}

	var args []string

	if argsRaw != "" {
		for _, a := range splitNUL(argsRaw) {
			if a != "" {
				args = append(args, a)
			}
		}
	}

	return execInRootfs(rootfs, workDir, args)
}

// RunInRootless runs args inside the distro rootfs using rootlesskit for
// user-namespace isolation. workDir is bind-mounted as /project.
func RunInRootless(distro, workDir string, args []string) error {
	rootfs, err := rootfsPath(distro)
	if err != nil {
		return err
	}

	if _, err := os.Stat(rootfs); os.IsNotExist(err) {
		return errors.New(errors.ErrTypeFileSystem,
			fmt.Sprintf("rootfs not found for %s — run 'yap pull %s' first", distro, distro)).
			WithOperation("RunInRootless")
	}

	stateDir, err := os.MkdirTemp("", "yap-rootlesskit-*")
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to create rootlesskit state dir").
			WithOperation("RunInRootless")
	}

	defer func() {
		if err := os.RemoveAll(stateDir); err != nil {
			logger.Warn("failed to remove rootlesskit state dir", "error", err)
		}
	}()

	logger.Info("starting rootless container", "distro", distro, "rootfs", rootfs)

	// Set env vars that the re-executed child will read.
	for k, v := range map[string]string{
		envExecMode:        "1",
		envChildRootfs:     rootfs,
		envChildWorkDir:    workDir,
		envChildArgs:       joinNUL(args),
		"YAP_IN_CONTAINER": "1",
	} {
		if err := os.Setenv(k, v); err != nil {
			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to set environment variable").
				WithOperation("RunInRootless").
				WithContext("key", k)
		}
	}

	parentOpt := parent.Opt{
		PipeFDEnvKey:             envPipeFD,
		ChildUseActivationEnvKey: envChildActivation,
		StateDir:                 stateDir,
		StateDirEnvKey:           envStateDir,
		CreatePIDNS:              true,
		Propagation:              "rprivate",
		SubidSource:              parent.SubidSourceAuto,
	}

	if err := parent.Parent(parentOpt); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild,
			"rootless container exited with error").
			WithOperation("RunInRootless").
			WithContext("distro", distro)
	}

	return nil
}

// execInRootfs bind-mounts /proc, /sys, /dev and workDir into rootfs,
// pivots into it, then execs args.
func execInRootfs(rootfs, workDir string, args []string) error {
	// Bind-mount /proc, /sys, /dev from host into rootfs.
	for _, dir := range []string{"proc", "sys", "dev"} {
		dest := filepath.Join(rootfs, dir)

		if err := bindMount("/"+dir, dest); err != nil {
			logger.Warn("bind mount failed", "dir", dir, "error", err)
		}
	}

	// Bind-mount workspace.
	if workDir != "" && workDir != "." {
		wsTarget := filepath.Join(rootfs, "project")

		if err := bindMount(workDir, wsTarget); err != nil {
			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to bind mount workspace").
				WithOperation("execInRootfs")
		}
	}

	// Provide working DNS inside the rootfs (best-effort).
	setupResolvConf(rootfs)

	if err := pivotOrChroot(rootfs); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to pivot root or chroot").
			WithOperation("execInRootfs")
	}

	if len(args) == 0 {
		return errors.New(errors.ErrTypeConfiguration, "no command specified").
			WithOperation("execInRootfs")
	}

	bin, err := exec.LookPath(args[0])
	if err != nil {
		bin = args[0]
	}

	// Clear the rootless control env vars before handing off to the real
	// target. Otherwise the exec'd binary re-enters MaybeRunAsChild (called
	// early in main, before cobra), sees envExecMode still set, and re-runs
	// execInRootfs a SECOND time — now inside the already-pivoted rootfs,
	// where the host-absolute workDir no longer exists, producing a spurious
	// ENOENT on the workspace bind. YAP_IN_CONTAINER is intentionally kept so
	// the inner process does not re-dispatch into a container.
	for _, k := range []string{envExecMode, envChildRootfs, envChildWorkDir, envChildArgs} {
		_ = os.Unsetenv(k)
	}

	return syscall.Exec(bin, args, os.Environ()) //nolint:gosec
}

// pivotOrChroot switches the process root to newRoot.
// Tries pivot_root first (preferred), falls back to chroot.
func pivotOrChroot(newRoot string) error {
	putOld := filepath.Join(newRoot, ".put_old")

	if err := os.MkdirAll(putOld, 0o700); err != nil { //nolint:gosec // path from rootfsPath, not user input
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create put_old directory").
			WithOperation("pivotOrChroot")
	}

	if err := syscall.PivotRoot(newRoot, putOld); err == nil {
		if err := syscall.Unmount("/.put_old", syscall.MNT_DETACH); err != nil {
			logger.Warn("failed to unmount old root", "error", err)
		}

		_ = os.Remove("/.put_old")

		return nil
	}

	// Fallback: plain chroot.
	if err := syscall.Chroot(newRoot); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to chroot").
			WithOperation("pivotOrChroot").
			WithContext("path", newRoot)
	}

	return os.Chdir("/")
}

// setupResolvConf bind-mounts the host's /etc/resolv.conf into the rootfs so
// name resolution works inside the pivoted environment. rootlesskit shares the
// host network namespace, so the host resolver is reachable — but the builder
// rootfs ships an empty /etc/resolv.conf, which makes glibc fall back to
// localhost (::1:53) and every lookup fails. Best-effort: all failures are
// logged and ignored so a missing resolv.conf never aborts the build.
func setupResolvConf(rootfs string) {
	hostResolv, err := filepath.EvalSymlinks("/etc/resolv.conf")
	if err != nil {
		return
	}

	if _, err := os.Stat(hostResolv); err != nil {
		return
	}

	dst := filepath.Join(rootfs, "etc", "resolv.conf")

	// Ensure a regular file exists at dst to bind onto.
	if _, err := os.Stat(dst); os.IsNotExist(err) { //nolint:gosec // path from rootfsPath, not user input
		_ = os.MkdirAll(filepath.Dir(dst), 0o755) //nolint:gosec // path from rootfsPath, not user input

		f, cerr := os.OpenFile(dst, os.O_CREATE, 0o644) //nolint:gosec // path from rootfsPath, not user input
		if cerr == nil {
			_ = f.Close()
		}
	}

	if err := syscall.Mount(hostResolv, dst, "", syscall.MS_BIND, ""); err != nil {
		logger.Warn("failed to bind mount resolv.conf", "error", err)
	}
}

// bindMount bind-mounts src to dest.
func bindMount(src, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil { //nolint:gosec // path from rootfsPath, not user input
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create mount destination directory").
			WithOperation("bindMount").
			WithContext("path", dest)
	}

	return syscall.Mount(src, dest, "", syscall.MS_BIND|syscall.MS_REC, "")
}

// argSep separates encoded child args. We use ASCII Unit Separator (0x1f)
// rather than NUL: the encoded value is passed through os.Setenv, which
// rejects any string containing a NUL byte ("setenv: invalid argument"), so a
// NUL separator broke every command with 2+ args. 0x1f never appears in real
// argv (paths, flags) yet is still a safe in-band delimiter.
const argSep = '\x1f'

// joinNUL encodes a string slice as argSep-separated bytes.
func joinNUL(ss []string) string {
	var b strings.Builder

	for i, s := range ss {
		if i > 0 {
			b.WriteByte(argSep)
		}

		b.WriteString(s)
	}

	return b.String()
}

// splitNUL decodes an argSep-separated string into a slice.
func splitNUL(s string) []string {
	var result []string

	cur := &strings.Builder{}

	for _, c := range s {
		if c == argSep {
			result = append(result, cur.String())
			cur.Reset()
		} else {
			cur.WriteRune(c)
		}
	}

	result = append(result, cur.String())

	return result
}
