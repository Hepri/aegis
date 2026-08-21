## Learned User Preferences

- Often gives short Russian ops commands (`заливай`, `запушь`); when asked to deploy, run `./deploy/deploy.sh` to the aegis server, and push to git only when explicitly requested.
- Wants session login timestamps from real Windows logon time (`WTSLogonTime`), not when the client first notices the session on its poll.
- Wants the client binary version sent on long-poll and shown in the admin UI next to online status.
- Prefers remote OTA client updates so each change does not require physical access to the Windows PC (first install can still be manual).

## Learned Workspace Facts

- Aegis is parental control for Windows account schedules (lock via password + logoff), with a Russian admin UI — not employee monitoring or general telemetry.
- Stack: Go `aegis-server` (JSON file store + embedded admin UI) and Windows `aegis-client` service; config is pushed over HTTP long-poll `GET /api/config` (no WebSocket).
- Activity pipeline: client watches WTS sessions and runs a per-session `session-agent` for open windows/focus; events queue locally then batch to the server as JSONL under `activity/{client_id}/{date}.jsonl`.
- App/focus events only appear when `session-agent` is running inside the interactive user session; service-only WTS polling covers login/logout/lock/unlock.
- OTA: `deploy/deploy.sh` (`redeploy` / `client-only`) publishes `updates/aegis-client.exe` + `client.json`; clients apply via the `update` field on `/api/config` (SHA256 check, replace binary, restart service).
- Deploy defaults live in `deploy/` (`DEPLOY_IP`, `DEPLOY_USER`, `DEPLOY_PATH=/opt/aegis`); systemd restarts need passwordless sudo (`deploy/sudoers.aegis`) or a manual `systemctl start`.
- Windows client installs to `C:\Program Files\Aegis\`; build/version for OTA uses `-ldflags "-X main.Version=..."`.
