# LeapView Docker Compose

This is the production operations package for the public LeapView image. It
runs exactly one application process with one named state volume and one
configured environment, and adds hardened defaults, HTTPS, backups, and paired
image-and-state rollback. The included `leapviewctl` is a standalone Go
operations binary for the archive's operating system and architecture.

```sh
cp deployment.env.example deployment.env
./leapviewctl init --admin-email admin@example.com --domain dash.example.com
./leapviewctl start
./leapviewctl first-login
```

Set the released `LEAPVIEW_IMAGE` digest before initialization. HTTPS is
enabled by default through the Caddy overlay. Use `--no-https` only when a
trusted external HTTPS proxy fronts the localhost-bound application port.

Pulling and running the public image does not require this package or the
controller; see the installation guide for the localhost evaluation path. For
production, `leapviewctl` provides the supported initialization, backup,
restore, upgrade, and rollback workflow. Run `./leapviewctl help` for its
commands.
