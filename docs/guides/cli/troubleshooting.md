# Troubleshooting

Start with the local validation error: it points to the project file and the invalid field. For remote operations, confirm the target URL, API token, and workspace selection.

```sh
libredash config validate --production
libredash healthcheck --url https://dash.example.com/healthz
```

Use `libredash <command> --help` for a quick reminder, then use the command reference for the complete option list.
