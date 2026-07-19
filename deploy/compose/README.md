# LibreDash Docker Compose

This is the production operations package for the public LibreDash image. It
runs exactly one application process with one named state volume and one
configured environment, and adds hardened defaults, HTTPS, backups, and paired
image-and-state rollback. The included `libredashctl` is a standalone Go
operations binary for the archive's operating system and architecture.

```sh
cp deployment.env.example deployment.env
./libredashctl init --admin-email admin@example.com --domain dash.example.com
./libredashctl start
./libredashctl first-login
```

Set the released `LIBREDASH_IMAGE` digest before initialization. HTTPS is
enabled by default through the Caddy overlay. Use `--no-https` only when a
trusted external HTTPS proxy fronts the localhost-bound application port.

Pulling and running the public image does not require this package or the
controller; see the installation guide for the localhost evaluation path. For
production, `libredashctl` provides the supported initialization, backup,
restore, upgrade, and rollback workflow. Run `./libredashctl help` for its
commands.
