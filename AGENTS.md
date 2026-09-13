## Learned User Preferences

- Often gives short Russian ops commands (`заливай`, `запушь`); when asked to deploy, run `./deploy/deploy.sh` to the aegis server, and push to git only when explicitly requested.
- Wants session login timestamps from real Windows logon time (`WTSLogonTime`), not when the client first notices the session on its poll.
- Wants the client binary version sent on long-poll and shown in the admin UI next to online status.
- Prefers remote OTA client updates so each change does not require physical access to the Windows PC (first install can still be manual); may ask to ship server-only without client OTA.
- In the admin activity UI: activity block at the bottom; group by user with that user’s sessions nested underneath; active sessions above completed with start/end/duration and a clear live highlight; locked/lock-screen sessions are not “active”; apps nested under sessions with per-app totals only (not a focus-event timeline); daily focus-time summary per managed user (day total + per app, focus only, no double-counting; exclude admin); minimal expandable session blocks.
- Wants `session-agent` fully hidden (no console window); lock must auto-enforce without any user-closable dialog.
- Wants fail-closed boot: after reboot or service start, managed accounts stay locked until the client successfully fetches schedule config — no open access when the PC is offline.
- Wants earn-time: child solves server-hosted tasks on the same PC via a separate restricted Windows account, accumulates bonus minutes, then redeems them with a “buy time” action.

## Learned Workspace Facts

- Aegis is parental control for Windows account schedules (lock via password + logoff), with a Russian admin UI — not employee monitoring or general telemetry.
- Stack: Go `aegis-server` (JSON file store + embedded admin UI) and Windows `aegis-client` service; config is pushed over HTTP long-poll `GET /api/config` (no WebSocket).
- Activity pipeline: client watches WTS sessions and runs a per-session `session-agent` for open windows/focus; events queue locally then batch to the server as JSONL under `activity/{client_id}/{date}.jsonl`.
- App/focus events only appear when `session-agent` is running inside the interactive user session; service-only WTS polling covers login/logout/lock/unlock.
- Activity “lock” time is Windows lock-screen duration, not Aegis schedule enforcement.
- OTA: `deploy/deploy.sh` (`redeploy` / `client-only`) publishes `updates/aegis-client.exe` + `client.json`; clients apply via the `update` field on `/api/config` (SHA256 check, replace binary, restart service).
- Deploy defaults live in `deploy/` (`DEPLOY_IP`, `DEPLOY_USER`, `DEPLOY_PATH=/opt/aegis`); systemd restarts need passwordless sudo (`deploy/sudoers.aegis`) or a manual `systemctl start`.
- Windows client installs to `C:\Program Files\Aegis\`; build/version for OTA uses `-ldflags "-X main.Version=..."`.
- `session-agent` is launched hidden (`CREATE_NO_WINDOW` / `SW_HIDE` / `FreeConsole`). Fail-closed on service start: managed usernames persist under `%ProgramData%\Aegis\`, random password + logoff apply before network, unlock only after a successful config poll in an allowed window.
- Same `aegis-client.exe` runs as the Windows service and as per-session `session-agent`; after service restart, orphaned agents are cleaned up on start/stop so Task Manager is not left with extra copies.
- Earn-time: server stores `earn_tasks` and a per-user minute wallet; child UI at `/earn`; redeem grants `temporary_access` like admin “add time”.
- Earn kiosk account `AegisTasks` (display «Задачки», no password): Assigned Access / Shell launches earn-kiosk; outbound firewall is **per-user** (`AegisTasksNet*`) only — never program-wide on `aegis-client.exe` (that blocked the Windows service with WSAEACCES). Child-account internet unchanged.
