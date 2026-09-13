@echo off
:: One-shot unblock: remove legacy AegisEarn* rules that blocked the Aegis service.
net session >nul 2>&1
if errorlevel 1 (
  powershell -NoProfile -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
  exit /b
)

echo Removing legacy Aegis program firewall rules...
netsh advfirewall firewall delete rule name=AegisEarnAllow >nul 2>&1
netsh advfirewall firewall delete rule name=AegisEarnAllowDNS >nul 2>&1
netsh advfirewall firewall delete rule name=AegisEarnBlock >nul 2>&1

echo Restarting AegisClient service...
net stop AegisClient >nul 2>&1
net start AegisClient
if errorlevel 1 (
  echo Failed to start service. Check services.msc
  pause
  exit /b 1
)

echo OK. Service should reach the Aegis server again.
echo Check C:\Program Files\Aegis\aegis-client.log for "Config updated" / "UNLOCKED".
timeout /t 5
