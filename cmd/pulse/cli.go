package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const updateRepository = "AmiwrM/Pulse"

type serviceTarget struct {
	unit  string
	label string
}

func isAgentApplet(name string) bool {
	name = strings.TrimSuffix(strings.ToLower(filepath.Base(name)), ".exe")
	return name == "pulse-agent" || name == "pulse-a"
}

func serviceTargetFor(agent bool) serviceTarget {
	if agent {
		return serviceTarget{unit: "pulse-agent", label: "Pulse agent"}
	}
	return serviceTarget{unit: "pulse", label: "Pulse panel"}
}

func managementHelp(agent bool) string {
	command := "pulse"
	if agent {
		command = "pulse-agent"
	}
	return fmt.Sprintf(`%s service commands:

  %s restart           restart the service
  %s status            show service status
  %s logs              show the last service logs
  %s update            update the binary from the latest GitHub release
  %s update --agent    update and restart the agent service

Options:
  --agent               target %s instead of %s
  --release TAG         update to a specific release (default: latest)
  --no-restart          update the binary without restarting the service
`, command, command, command, command, command, command,
		serviceTargetFor(true).label, serviceTargetFor(false).label)
}

// handleManagementCLI handles the small operational commands before the main
// flag parser runs. This keeps `pulse --agent`, `pulse --addr`, etc. backward
// compatible while adding service management commands.
func handleManagementCLI(args []string) bool {
	appletAgent := isAgentApplet(os.Args[0])
	rest := append([]string(nil), args...)
	agentTarget := appletAgent
	if len(rest) > 0 && strings.EqualFold(rest[0], "agent") {
		agentTarget = true
		rest = rest[1:]
	}
	if len(rest) == 0 {
		if appletAgent {
			fmt.Print(managementHelp(true))
			return true
		}
		return false
	}
	command := strings.ToLower(rest[0])
	if (command == "help" || command == "--help" || command == "-h") && len(rest) == 1 {
		fmt.Print(managementHelp(agentTarget))
		return true
	}
	switch command {
	case "restart":
		return handleRestart(rest[1:], agentTarget)
	case "status":
		return handleServiceStatus(rest[1:], agentTarget)
	case "logs":
		return handleServiceLogs(rest[1:], agentTarget)
	case "update":
		return handleUpdate(rest[1:], agentTarget)
	default:
		if appletAgent {
			fmt.Fprintf(os.Stderr, "Unknown agent command: %s\n\n%s", rest[0], managementHelp(true))
			return true
		}
		return false
	}
}

func agentFlag(fs *flag.FlagSet, agent *bool, defaultAgent bool) {
	*agent = defaultAgent
	fs.BoolVar(agent, "agent", defaultAgent, "target the pulse-agent service")
}

func parseCommandFlags(fs *flag.FlagSet, args []string) (bool, error) {
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return false, err
	}
	return true, nil
}

func handleRestart(args []string, defaultAgent bool) bool {
	fs := flag.NewFlagSet("restart", flag.ContinueOnError)
	var agent bool
	agentFlag(fs, &agent, defaultAgent)
	fs.Usage = func() { fmt.Print(managementHelp(agent)) }
	if ok, err := parseCommandFlags(fs, args); !ok {
		if errors.Is(err, flag.ErrHelp) {
			return true
		}
		return true
	}
	target := serviceTargetFor(agent)
	if err := requireRoot(); err != nil {
		cliExit(err)
	}
	if err := runSystemctl("restart", target.unit); err != nil {
		cliExit(fmt.Errorf("restart failed: %w", err))
	}
	fmt.Printf("%s restarted (%s).\n", target.label, target.unit)
	return true
}

func handleServiceStatus(args []string, defaultAgent bool) bool {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	var agent bool
	agentFlag(fs, &agent, defaultAgent)
	fs.Usage = func() { fmt.Print(managementHelp(agent)) }
	if ok, err := parseCommandFlags(fs, args); !ok {
		if errors.Is(err, flag.ErrHelp) {
			return true
		}
		return true
	}
	target := serviceTargetFor(agent)
	cmd := exec.Command("systemctl", "status", target.unit, "--no-pager", "--lines=25")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return true
	}
	return true
}

func handleServiceLogs(args []string, defaultAgent bool) bool {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	var agent bool
	agentFlag(fs, &agent, defaultAgent)
	fs.Usage = func() { fmt.Print(managementHelp(agent)) }
	if ok, err := parseCommandFlags(fs, args); !ok {
		if errors.Is(err, flag.ErrHelp) {
			return true
		}
		return true
	}
	target := serviceTargetFor(agent)
	cmd := exec.Command("journalctl", "-u", target.unit, "-n", "120", "--no-pager")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return true
	}
	return true
}

func handleUpdate(args []string, defaultAgent bool) bool {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	var agent, noRestart bool
	release := "latest"
	agentFlag(fs, &agent, defaultAgent)
	fs.StringVar(&release, "release", "latest", "release tag to install")
	fs.BoolVar(&noRestart, "no-restart", false, "do not restart the service")
	fs.Usage = func() { fmt.Print(managementHelp(agent)) }
	if ok, err := parseCommandFlags(fs, args); !ok {
		if errors.Is(err, flag.ErrHelp) {
			return true
		}
		return true
	}
	target := serviceTargetFor(agent)
	if err := requireRoot(); err != nil {
		cliExit(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	newVersion, err := updateBinary(ctx, release)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "If this install has no GitHub release yet, update with the installer instead:")
		fmt.Fprintln(os.Stderr, "  sudo bash /tmp/pulse-install.sh --source")
		os.Exit(1)
	}
	fmt.Printf("Pulse binary updated%s.\n", newVersion)
	if noRestart {
		fmt.Println("Service restart skipped (--no-restart).")
		return true
	}
	if err := runSystemctl("restart", target.unit); err != nil {
		fmt.Fprintf(os.Stderr, "Binary updated, but service restart failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run manually: systemctl restart %s\n", target.unit)
		os.Exit(1)
	}
	fmt.Printf("%s restarted (%s).\n", target.label, target.unit)
	return true
}

func requireRoot() error {
	if runtime.GOOS != "linux" {
		return errors.New("this command is supported on Linux VPS hosts only")
	}
	out, err := exec.Command("id", "-u").Output()
	if err != nil {
		return errors.New("could not determine the current user")
	}
	if strings.TrimSpace(string(out)) != "0" {
		return errors.New("run this command as root (use sudo)")
	}
	return nil
}

func cliExit(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func runSystemctl(action, unit string) error {
	cmd := exec.Command("systemctl", action, unit)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func updateBinary(ctx context.Context, release string) (string, error) {
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		return "", fmt.Errorf("unsupported architecture: %s", arch)
	}
	asset := "pulse-linux-" + arch
	base := "https://github.com/" + updateRepository + "/releases/latest/download/"
	if release != "" && release != "latest" {
		if !strings.HasPrefix(release, "v") {
			release = "v" + release
		}
		base = "https://github.com/" + updateRepository + "/releases/download/" + release + "/"
	}
	client := &http.Client{Timeout: 3 * time.Minute}
	binaryData, err := fetch(ctx, client, base+asset)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", asset, err)
	}
	if len(binaryData) < 1024 || len(binaryData) > 128<<20 {
		return "", fmt.Errorf("downloaded binary has an unexpected size: %d bytes", len(binaryData))
	}
	sumData, err := fetch(ctx, client, base+"SHA256SUMS")
	if err != nil {
		return "", fmt.Errorf("download SHA256SUMS: %w", err)
	}
	want := checksumFor(string(sumData), asset)
	if want == "" {
		return "", fmt.Errorf("no checksum entry for %s", asset)
	}
	got := sha256.Sum256(binaryData)
	if hex.EncodeToString(got[:]) != strings.ToLower(want) {
		return "", errors.New("downloaded binary checksum does not match")
	}

	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".pulse-update-*")
	if err != nil {
		return "", fmt.Errorf("create update file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(binaryData); err != nil {
		return "", err
	}
	if err := tmp.Chmod(0755); err != nil {
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	versionOut, err := exec.Command(tmpPath, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("downloaded binary is not runnable: %w", err)
	}
	version := strings.TrimSpace(string(versionOut))
	if !strings.HasPrefix(version, "pulse ") {
		return "", fmt.Errorf("unexpected binary version output: %s", version)
	}
	if err := os.Rename(tmpPath, exe); err != nil {
		return "", fmt.Errorf("replace %s: %w", exe, err)
	}
	return " (" + strings.TrimPrefix(version, "pulse ") + ")", nil
}

func fetch(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Pulse-Updater/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("no published release asset found")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 128<<20))
}

func checksumFor(sums, asset string) string {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == asset {
			return fields[0]
		}
	}
	return ""
}
