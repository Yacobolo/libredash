# CLI overview

The `leapview` CLI runs the local server, validates a dashboard-as-code project, and deploys it to a LeapView instance.

## Common workflow

```sh
leapview validate --project dashboards/leapview.yaml
leapview plan --project dashboards/leapview.yaml
leapview deploy --project dashboards/leapview.yaml --target https://dash.example.com
```

Use the generated command reference when you need every flag and subcommand.
