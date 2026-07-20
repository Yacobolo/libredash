# Install and authenticate

Run the CLI from the LeapView release or from a checked-out repository.

Store an API token for a target before calling remote operations.

```sh
leapview login --target https://dash.example.com --token "$LEAPVIEW_API_TOKEN"
```

For automation, provide `LEAPVIEW_TARGET` and `LEAPVIEW_API_TOKEN` through the environment instead of writing credentials to a local target configuration.
