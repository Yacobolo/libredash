# Automation and CI

Treat validation and deployment as separate CI steps. Validate every change, generate a plan for review, then deploy only from an approved branch.

```sh
libredash validate --project dashboards/libredash.yaml --json
libredash plan --project dashboards/libredash.yaml --target "$LIBREDASH_TARGET" --json
libredash deploy --project dashboards/libredash.yaml --target "$LIBREDASH_TARGET" --auto-approve
```

Provide tokens only through the deployment environment or your CI secret manager.
