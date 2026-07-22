# Automation and CI

Treat validation and deployment as separate CI steps. Validate every change, generate a plan for review, then deploy only from an approved branch.

```sh
leapview validate --project dashboards/leapview.yaml --json
leapview plan --project dashboards/leapview.yaml --target "$LEAPVIEW_TARGET" --json
leapview deploy --project dashboards/leapview.yaml --target "$LEAPVIEW_TARGET" --auto-approve
```

Provide tokens only through the deployment environment or your CI secret manager.
