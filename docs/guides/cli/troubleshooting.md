# Troubleshooting

Start with the local validation error: it points to the project file and the invalid field. For remote operations, confirm the target URL, API token, and workspace selection.

```sh
leapview config validate --production
leapview healthcheck --url https://dash.example.com/healthz
```

Use `leapview <command> --help` for a quick reminder, then use the command reference for the complete option list.
