# Targets and environments

Use an explicit target and environment for production-like plans and deployments. Keep local development, staging, and production targets separate.

```sh
leapview plan --project dashboards/leapview.yaml \
  --target https://dash.staging.example.com \
  --environment staging
```

The same project definition can be validated locally before it is deployed to a remote target.
