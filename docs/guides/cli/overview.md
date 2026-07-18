# CLI overview

The `libredash` CLI runs the local server, validates a dashboard-as-code project, and deploys it to a LibreDash instance.

## Common workflow

```sh
libredash validate --project dashboards/libredash.yaml
libredash plan --project dashboards/libredash.yaml
libredash deploy --project dashboards/libredash.yaml --target https://dash.example.com
```

Use the generated command reference when you need every flag and subcommand.
