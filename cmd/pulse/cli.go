package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"pulse/internal/model"
	"pulse/internal/store"
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

// appletDisplayName returns the stable name used by `--version`. The updater
// runs the downloaded binary under a temporary name, so the temp file name
// must never leak into the version output.
func appletDisplayName(argv0 string) string {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(argv0)), ".exe")
	if name == "pulse-agent" || name == "pulse-a" {
		return name
	}
	return "pulse"
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
	extra := "  " + command + " port [PORT]      show or change the panel listen port\n"
	if agent {
		extra = ""
	}
	return fmt.Sprintf(`%s management commands:

  %s start             start the service
  %s stop              stop the service
  %s restart           restart the service
  %s status            show service status
  %s logs [options]    show logs (category/level/follow/journal)
  %s update [options]  update the binary from the latest GitHub release
  %s uninstall         remove the service (alias: remove, delete)
%s
Options:
  --agent               target %s instead of %s
  --release TAG         update to a specific release (default: latest)
  --no-restart          update/port change without restarting the service
  --purge               uninstall also removes data/config (with --yes)
  --yes                 do not ask for confirmation
  --lines N             logs: number of entries (default: 120)
  --follow              logs: keep following
  --category NAME       logs: panel, monitor, xray, cloudflare, notify, agent, security, api
  --level LEVEL         logs: debug, info, warn, error
  --q TEXT              logs: search message/detail text
  --since TIME          logs/journal: journalctl since value
  --journal             logs: read journalctl instead of the app log database
  --db PATH             logs: app database path
`, command, command, command, command, command, command, command, command,
		extra, serviceTargetFor(true).label, serviceTargetFor(false).label)
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
	case "start", "stop", "restart":
		return handleServiceAction(command, rest[1:], agentTarget)
	case "status":
		return handleServiceStatus(rest[1:], agentTarget)
	case "logs":
		return handleServiceLogs(rest[1:], agentTarget)
	case "update":
		return handleUpdate(rest[1:], agentTarget)
	case "uninstall", "remove", "delete":
		return handleUninstall(rest[1:], agentTarget)
	case "port":
		return handlePort(rest[1:], agentTarget)
	case "version", "--version", "-v":
		fmt.Printf("%s %s\n", appletDisplayName(os.Args[0]), version)
		return true
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

func handleServiceAction(action string, args []string, defaultAgent bool) bool {
	fs := flag.NewFlagSet(action, flag.ContinueOnError)
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
	if err := runSystemctl(action, target.unit); err != nil {
		cliExit(fmt.Errorf("%s failed: %w", action, err))
	}
	past := map[string]string{"start": "started", "stop": "stopped", "restart": "restarted"}[action]
	fmt.Printf("%s %s (%s).\n", target.label, past, target.unit)
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
	var agent, journal, follow bool
	var lines int
	var since, category, level, query, dbPath string
	agentFlag(fs, &agent, defaultAgent)
	fs.BoolVar(&journal, "journal", false, "read journalctl instead of the app database")
	fs.BoolVar(&follow, "follow", false, "follow new entries")
	fs.IntVar(&lines, "lines", 120, "number of entries")
	fs.StringVar(&since, "since", "", "journalctl since value")
	fs.StringVar(&category, "category", "", "log category")
	fs.StringVar(&level, "level", "", "log level")
	fs.StringVar(&query, "q", "", "search text")
	fs.StringVar(&dbPath, "db", "", "app database path")
	fs.Usage = func() { fmt.Print(managementHelp(agent)) }
	if ok, err := parseCommandFlags(fs, args); !ok {
		if errors.Is(err, flag.ErrHelp) {
			return true
		}
		return true
	}
	if lines <= 0 {
		lines = 120
	}
	if lines > 5000 {
		lines = 5000
	}
	target := serviceTargetFor(agent)
	// The agent has no application database; its journal is the dedicated log.
	if agent || journal {
		cmdArgs := []string{"-u", target.unit, "-n", strconv.Itoa(lines), "--no-pager"}
		if since != "" {
			cmdArgs = append(cmdArgs, "--since", since)
		}
		if follow {
			cmdArgs = append(cmdArgs, "-f")
		}
		cmd := exec.Command("journalctl", cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
		return true
	}

	path := strings.TrimSpace(dbPath)
	if path == "" {
		path = strings.TrimSpace(os.Getenv("PULSE_DB_PATH"))
	}
	if path == "" {
		if _, err := os.Stat("/var/lib/pulse/pulse.db"); err == nil {
			path = "/var/lib/pulse/pulse.db"
		} else {
			path = "./data/pulse.db"
		}
	}
	if _, err := os.Stat(path); err != nil {
		fmt.Fprintf(os.Stderr, "App log database not found at %s; falling back to journalctl.\n", path)
		cmd := exec.Command("journalctl", "-u", target.unit, "-n", strconv.Itoa(lines), "--no-pager")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
		return true
	}
	db, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not open %s: %v; falling back to journalctl.\n", path, err)
		cmd := exec.Command("journalctl", "-u", target.unit, "-n", strconv.Itoa(lines), "--no-pager")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
		return true
	}
	defer db.Close()

	filter := store.LogFilter{Category: category, Level: level, Query: query, Limit: lines}
	entries, err := db.ListLogs(filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not read app logs: %v\n", err)
		return true
	}
	var lastID int64
	printed := 0
	for i := len(entries) - 1; i >= 0; i-- {
		printAppLog(entries[i])
		if entries[i].ID > lastID {
			lastID = entries[i].ID
		}
		printed++
	}
	if !follow {
		if printed == 0 {
			fmt.Println("No log entries.")
		}
		return true
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-interrupt:
			return true
		case <-ticker.C:
			filter.AfterID = lastID
			filter.Limit = 200
			fresh, err := db.ListLogs(filter)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Could not read app logs: %v\n", err)
				continue
			}
			for i := len(fresh) - 1; i >= 0; i-- {
				printAppLog(fresh[i])
				if fresh[i].ID > lastID {
					lastID = fresh[i].ID
				}
			}
		}
	}
}

func printAppLog(entry model.LogEntry) {
	at := time.Unix(entry.At, 0).Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("%s  %-5s  %-10s  %s", at, strings.ToUpper(entry.Level), entry.Category, entry.Message)
	if strings.TrimSpace(entry.Detail) != "" {
		line += "  " + entry.Detail
	}
	fmt.Println(line)
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

func handleUninstall(args []string, defaultAgent bool) bool {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	var agent, yes, purge bool
	agentFlag(fs, &agent, defaultAgent)
	fs.BoolVar(&yes, "yes", false, "do not ask for confirmation")
	fs.BoolVar(&purge, "purge", false, "also remove data and configuration")
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
	if !yes && !confirmDestructive(fmt.Sprintf("Remove %s (%s)?", target.label, target.unit)) {
		fmt.Println("Aborted.")
		return true
	}
	_ = runSystemctl("stop", target.unit)
	_ = runSystemctl("disable", target.unit)
	unitPath := "/etc/systemd/system/" + target.unit + ".service"
	if err := os.Remove(unitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "remove %s: %v\n", unitPath, err)
	}
	if agent {
		_ = os.Remove("/usr/local/bin/pulse-agent")
		_ = os.Remove("/usr/local/bin/pulse-a")
		_ = os.Remove("/etc/pulse/agent.env")
		if purge && !fileExists("/etc/pulse/pulse.env") {
			_ = os.RemoveAll("/usr/local/lib/pulse")
		}
	} else {
		if purge {
			_ = os.Remove("/etc/caddy/conf.d/pulse.caddy")
			_ = os.Remove("/etc/pulse/pulse.env")
			_ = os.RemoveAll("/var/lib/pulse")
			if !fileExists("/etc/systemd/system/pulse-agent.service") {
				_ = os.RemoveAll("/usr/local/lib/pulse")
			}
			reloadCaddy()
		}
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	fmt.Printf("%s service removed. The binary at /usr/local/bin/pulse was kept.\n", target.label)
	if purge {
		fmt.Println("Data and configuration were removed (--purge).")
	} else {
		fmt.Println("Your data and configuration were kept. Use --purge to remove them.")
	}
	return true
}

func confirmDestructive(prompt string) bool {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		fmt.Fprintln(os.Stderr, "Non-interactive input; use --yes to confirm.")
		return false
	}
	fmt.Printf("%s Type 'yes' to continue: ", prompt)
	var answer string
	_, _ = fmt.Fscanln(os.Stdin, &answer)
	return strings.EqualFold(strings.TrimSpace(answer), "yes")
}

func handlePort(args []string, defaultAgent bool) bool {
	fs := flag.NewFlagSet("port", flag.ContinueOnError)
	var agent, noRestart, caddy, dryRun bool
	agentFlag(fs, &agent, defaultAgent)
	fs.BoolVar(&noRestart, "no-restart", false, "do not restart the service")
	fs.BoolVar(&caddy, "caddy", true, "update the Pulse Caddy site if present")
	fs.BoolVar(&dryRun, "dry-run", false, "print changes without writing")
	fs.Usage = func() { fmt.Print(managementHelp(agent)) }
	if ok, err := parseCommandFlags(fs, args); !ok {
		if errors.Is(err, flag.ErrHelp) {
			return true
		}
		return true
	}
	if agent {
		fmt.Println("The agent has no listening port; it connects outbound to the panel URL.")
		return true
	}
	unitPath := "/etc/systemd/system/pulse.service"
	unit, err := os.ReadFile(unitPath)
	if err != nil {
		cliExit(fmt.Errorf("cannot read %s: %w (change the port with the installer: --port)", unitPath, err))
	}
	currentAddr := unitAddr(string(unit))
	currentPort := addrPort(currentAddr)
	if len(fs.Args()) == 0 {
		fmt.Printf("Panel listen address: %s (port %s)\n", currentAddr, currentPort)
		return true
	}
	port, err := strconv.Atoi(strings.TrimSpace(fs.Args()[0]))
	if err != nil || port < 1 || port > 65535 {
		cliExit(errors.New("invalid port: use a number between 1 and 65535"))
	}
	if port < 1024 {
		fmt.Fprintln(os.Stderr, "Warning: ports below 1024 normally require root.")
	}
	if err := requireRoot(); err != nil {
		cliExit(err)
	}
	newAddr := withPort(currentAddr, port)
	newUnit := rewriteUnitAddr(string(unit), newAddr)
	caddyPath := "/etc/caddy/conf.d/pulse.caddy"
	var oldCaddy, newCaddy string
	if caddy {
		if raw, err := os.ReadFile(caddyPath); err == nil {
			oldCaddy = string(raw)
			newCaddy = rewriteCaddyPort(oldCaddy, currentPort, strconv.Itoa(port))
		}
	}
	envPath := "/etc/pulse/pulse.env"
	var oldEnv, newEnv string
	if raw, err := os.ReadFile(envPath); err == nil {
		oldEnv = string(raw)
		newEnv = rewritePublicPort(oldEnv, currentPort, strconv.Itoa(port))
	}
	if dryRun {
		fmt.Printf("Would set listen address: %s\n", newAddr)
		if oldCaddy != "" && newCaddy != oldCaddy {
			fmt.Printf("Would update %s\n", caddyPath)
		}
		return true
	}
	if err := backupAndWrite(unitPath, []byte(newUnit), 0644); err != nil {
		cliExit(err)
	}
	caddyChanged := false
	if oldCaddy != "" && newCaddy != oldCaddy {
		if err := backupAndWrite(caddyPath, []byte(newCaddy), 0644); err != nil {
			cliExit(err)
		}
		caddyChanged = true
	}
	if oldEnv != "" && newEnv != oldEnv {
		_ = backupAndWrite(envPath, []byte(newEnv), 0600)
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	if !noRestart {
		if err := runSystemctl("restart", "pulse"); err != nil {
			fmt.Fprintf(os.Stderr, "Port updated, but restart failed: %v\n", err)
		}
		if caddyChanged {
			reloadCaddy()
		}
	}
	fmt.Printf("Panel port changed from %s to %d (listen %s).\n", currentPort, port, newAddr)
	fmt.Printf("Check: journalctl -u pulse -n 40 --no-pager\n")
	return true
}

func reloadCaddy() {
	if err := exec.Command("caddy", "reload", "--config", "/etc/caddy/Caddyfile").Run(); err != nil {
		_ = exec.Command("systemctl", "reload", "caddy").Run()
	}
}

func backupAndWrite(path string, data []byte, mode os.FileMode) error {
	backup := path + ".bak-" + time.Now().Format("20060102-150405")
	if old, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(backup, old, mode)
	}
	return os.WriteFile(path, data, mode)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

var unitAddrRE = regexp.MustCompile(`--addr\s+(\S+)`)

func unitAddr(unit string) string {
	if m := unitAddrRE.FindStringSubmatch(unit); len(m) == 2 {
		return strings.Trim(m[1], `"'`)
	}
	return ":8080"
}

func addrPort(addr string) string {
	if addr == "" {
		return ""
	}
	if _, port, err := net.SplitHostPort(addr); err == nil {
		return port
	}
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i+1:]
	}
	return ""
}

func withPort(addr string, port int) string {
	host := ""
	if addr != "" {
		if h, _, err := net.SplitHostPort(addr); err == nil {
			host = h
		} else {
			host = strings.Trim(addr, "[]")
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func rewriteUnitAddr(unit, addr string) string {
	if unitAddrRE.MatchString(unit) {
		return unitAddrRE.ReplaceAllString(unit, "--addr "+addr)
	}
	re := regexp.MustCompile(`(?m)^(ExecStart=.*)$`)
	return re.ReplaceAllString(unit, `${1} --addr `+addr)
}

func rewriteCaddyPort(content, oldPort, newPort string) string {
	if oldPort == "" || oldPort == newPort {
		return content
	}
	out := strings.ReplaceAll(content, "127.0.0.1:"+oldPort, "127.0.0.1:"+newPort)
	out = strings.ReplaceAll(out, "localhost:"+oldPort, "localhost:"+newPort)
	return out
}

func rewritePublicPort(env, oldPort, newPort string) string {
	if oldPort == "" || oldPort == newPort {
		return env
	}
	lines := strings.Split(env, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "PULSE_PUBLIC_BASE_URL=") || strings.HasPrefix(line, "PULSE_ADDR=") {
			lines[i] = strings.ReplaceAll(line, ":"+oldPort, ":"+newPort)
		}
	}
	return strings.Join(lines, "\n")
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
