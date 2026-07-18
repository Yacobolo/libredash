# Install and authenticate

Run the CLI from the LibreDash release or from a checked-out repository.

Store an API token for a target before calling remote operations.

```sh
libredash login --target https://dash.example.com --token "$LIBREDASH_API_TOKEN"
```

For automation, provide `LIBREDASH_TARGET` and `LIBREDASH_API_TOKEN` through the environment instead of writing credentials to a local target configuration.
