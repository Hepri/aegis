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
	earnUserFWPrefix  = "AegisTasksNet"
	earnLogonTaskName = "AegisEarnKiosk"
	earnLauncherName  = "AegisEarnKiosk.cmd"
)

// EnsureEarnKiosk creates/locks Задачки and ensures Edge opens /earn on logon.
func EnsureEarnKiosk(serverURL, clientID, exePath string) {
	// Drop legacy program-wide rules that blocked the Windows service itself
	// (WSAEACCES / "socket ... forbidden by its access permissions").
	removeLegacyEarnProgramFirewall()

	if err := ensureEarnUser(); err != nil {
		log.Printf("earn kiosk user: %v", err)
	}
	earnURL := buildEarnURL(serverURL, clientID)
	log.Printf("Earn kiosk URL: %s", earnURL)

	launcher, err := writeEarnLauncher(earnURL)
	if err != nil {
		log.Printf("earn launcher: %v", err)
	} else {
		log.Printf("Earn launcher: %s", launcher)
	}

	if err := ensureEarnUserLockdown(serverURL, earnURL, launcher); err != nil {
		log.Printf("earn kiosk user lockdown: %v", err)
	}
	if err := ensureEarnLogonAutostart(earnURL, launcher); err != nil {
		log.Printf("earn kiosk logon autostart: %v", err)
	}
	if err := ensureAssignedAccess(earnURL); err != nil {
		log.Printf("earn kiosk Assigned Access: %v (launcher/Shell still applied)", err)
	}
}

func buildEarnURL(serverURL, clientID string) string {
	base := strings.TrimRight(serverURL, "/")
	return base + "/earn?client_id=" + url.QueryEscape(clientID)
}

func writeEarnLauncher(earnURL string) (string, error) {
	_ = earnURL // URL is resolved by earn-kiosk from yaml
	dir := filepath.Join(os.Getenv("ProgramData"), "Aegis")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, earnLauncherName)
	exe := filepath.Join(`C:\Program Files\Aegis`, "aegis-client.exe")
	if p, err := os.Executable(); err == nil {
		exe = p
	}
	content := fmt.Sprintf("@echo off\r\n"+
		"title Aegis Earn Kiosk\r\n"+
		"\"%s\" earn-kiosk\r\n", exe)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return path, nil
}

func ensureEarnUser() error {
	check := exec.Command("net", "user", earnKioskUser)
	check.SysProcAttr = hiddenProcAttr()
	exists := check.Run() == nil

	// Temporary password so we can create a profile with runas; cleared afterward.
	tempPass := "AegisTmp1!"
	if !exists {
		add := exec.Command("net", "user", earnKioskUser, tempPass,
			"/add", "/fullname:"+earnKioskFullName, "/passwordchg:no", "/expires:never", "/y")
		add.SysProcAttr = hiddenProcAttr()
		if out, err := add.CombinedOutput(); err != nil {
			return fmt.Errorf("create user: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		log.Printf("Created local user %s", earnKioskUser)
	} else {
		set := exec.Command("net", "user", earnKioskUser, tempPass, "/passwordchg:no")
		set.SysProcAttr = hiddenProcAttr()
		_ = set.Run()
	}

	_ = exec.Command("net", "localgroup", "Users", earnKioskUser, "/add").Run()
	for _, g := range []string{"Administrators", "Remote Desktop Users", "Power Users"} {
		cmd := exec.Command("net", "localgroup", g, earnKioskUser, "/delete")
		cmd.SysProcAttr = hiddenProcAttr()
		_ = cmd.Run()
	}

	// Bootstrap profile (needs non-empty password on many Win11 builds).
	ps := fmt.Sprintf(`
$ErrorActionPreference = 'Continue'
$pass = ConvertTo-SecureString '%s' -AsPlainText -Force
$cred = New-Object System.Management.Automation.PSCredential ('%s', $pass)
try {
  Start-Process -FilePath 'cmd.exe' -ArgumentList '/c exit' -Credential $cred -LoadUserProfile -WindowStyle Hidden -Wait | Out-Null
} catch { Write-Output $_ }
`, tempPass, earnKioskUser)
	boot := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	boot.SysProcAttr = hiddenProcAttr()
	if out, err := boot.CombinedOutput(); err != nil {
		log.Printf("earn profile bootstrap: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	// Back to blank password for easy login-screen click.
	clear := exec.Command("net", "user", earnKioskUser, "", "/passwordchg:no")
	clear.SysProcAttr = hiddenProcAttr()
	if out, err := clear.CombinedOutput(); err != nil {
		log.Printf("clear %s password: %v (%s)", earnKioskUser, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func resolveServerIPv4Port(serverURL string) (remoteIPs string, port string, err error) {
	host, port, err := parseServerHostPort(serverURL)
	if err != nil {
		return "", "", err
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		if ip := net.ParseIP(host); ip != nil {
			ips = []net.IP{ip}
		} else {
			return "", "", fmt.Errorf("resolve %s: %v", host, err)
		}
	}
	var v4 []string
	for _, ip := range ips {
		if x := ip.To4(); x != nil {
			v4 = append(v4, x.String())
		}
	}
	if len(v4) == 0 {
		return "", "", fmt.Errorf("no IPv4 for %s", host)
	}
	return strings.Join(v4, ","), port, nil
}

// removeLegacyEarnProgramFirewall deletes old rules that blocked ALL outbound
// traffic from aegis-client.exe (including the Windows service). Network limits
// for Задачки stay on per-user AegisTasksNet* rules only.
func removeLegacyEarnProgramFirewall() {
	for _, name := range []string{earnFirewallAllow, earnFirewallAllow + "DNS", earnFirewallBlock} {
		if err := deleteFirewallRule(name); err == nil {
			log.Printf("Removed legacy firewall rule %s", name)
		}
	}
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

func edgeKioskArgs(earnURL string) string {
	return fmt.Sprintf("--kiosk %s --edge-kiosk-type=fullscreen --no-first-run --disable-features=TranslateUI", earnURL)
}

func ensureEarnUserLockdown(serverURL, earnURL, launcher string) error {
	remote, port, err := resolveServerIPv4Port(serverURL)
	if err != nil {
		return err
	}
	host, _, _ := parseServerHostPort(serverURL)
	if launcher == "" {
		launcher = filepath.Join(os.Getenv("ProgramData"), "Aegis", earnLauncherName)
	}
	// Shell must be a single executable path — cmd.exe running our launcher is reliable.
	shell := fmt.Sprintf(`%s /c "%s"`, filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"), launcher)

	ps := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$user = '%s'
$remote = '%s'
$port = '%s'
$hostName = '%s'
$earnUrl = '%s'
$shell = '%s'
$fwPrefix = '%s'

$sid = (New-Object System.Security.Principal.NTAccount($user)).Translate([System.Security.Principal.SecurityIdentifier]).Value
$localUser = 'O:LSD:(A;;CC;;;' + $sid + ')'

Get-NetFirewallRule -ErrorAction SilentlyContinue | Where-Object { $_.DisplayName -like ($fwPrefix + '*') } | Remove-NetFirewallRule -ErrorAction SilentlyContinue
try {
  New-NetFirewallRule -DisplayName ($fwPrefix + 'AllowAegis') -Direction Outbound -Action Allow -Protocol TCP -RemoteAddress ($remote -split ',') -RemotePort $port -LocalUser $localUser -Profile Any | Out-Null
  New-NetFirewallRule -DisplayName ($fwPrefix + 'AllowDNS') -Direction Outbound -Action Allow -Protocol UDP -RemotePort 53 -LocalUser $localUser -Profile Any | Out-Null
  New-NetFirewallRule -DisplayName ($fwPrefix + 'AllowLoopback') -Direction Outbound -Action Allow -RemoteAddress @('127.0.0.1','::1') -LocalUser $localUser -Profile Any | Out-Null
  New-NetFirewallRule -DisplayName ($fwPrefix + 'BlockRest') -Direction Outbound -Action Block -LocalUser $localUser -Profile Any | Out-Null
} catch { Write-Output ("fw_err=" + $_) }

$profile = $null
Get-CimInstance Win32_UserProfile | ForEach-Object {
  try {
    $acc = (New-Object System.Security.Principal.SecurityIdentifier($_.SID)).Translate([System.Security.Principal.NTAccount]).Value
    if ($acc -match [regex]::Escape($user) + '$') { $profile = $_.LocalPath }
  } catch {}
}
if (-not $profile) { throw "profile for $user not found — log in once as Задачки then restart Aegis service" }
$ntuser = Join-Path $profile 'NTUSER.DAT'
if (-not (Test-Path $ntuser)) { throw "missing $ntuser" }

function Set-EarnPolicies($root) {
  $winlogon = Join-Path $root 'Software\Microsoft\Windows NT\CurrentVersion\Winlogon'
  New-Item -Path $winlogon -Force | Out-Null
  New-ItemProperty -Path $winlogon -Name Shell -PropertyType String -Value $shell -Force | Out-Null

  $sysPol = Join-Path $root 'Software\Microsoft\Windows\CurrentVersion\Policies\System'
  New-Item -Path $sysPol -Force | Out-Null
  foreach ($n in @('DisableTaskMgr','DisableRegistryTools','DisableChangePassword','DisableLockWorkstation')) {
    New-ItemProperty -Path $sysPol -Name $n -PropertyType DWord -Value 1 -Force | Out-Null
  }

  $expPol = Join-Path $root 'Software\Microsoft\Windows\CurrentVersion\Policies\Explorer'
  New-Item -Path $expPol -Force | Out-Null
  foreach ($n in @('NoRun','NoControlPanel','NoClose','NoViewContextMenu','NoTrayContextMenu','NoSetTaskbar')) {
    New-ItemProperty -Path $expPol -Name $n -PropertyType DWord -Value 1 -Force | Out-Null
  }

  $cmdPol = Join-Path $root 'Software\Policies\Microsoft\Windows\System'
  New-Item -Path $cmdPol -Force | Out-Null
  # Do NOT DisableCMD — Shell is cmd.exe running the Edge launcher.
  Remove-ItemProperty -Path $cmdPol -Name DisableCMD -ErrorAction SilentlyContinue

  # Edge: block all URLs except Aegis; force dead proxy with bypass for Aegis host/IP.
  $edge = Join-Path $root 'Software\Policies\Microsoft\Edge'
  New-Item -Path $edge -Force | Out-Null
  New-ItemProperty -Path $edge -Name ProxyMode -PropertyType String -Value 'fixed_servers' -Force | Out-Null
  New-ItemProperty -Path $edge -Name ProxyServer -PropertyType String -Value 'http://127.0.0.1:9' -Force | Out-Null
  $bypass = ($remote -split ',' ) + @($hostName, 'localhost', '127.0.0.1') -join ';'
  New-ItemProperty -Path $edge -Name ProxyBypassList -PropertyType String -Value $bypass -Force | Out-Null

  $block = Join-Path $edge 'URLBlocklist'
  New-Item -Path $block -Force | Out-Null
  New-ItemProperty -Path $block -Name '1' -PropertyType String -Value '*' -Force | Out-Null

  $allow = Join-Path $edge 'URLAllowlist'
  New-Item -Path $allow -Force | Out-Null
  $i = 1
  foreach ($p in @($earnUrl, ("http://" + $hostName + ":*"), ("http://" + $hostName + ":*/*"), 'http://127.0.0.1:*', 'http://127.0.0.1:*/*', 'http://localhost:*', 'http://localhost:*/*')) {
    New-ItemProperty -Path $allow -Name ([string]$i) -PropertyType String -Value $p -Force | Out-Null
    $i++
  }
  foreach ($ip in ($remote -split ',')) {
    New-ItemProperty -Path $allow -Name ([string]$i) -PropertyType String -Value ("http://" + $ip + ":*") -Force | Out-Null; $i++
    New-ItemProperty -Path $allow -Name ([string]$i) -PropertyType String -Value ("http://" + $ip + ":*/*") -Force | Out-Null; $i++
  }
}

# If user is logged in, write live hive; always also write NTUSER.DAT for next logon.
$live = 'Registry::HKU\' + $sid
if (Test-Path $live) {
  Set-EarnPolicies $live
  Write-Output 'live_hive=1'
}

$hive = 'HKU\AegisEarnTemp'
reg unload $hive 2>$null | Out-Null
$load = reg load $hive $ntuser 2>&1
if ($LASTEXITCODE -ne 0) { throw "reg load failed: $load" }
try {
  Set-EarnPolicies 'Registry::HKU\AegisEarnTemp'
  Write-Output ("sid=" + $sid + " shell=" + $shell + " net=" + $remote + ":" + $port)
} finally {
  [gc]::Collect()
  Start-Sleep -Milliseconds 400
  reg unload $hive | Out-Null
}
`, earnKioskUser, remote, port, host,
		strings.ReplaceAll(earnURL, `'`, `''`),
		strings.ReplaceAll(shell, `'`, `''`),
		earnUserFWPrefix)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = hiddenProcAttr()
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		return fmt.Errorf("%w (%s)", err, msg)
	}
	log.Printf("Earn user lockdown: %s", msg)
	return nil
}

func ensureEarnLogonAutostart(earnURL, launcher string) error {
	if launcher == "" {
		launcher = filepath.Join(os.Getenv("ProgramData"), "Aegis", earnLauncherName)
	}
	launcherEsc := strings.ReplaceAll(launcher, `'`, `''`)
	ps := fmt.Sprintf(`
$ErrorActionPreference = 'Continue'
$user = '%s'
$launcher = '%s'
$profile = $null
Get-CimInstance Win32_UserProfile | ForEach-Object {
  try {
    $acc = (New-Object System.Security.Principal.SecurityIdentifier($_.SID)).Translate([System.Security.Principal.NTAccount]).Value
    if ($acc -match [regex]::Escape($user) + '$') { $profile = $_.LocalPath }
  } catch {}
}
if (-not $profile) { $profile = Join-Path $env:SystemDrive ('Users\' + $user) }
$startup = Join-Path $profile 'AppData\Roaming\Microsoft\Windows\Start Menu\Programs\Startup'
New-Item -ItemType Directory -Force -Path $startup | Out-Null
$cmdPath = Join-Path $startup 'AegisEarnKiosk.cmd'
Copy-Item -Force $launcher $cmdPath
Write-Output ("startup=" + $cmdPath)
`, earnKioskUser, launcherEsc)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = hiddenProcAttr()
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		return fmt.Errorf("%w (%s)", err, msg)
	}
	log.Printf("Earn logon autostart: %s", msg)

	tr := fmt.Sprintf(`"%s"`, launcher)
	_ = exec.Command("schtasks", "/Delete", "/TN", earnLogonTaskName, "/F").Run()
	task := exec.Command("schtasks", "/Create", "/TN", earnLogonTaskName,
		"/TR", tr, "/SC", "ONLOGON", "/RU", earnKioskUser, "/RP", "",
		"/RL", "LIMITED", "/F", "/IT")
	task.SysProcAttr = hiddenProcAttr()
	if out, err := task.CombinedOutput(); err != nil {
		log.Printf("earn logon task: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func ensureAssignedAccess(earnURL string) error {
	edge := edgePath()
	edgeXML := strings.ReplaceAll(edge, `&`, `&amp;`)
	edgeXML = strings.ReplaceAll(edgeXML, `'`, `''`)
	args := edgeKioskArgs(earnURL)
	argsXML := strings.ReplaceAll(args, `&`, `&amp;`)
	argsXML = strings.ReplaceAll(argsXML, `'`, `''`)
	// Also try launching our cmd launcher as classic app (more reliable args).
	launcher := filepath.Join(os.Getenv("ProgramData"), "Aegis", earnLauncherName)
	launcherXML := strings.ReplaceAll(launcher, `&`, `&amp;`)
	launcherXML = strings.ReplaceAll(launcherXML, `'`, `''`)
	ps := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$xml = @"
<?xml version="1.0" encoding="utf-8"?>
<AssignedAccessConfiguration xmlns="http://schemas.microsoft.com/AssignedAccess/2017/config"
  xmlns:v4="http://schemas.microsoft.com/AssignedAccess/2021/config">
  <Profiles>
    <Profile Id="{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}">
      <KioskModeApp v4:ClassicAppPath="%s" v4:ClassicAppArguments="/c \"%s\"" />
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
if (-not $obj) { throw 'MDM_AssignedAccess not available' }
$obj.Configuration = [System.Net.WebUtility]::HtmlEncode($xml)
Set-CimInstance -CimInstance $obj
Write-Output 'ok'
`, filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"), launcherXML, earnKioskUser)

	_ = edgeXML
	_ = argsXML
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = hiddenProcAttr()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (%s)", err, strings.TrimSpace(string(out)))
	}
	log.Printf("Assigned Access: cmd launcher → %s", earnURL)
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
	return `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`
}
