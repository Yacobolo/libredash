# Get started with LibreDash

LibreDash keeps the semantic layer, report definition, and visual runtime together. Start with the included workspace, then make it your own.

This guide uses a source checkout so you can edit the bundled project. To evaluate the running server without building LibreDash, start with the public-image path in [Installation](/docs/installation).

## Bootstrap the workspace

Download the sample data and prepare the local workspace.

```sh
task bootstrap
```

## Run LibreDash

Start the local application and open the dashboard workspace.

```sh
task dev
```

## Edit the model and dashboard

Keep the project entry point, shared data inputs, and workspace-owned models and dashboards together under `dashboards/`.

```text
dashboards/
  libredash.yaml
  connections/
    olist.yaml
  sources/
    olist.orders.yaml
  workspaces/
    sales/
      workspace.yaml
      models/
        orders.yaml
      semantic-models/
        sales.yaml
      dashboards/
        executive-sales.yaml
```

## Explore the visual system

See the chart, table, matrix, and pivot components that the dashboard contract can render in the [visual gallery](/visuals). The project source and issue tracker are available on [GitHub](https://github.com/Yacobolo/libredash).
