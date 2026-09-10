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
	earnFirewallAllow = "AegisEarnAllow" // aegis-client.exe service allowlist
	earnFirewallBlock = "AegisEarnBlock"
	earnUserFWPrefix  = "AegisTasksNet" // per-user firewall (only AegisTasks SID)
	earnLogonTaskName = "AegisEarnKiosk"
)

// EnsureEarnKiosk creates the Задачки account, locks it down (AA + Shell + policies +
// per-user firewall only to Aegis), and points Edge kiosk at /earn.
func EnsureEarnKiosk(serverURL, clientID, exePath string) {
	if err := ensureEarnUser(); err != nil {
		log.Printf("earn kiosk user: %v", err)
	}
	if err := ensureEarnFirewall(serverURL, exePath); err != nil {
		log.Printf("earn kiosk service firewall: %v", err)
	}
	earnURL := buildEarnURL(serverURL, clientID)
	log.Printf("Earn kiosk URL: %s", earnURL)
	if err := ensureEarnUserLockdown(serverURL, earnURL); err != nil {
		log.Printf("earn kiosk user lockdown: %v", err)
	}
	if err := ensureEarnLogonAutostart(earnURL); err != nil {
		log.Printf("earn kiosk logon autostart: %v", err)
	}
	if err := ensureAssignedAccess(earnURL); err != nil {
		log.Printf("earn kiosk Assigned Access: %v (Shell/Startup/firewall still applied)", err)
	}
}

func buildEarnURL(serverURL, clientID string) string {
	base := strings.TrimRight(serverURL, "/")
	return base + "/earn?client_id=" + url.QueryEscape(clientID)
}

func ensureEarnUser() error {
	check := exec.Command("net", "user", earnKioskUser)
	check.SysProcAttr = hiddenProcAttr()
	if err := check.Run(); err != nil {
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
	for _, g := range []string{"Administrators", "Remote Desktop Users", "Power Users"} {
		cmd := exec.Command("net", "localgroup", g, earnKioskUser, "/delete")
		cmd.SysProcAttr = hiddenProcAttr()
		_ = cmd.Run()
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

func ensureEarnFirewall(serverURL, exePath string) error {
	remote, port, err := resolveServerIPv4Port(serverURL)
	if err != nil {
		return err
	}
	exePath, _ = filepath.Abs(exePath)

	_ = deleteFirewallRule(earnFirewallAllow)
	_ = deleteFirewallRule(earnFirewallAllow + "DNS")
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

	allowDNS := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+earnFirewallAllow+"DNS",
		"dir=out", "action=allow",
		"program="+exePath,
		"protocol=UDP",
		"remoteport=53",
		"enable=yes",
	)
	allowDNS.SysProcAttr = hiddenProcAttr()
	_ = allowDNS.Run()

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
	log.Printf("Earn service firewall: %s → %s:%s", exePath, remote, port)
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

func edgeKioskArgs(earnURL string) string {
	return fmt.Sprintf("--kiosk %s --edge-kiosk-type=fullscreen --no-first-run --disable-features=TranslateUI", earnURL)
}

// ensureEarnUserLockdown: per-user firewall (only AegisTasks → Aegis), Shell=Edge kiosk,
// and HKCU policies that strip Task Manager / Run / CMD / Control Panel.
func ensureEarnUserLockdown(serverURL, earnURL string) error {
	remote, port, err := resolveServerIPv4Port(serverURL)
	if err != nil {
		return err
	}
	edge := edgePath()
	shell := fmt.Sprintf(`%s %s`, edge, edgeKioskArgs(earnURL))

	ps := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$user = '%s'
$remote = '%s'
$port = '%s'
$shell = '%s'
$fwPrefix = '%s'

# Resolve SID
$sid = (New-Object System.Security.Principal.NTAccount($user)).Translate([System.Security.Principal.SecurityIdentifier]).Value
$localUser = 'O:LSD:(A;;CC;;;' + $sid + ')'

# --- Per-user firewall: only this SID, not other accounts ---
Get-NetFirewallRule -ErrorAction SilentlyContinue | Where-Object { $_.DisplayName -like ($fwPrefix + '*') } | Remove-NetFirewallRule -ErrorAction SilentlyContinue

New-NetFirewallRule -DisplayName ($fwPrefix + 'AllowAegis') -Direction Outbound -Action Allow -Protocol TCP -RemoteAddress ($remote -split ',') -RemotePort $port -LocalUser $localUser -Profile Any | Out-Null
New-NetFirewallRule -DisplayName ($fwPrefix + 'AllowDNS') -Direction Outbound -Action Allow -Protocol UDP -RemotePort 53 -LocalUser $localUser -Profile Any | Out-Null
New-NetFirewallRule -DisplayName ($fwPrefix + 'AllowLoopback') -Direction Outbound -Action Allow -RemoteAddress @('127.0.0.1','::1') -LocalUser $localUser -Profile Any | Out-Null
New-NetFirewallRule -DisplayName ($fwPrefix + 'BlockRest') -Direction Outbound -Action Block -LocalUser $localUser -Profile Any | Out-Null

# --- Profile + Shell + lockdown policies ---
$pass = New-Object System.Security.SecureString
$cred = New-Object System.Management.Automation.PSCredential ($user, $pass)
try {
  Start-Process -FilePath 'cmd.exe' -ArgumentList '/c exit' -Credential $cred -LoadUserProfile -WindowStyle Hidden -Wait | Out-Null
} catch {}

$profile = $null
Get-CimInstance Win32_UserProfile | ForEach-Object {
  try {
    $acc = (New-Object System.Security.Principal.SecurityIdentifier($_.SID)).Translate([System.Security.Principal.NTAccount]).Value
    if ($acc -match [regex]::Escape($user) + '$') { $profile = $_.LocalPath }
  } catch {}
}
if (-not $profile) { throw "profile for $user not found" }
$ntuser = Join-Path $profile 'NTUSER.DAT'
if (-not (Test-Path $ntuser)) { throw "missing $ntuser" }

$hive = 'HKU\AegisEarnTemp'
reg unload $hive 2>$null | Out-Null
$load = reg load $hive $ntuser 2>&1
if ($LASTEXITCODE -ne 0) { throw "reg load failed: $load" }
try {
  $root = 'Registry::HKU\AegisEarnTemp'
  $winlogon = Join-Path $root 'Software\Microsoft\Windows NT\CurrentVersion\Winlogon'
  New-Item -Path $winlogon -Force | Out-Null
  New-ItemProperty -Path $winlogon -Name Shell -PropertyType String -Value $shell -Force | Out-Null

  $sysPol = Join-Path $root 'Software\Microsoft\Windows\CurrentVersion\Policies\System'
  New-Item -Path $sysPol -Force | Out-Null
  New-ItemProperty -Path $sysPol -Name DisableTaskMgr -PropertyType DWord -Value 1 -Force | Out-Null
  New-ItemProperty -Path $sysPol -Name DisableRegistryTools -PropertyType DWord -Value 1 -Force | Out-Null
  New-ItemProperty -Path $sysPol -Name DisableChangePassword -PropertyType DWord -Value 1 -Force | Out-Null
  New-ItemProperty -Path $sysPol -Name DisableLockWorkstation -PropertyType DWord -Value 1 -Force | Out-Null

  $expPol = Join-Path $root 'Software\Microsoft\Windows\CurrentVersion\Policies\Explorer'
  New-Item -Path $expPol -Force | Out-Null
  New-ItemProperty -Path $expPol -Name NoRun -PropertyType DWord -Value 1 -Force | Out-Null
  New-ItemProperty -Path $expPol -Name NoControlPanel -PropertyType DWord -Value 1 -Force | Out-Null
  New-ItemProperty -Path $expPol -Name NoClose -PropertyType DWord -Value 1 -Force | Out-Null
  New-ItemProperty -Path $expPol -Name NoLogoff -PropertyType DWord -Value 0 -Force | Out-Null
  New-ItemProperty -Path $expPol -Name NoViewContextMenu -PropertyType DWord -Value 1 -Force | Out-Null
  New-ItemProperty -Path $expPol -Name NoTrayContextMenu -PropertyType DWord -Value 1 -Force | Out-Null
  New-ItemProperty -Path $expPol -Name NoSetTaskbar -PropertyType DWord -Value 1 -Force | Out-Null

  $cmdPol = Join-Path $root 'Software\Policies\Microsoft\Windows\System'
  New-Item -Path $cmdPol -Force | Out-Null
  New-ItemProperty -Path $cmdPol -Name DisableCMD -PropertyType DWord -Value 1 -Force | Out-Null

  Write-Output ("sid=" + $sid + " shell=" + $shell + " net=" + $remote + ":" + $port)
} finally {
  [gc]::Collect()
  Start-Sleep -Milliseconds 300
  reg unload $hive | Out-Null
}
`, earnKioskUser, remote, port, strings.ReplaceAll(shell, `'`, `''`), earnUserFWPrefix)

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

func ensureEarnLogonAutostart(earnURL string) error {
	edge := edgePath()
	edgeEsc := strings.ReplaceAll(edge, `'`, `''`)
	urlEsc := strings.ReplaceAll(earnURL, `'`, `''`)
	ps := fmt.Sprintf(`
$ErrorActionPreference = 'Continue'
$user = '%s'
$edge = '%s'
$url = '%s'
$pass = New-Object System.Security.SecureString
$cred = New-Object System.Management.Automation.PSCredential ($user, $pass)
try {
  Start-Process -FilePath 'cmd.exe' -ArgumentList '/c exit' -Credential $cred -LoadUserProfile -WindowStyle Hidden -Wait | Out-Null
} catch {}
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
$lines = @(
  '@echo off',
  ('start "" "' + $edge + '" --kiosk "' + $url + '" --edge-kiosk-type=fullscreen --no-first-run')
)
Set-Content -Path $cmdPath -Encoding ASCII -Value ($lines -join [Environment]::NewLine)
Write-Output ("startup=" + $cmdPath)
`, earnKioskUser, edgeEsc, urlEsc)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = hiddenProcAttr()
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		return fmt.Errorf("%w (%s)", err, msg)
	}
	log.Printf("Earn logon autostart: %s", msg)

	tr := fmt.Sprintf(`%s %s`, edge, edgeKioskArgs(earnURL))
	_ = exec.Command("schtasks", "/Delete", "/TN", earnLogonTaskName, "/F").Run()
	task := exec.Command("schtasks", "/Create", "/TN", earnLogonTaskName,
		"/TR", tr, "/SC", "ONLOGON", "/RU", earnKioskUser, "/RP", "",
		"/RL", "LIMITED", "/F", "/IT",
	)
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
	ps := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$xml = @"
<?xml version="1.0" encoding="utf-8"?>
<AssignedAccessConfiguration xmlns="http://schemas.microsoft.com/AssignedAccess/2017/config"
  xmlns:v4="http://schemas.microsoft.com/AssignedAccess/2021/config">
  <Profiles>
    <Profile Id="{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}">
      <KioskModeApp v4:ClassicAppPath="%s" v4:ClassicAppArguments="%s" />
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
`, edgeXML, argsXML, earnKioskUser)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = hiddenProcAttr()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (%s)", err, strings.TrimSpace(string(out)))
	}
	log.Printf("Assigned Access: Edge kiosk → %s", earnURL)
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
