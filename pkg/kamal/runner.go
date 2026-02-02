package kamal

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// RunOptions are common options for Kamal CLI.
type RunOptions struct {
	Cwd         string
	ConfigFile  string
	Destination string
	Primary     bool
	Hosts       string
	Roles       string
	Version     string
	SkipHooks   bool
	Verbose     bool
	Quiet       bool
}

// Result holds stdout, stderr and exit code.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func (r Result) Combined() string {
	if r.Stderr != "" {
		return r.Stdout + "\n" + r.Stderr
	}
	return r.Stdout
}

func buildGlobalArgs(opts RunOptions) []string {
	var args []string
	if opts.ConfigFile != "" {
		args = append(args, "--config-file", opts.ConfigFile)
	}
	if opts.Destination != "" && opts.Destination != "production" {
		args = append(args, "--destination", opts.Destination)
	}
	if opts.Primary {
		args = append(args, "--primary")
	}
	if opts.Hosts != "" {
		args = append(args, "--hosts", opts.Hosts)
	}
	if opts.Roles != "" {
		args = append(args, "--roles", opts.Roles)
	}
	if opts.Version != "" {
		args = append(args, "--version", opts.Version)
	}
	if opts.SkipHooks {
		args = append(args, "--skip-hooks")
	}
	if opts.Verbose {
		args = append(args, "--verbose")
	}
	if opts.Quiet {
		args = append(args, "--quiet")
	}
	return args
}

// RunKamal runs the kamal CLI with the given subcommand and options.
func RunKamal(subcommand []string, opts RunOptions) (Result, error) {
	args := append(buildGlobalArgs(opts), subcommand...)
	cmd := exec.Command("kamal", args...)
	cmd.Dir = opts.Cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
		err = nil
	} else if err != nil {
		return Result{}, err
	}
	return Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: code,
	}, nil
}

// RunKamalStream runs kamal with the given subcommand and streams stdout+stderr
// line-by-line to onLine. It returns when the command exits or stopCh is closed.
// onLine is called from a goroutine; the caller may use it to update UI (e.g. append to log).
func RunKamalStream(subcommand []string, opts RunOptions, onLine func(line string), stopCh <-chan struct{}) error {
	args := append(buildGlobalArgs(opts), subcommand...)
	cmd := exec.Command("kamal", args...)
	cmd.Dir = opts.Cwd
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	readLines := func(r io.Reader) {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			select {
			case <-stopCh:
				return
			default:
				onLine(sc.Text())
			}
		}
	}
	go readLines(stdout)
	go readLines(stderr)
	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-stopCh:
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return nil
	}
}

// RunOpts builds RunOptions from CWD and optional destination.
func RunOpts(cwd string, dest *DeployDestination) RunOptions {
	o := RunOptions{Cwd: cwd}
	if dest != nil {
		o.ConfigFile = dest.ConfigPath
		if dest.Name != "production" {
			o.Destination = dest.Name
		}
	}
	return o
}

// Deploy runs kamal deploy (optionally with --skip-push).
func Deploy(opts RunOptions, skipPush bool) (Result, error) {
	if skipPush {
		return RunKamal([]string{"deploy", "--skip-push"}, opts)
	}
	return RunKamal([]string{"deploy"}, opts)
}

// Redeploy runs kamal redeploy.
func Redeploy(opts RunOptions) (Result, error) {
	return RunKamal([]string{"redeploy"}, opts)
}

// Rollback runs kamal rollback [version].
func Rollback(opts RunOptions, version string) (Result, error) {
	if version != "" {
		return RunKamal([]string{"rollback", version}, opts)
	}
	return RunKamal([]string{"rollback"}, opts)
}

// Setup runs kamal setup.
func Setup(opts RunOptions) (Result, error) {
	return RunKamal([]string{"setup"}, opts)
}

// Remove runs kamal remove.
func Remove(opts RunOptions) (Result, error) {
	return RunKamal([]string{"remove"}, opts)
}

// Prune runs kamal prune.
func Prune(opts RunOptions) (Result, error) {
	return RunKamal([]string{"prune"}, opts)
}

// Config runs kamal config.
func Config(opts RunOptions) (Result, error) {
	return RunKamal([]string{"config"}, opts)
}

// Details runs kamal details.
func Details(opts RunOptions) (Result, error) {
	return RunKamal([]string{"details"}, opts)
}

// Audit runs kamal audit.
func Audit(opts RunOptions) (Result, error) {
	return RunKamal([]string{"audit"}, opts)
}

// Version runs kamal version.
func Version(opts RunOptions) (Result, error) {
	return RunKamal([]string{"version"}, opts)
}

// LockStatus runs kamal lock status.
func LockStatus(opts RunOptions) (Result, error) {
	return RunKamal([]string{"lock", "status"}, opts)
}

// LockAcquire runs kamal lock acquire.
func LockAcquire(opts RunOptions) (Result, error) {
	return RunKamal([]string{"lock", "acquire"}, opts)
}

// LockRelease runs kamal lock release.
func LockRelease(opts RunOptions) (Result, error) {
	return RunKamal([]string{"lock", "release"}, opts)
}

// LockReleaseForce runs kamal lock release --force.
func LockReleaseForce(opts RunOptions) (Result, error) {
	return RunKamal([]string{"lock", "release", "--force"}, opts)
}

// Build runs kamal build (build application image only, no deploy).
func Build(opts RunOptions) (Result, error) {
	return RunKamal([]string{"build"}, opts)
}

// RegistryLogin runs kamal registry login.
func RegistryLogin(opts RunOptions) (Result, error) {
	return RunKamal([]string{"registry", "login"}, opts)
}

// RegistryLogout runs kamal registry logout.
func RegistryLogout(opts RunOptions) (Result, error) {
	return RunKamal([]string{"registry", "logout"}, opts)
}

// Secrets runs kamal secrets (helpers for extracting secrets; use subcommand via RunKamal if needed).
func Secrets(opts RunOptions) (Result, error) {
	return RunKamal([]string{"secrets"}, opts)
}

// EnvPush runs kamal env push (push environment variables to servers).
func EnvPush(opts RunOptions) (Result, error) {
	return RunKamal([]string{"env", "push"}, opts)
}

// EnvPull runs kamal env pull (pull environment variables from servers).
func EnvPull(opts RunOptions) (Result, error) {
	return RunKamal([]string{"env", "pull"}, opts)
}

// EnvDelete runs kamal env delete (delete environment variables from servers).
func EnvDelete(opts RunOptions) (Result, error) {
	return RunKamal([]string{"env", "delete"}, opts)
}

// Docs runs kamal docs [SECTION] (Kamal configuration documentation).
func Docs(opts RunOptions, section string) (Result, error) {
	if section != "" {
		return RunKamal([]string{"docs", section}, opts)
	}
	return RunKamal([]string{"docs"}, opts)
}

// Help runs kamal help [COMMAND].
func Help(opts RunOptions, command string) (Result, error) {
	if command != "" {
		return RunKamal([]string{"help", command}, opts)
	}
	return RunKamal([]string{"help"}, opts)
}

// Init runs kamal init (create config stub and .kamal secrets stub).
func Init(opts RunOptions) (Result, error) {
	return RunKamal([]string{"init"}, opts)
}

// Upgrade runs kamal upgrade (Kamal 1.x to 2.0).
func Upgrade(opts RunOptions) (Result, error) {
	return RunKamal([]string{"upgrade"}, opts)
}

// App subcommands
func AppBoot(opts RunOptions) (Result, error)    { return RunKamal([]string{"app", "boot"}, opts) }
func AppStart(opts RunOptions) (Result, error)     { return RunKamal([]string{"app", "start"}, opts) }
func AppStop(opts RunOptions) (Result, error)      { return RunKamal([]string{"app", "stop"}, opts) }
func AppRestart(opts RunOptions) (Result, error)   { return RunKamal([]string{"app", "restart"}, opts) }
func AppLogs(opts RunOptions) (Result, error)      { return RunKamal([]string{"app", "logs"}, opts) }
func AppContainers(opts RunOptions) (Result, error) {
	return RunKamal([]string{"app", "containers"}, opts)
}
func AppDetails(opts RunOptions) (Result, error) {
	return RunKamal([]string{"app", "details"}, opts)
}
func AppImages(opts RunOptions) (Result, error) {
	return RunKamal([]string{"app", "images"}, opts)
}
func AppStaleContainers(opts RunOptions) (Result, error) {
	return RunKamal([]string{"app", "stale_containers"}, opts)
}
func AppExec(opts RunOptions, cmd ...string) (Result, error) {
	return RunKamal(append([]string{"app", "exec"}, cmd...), opts)
}
func AppVersion(opts RunOptions) (Result, error) { return RunKamal([]string{"app", "version"}, opts) }
func AppMaintenance(opts RunOptions) (Result, error) {
	return RunKamal([]string{"app", "maintenance"}, opts)
}
func AppLive(opts RunOptions) (Result, error)   { return RunKamal([]string{"app", "live"}, opts) }
func AppRemove(opts RunOptions) (Result, error) { return RunKamal([]string{"app", "remove"}, opts) }

// Server subcommands
func ServerBootstrap(opts RunOptions) (Result, error) {
	return RunKamal([]string{"server", "bootstrap"}, opts)
}
func ServerExec(opts RunOptions, cmd ...string) (Result, error) {
	return RunKamal(append([]string{"server", "exec"}, cmd...), opts)
}

// Accessory subcommands (name can be "all" or specific accessory name)
func AccessoryBoot(opts RunOptions, name string) (Result, error) {
	return RunKamal([]string{"accessory", "boot", name}, opts)
}
func AccessoryStart(opts RunOptions, name string) (Result, error) {
	return RunKamal([]string{"accessory", "start", name}, opts)
}
func AccessoryStop(opts RunOptions, name string) (Result, error) {
	return RunKamal([]string{"accessory", "stop", name}, opts)
}
func AccessoryRestart(opts RunOptions, name string) (Result, error) {
	return RunKamal([]string{"accessory", "restart", name}, opts)
}
func AccessoryReboot(opts RunOptions, name string) (Result, error) {
	return RunKamal([]string{"accessory", "reboot", name}, opts)
}
func AccessoryRemove(opts RunOptions, name string) (Result, error) {
	return RunKamal([]string{"accessory", "remove", name}, opts)
}
func AccessoryDetails(opts RunOptions, name string) (Result, error) {
	return RunKamal([]string{"accessory", "details", name}, opts)
}
func AccessoryLogs(opts RunOptions, name string) (Result, error) {
	return RunKamal([]string{"accessory", "logs", name}, opts)
}
func AccessoryExec(opts RunOptions, name string, cmd ...string) (Result, error) {
	return RunKamal(append([]string{"accessory", "exec", name}, cmd...), opts)
}
func AccessoryUpgrade(opts RunOptions) (Result, error) {
	return RunKamal([]string{"accessory", "upgrade"}, opts)
}

// Proxy subcommands
func ProxyBoot(opts RunOptions) (Result, error)    { return RunKamal([]string{"proxy", "boot"}, opts) }
func ProxyStart(opts RunOptions) (Result, error)  { return RunKamal([]string{"proxy", "start"}, opts) }
func ProxyStop(opts RunOptions) (Result, error)   { return RunKamal([]string{"proxy", "stop"}, opts) }
func ProxyRestart(opts RunOptions) (Result, error) { return RunKamal([]string{"proxy", "restart"}, opts) }
func ProxyReboot(opts RunOptions, rolling bool) (Result, error) {
	if rolling {
		return RunKamal([]string{"proxy", "reboot", "--rolling"}, opts)
	}
	return RunKamal([]string{"proxy", "reboot"}, opts)
}
func ProxyLogs(opts RunOptions) (Result, error)   { return RunKamal([]string{"proxy", "logs"}, opts) }
func ProxyDetails(opts RunOptions) (Result, error) { return RunKamal([]string{"proxy", "details"}, opts) }
func ProxyRemove(opts RunOptions) (Result, error) { return RunKamal([]string{"proxy", "remove"}, opts) }

// ProxyBootConfigGet/Set/Reset (deprecated in favor of proxy run config; still available in Kamal).
func ProxyBootConfigGet(opts RunOptions) (Result, error) {
	return RunKamal([]string{"proxy", "boot_config", "get"}, opts)
}
func ProxyBootConfigSet(opts RunOptions) (Result, error) {
	return RunKamal([]string{"proxy", "boot_config", "set"}, opts)
}
func ProxyBootConfigReset(opts RunOptions) (Result, error) {
	return RunKamal([]string{"proxy", "boot_config", "reset"}, opts)
}

// Label returns a short label for the destination (e.g. "myapp (staging)").
func (d *DeployDestination) Label() string {
	return fmt.Sprintf("%s (%s)", d.Service, d.Name)
}

// Lines splits combined output into lines for display.
func (r Result) Lines() []string {
	s := r.Combined()
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
