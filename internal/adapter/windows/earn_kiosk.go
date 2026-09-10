//go:build windows

package windows

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	earnKioskUser     = "AegisTasks"
	earnKioskFullName = "Задачки"
	earnFirewallAllow = "AegisEarnAllow"
	earnFirewallBlock = "AegisEarnBlock"
)

// EnsureEarnKiosk creates/repairs the restricted Tasks account, firewall allowlist
// for aegis-client.exe → server, and Assigned Access kiosk shell.
func EnsureEarnKiosk(serverURL, exePath string) {
	if err := ensureEarnUser(); err != nil {
		log.Printf("earn kiosk user: %v", err)
	}
	if err := ensureEarnFirewall(serverURL, exePath); err != nil {
		log.Printf("earn kiosk firewall: %v", err)
	}
	if err := ensureAssignedAccess(exePath); err != nil {
		log.Printf("earn kiosk Assigned Access: %v", err)
	}
}

func ensureEarnUser() error {
	check := exec.Command("net", "user", earnKioskUser)
	check.SysProcAttr = hiddenProcAttr()
	if err := check.Run(); err != nil {
		// Empty password: child picks «Задачки» on the login screen with no PIN/password.
		// Windows still allows blank passwords for interactive console logon by default.
		add := exec.Command("net", "user", earnKioskUser, "",
			"/add", "/fullname:"+earnKioskFullName, "/passwordchg:no", "/expires:never", "/y")
		add.SysProcAttr = hiddenProcAttr()
		if out, err := add.CombinedOutput(); err != nil {
			return fmt.Errorf("create user: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		log.Printf("Created local user %s (no password)", earnKioskUser)
	} else {
		set := exec.Command("net", "user", earnKioskUser, "", "/passwordchg:no")
		set.SysProcAttr = hiddenProcAttr()
		if out, err := set.CombinedOutput(); err != nil {
			log.Printf("clear %s password: %v (%s)", earnKioskUser, err, strings.TrimSpace(string(out)))
		}
	}
	_ = exec.Command("net", "localgroup", "Users", earnKioskUser, "/add").Run()
	delAdmin := exec.Command("net", "localgroup", "Administrators", earnKioskUser, "/delete")
	delAdmin.SysProcAttr = hiddenProcAttr()
	_ = delAdmin.Run()
	return nil
}

func ensureEarnFirewall(serverURL, exePath string) error {
	host, port, err := parseServerHostPort(serverURL)
	if err != nil {
		return err
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		// Fall back to literal host (may already be an IP)
		if ip := net.ParseIP(host); ip != nil {
			ips = []net.IP{ip}
		} else {
			return fmt.Errorf("resolve %s: %v", host, err)
		}
	}
	remoteIPs := make([]string, 0, len(ips))
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			remoteIPs = append(remoteIPs, v4.String())
		}
	}
	if len(remoteIPs) == 0 {
		return fmt.Errorf("no IPv4 for %s", host)
	}
	remote := strings.Join(remoteIPs, ",")
	exePath, _ = filepath.Abs(exePath)

	_ = deleteFirewallRule(earnFirewallAllow)
	_ = deleteFirewallRule(earnFirewallBlock)

	allow := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+earnFirewallAllow,
		"dir=out", "action=allow",
		"program="+exePath,
		"remoteip="+remote,
		"protocol=TCP",
		"remoteport="+port,
		"enable=yes",
	)
	allow.SysProcAttr = hiddenProcAttr()
	if out, err := allow.CombinedOutput(); err != nil {
		return fmt.Errorf("allow rule: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// DNS must remain available so server_url hostnames still resolve.
	allowDNS := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+earnFirewallAllow+"DNS",
		"dir=out", "action=allow",
		"program="+exePath,
		"protocol=UDP",
		"remoteport=53",
		"enable=yes",
	)
	allowDNS.SysProcAttr = hiddenProcAttr()
	_ = deleteFirewallRule(earnFirewallAllow + "DNS")
	if out, err := allowDNS.CombinedOutput(); err != nil {
		log.Printf("earn DNS allow rule: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	block := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+earnFirewallBlock,
		"dir=out", "action=block",
		"program="+exePath,
		"enable=yes",
	)
	block.SysProcAttr = hiddenProcAttr()
	if out, err := block.CombinedOutput(); err != nil {
		return fmt.Errorf("block rule: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	log.Printf("Earn firewall: %s → %s:%s", exePath, remote, port)
	return nil
}

func deleteFirewallRule(name string) error {
	cmd := exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+name)
	cmd.SysProcAttr = hiddenProcAttr()
	return cmd.Run()
}

func parseServerHostPort(serverURL string) (host, port string, err error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "", "", err
	}
	host = u.Hostname()
	port = u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	if host == "" {
		return "", "", fmt.Errorf("empty host in %s", serverURL)
	}
	return host, port, nil
}

func ensureAssignedAccess(exePath string) error {
	exePath, _ = filepath.Abs(exePath)
	exePath = strings.ReplaceAll(exePath, `'`, `''`)
	edge := strings.ReplaceAll(edgePath(), `'`, `''`)
	ps := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$xml = @"
<?xml version="1.0" encoding="utf-8"?>
<AssignedAccessConfiguration xmlns="http://schemas.microsoft.com/AssignedAccess/2017/config"
  xmlns:rs5="http://schemas.microsoft.com/AssignedAccess/201810/config"
  xmlns:v4="http://schemas.microsoft.com/AssignedAccess/2021/config">
  <Profiles>
    <Profile Id="{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}">
      <AllAppsList>
        <AllowedApps>
          <App DesktopAppPath="%s" rs5:AutoLaunch="true" rs5:AutoLaunchArguments="earn-kiosk" />
          <App DesktopAppPath="%s" />
        </AllowedApps>
      </AllAppsList>
      <Taskbar rs5:ShowTaskbar="false"/>
    </Profile>
  </Profiles>
  <Configs>
    <Config>
      <Account>%s</Account>
      <DefaultProfile Id="{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}"/>
    </Config>
  </Configs>
</AssignedAccessConfiguration>
"@
$namespaceName = 'root\cimv2\mdm\dmmap'
$className = 'MDM_AssignedAccess'
$obj = Get-CimInstance -Namespace $namespaceName -ClassName $className -ErrorAction SilentlyContinue
if (-not $obj) { throw 'MDM_AssignedAccess not available (Windows edition may lack Assigned Access)' }
$obj.Configuration = [System.Net.WebUtility]::HtmlEncode($xml)
Set-CimInstance -CimInstance $obj
`, exePath, edge, earnKioskUser)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = hiddenProcAttr()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (%s)", err, strings.TrimSpace(string(out)))
	}
	log.Printf("Assigned Access configured for %s", earnKioskUser)
	return nil
}

func hiddenProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
}

func edgePath() string {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(os.Getenv("ProgramFiles"), `Microsoft\Edge\Application\msedge.exe`),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return `msedge.exe`
}
