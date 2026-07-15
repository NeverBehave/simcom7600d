# Dev TTY bridge

Runs the modem TTY on the remote NixOS host across an SSH-forwarded TCP socket
and into a local PTY. The Go daemon then opens the local PTY exactly the same
way it would open `/dev/ttyUSB3` in production — no `if dev` branches.

## One-time prerequisites

- `socat` and `openssh` available on the remote host.
- SSH key access to the remote (`root@203.0.113.10` by default; override with
  `REMOTE=user@host`).

## Three terminals (or three tmux panes)

```
# 1. Remote-side: expose the TTY over a localhost-bound TCP listener.
make bridge-remote

# 2. Local-side: forward the remote TCP port through SSH.
make bridge-tunnel

# 3. Local-side: terminate the TCP into /tmp/sim7600 (a PTY device file).
make bridge-local

# 4. Now run the daemon:
make run
```

## Latency

40–100 ms over the SSH tunnel. Fine for AT command roundtrips. (Would be a
problem for live audio; out of scope for v1.)

## Verifying

```sh
# Health
curl -H "Authorization: Bearer $(cat sim7600d.db.dir/auth_token)" \
     http://127.0.0.1:8080/v1/status

# List recent SMS
curl -H "Authorization: Bearer ..." http://127.0.0.1:8080/v1/sms
```

## Stopping

```
make bridge-stop
```

(closes the local socat + the SSH tunnel; the remote socat exits when its TCP
connection drops).
