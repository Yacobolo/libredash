# Core concepts

LeapView separates physical data access, reusable business meaning, and dashboard presentation. The three layers are delivered together, but each layer has a clear responsibility and can be reviewed without understanding every implementation detail below it.

## Data resources

A **connection** describes how LeapView reaches data. A **source** gives a stable project-level identity to a file, table, object, or other input exposed through that connection. A workspace must explicitly list the project sources it is permitted to use.

A **model table** turns one or more permitted sources into a stable analytical table. Its primary key, grain, fields, and SQL transformation make source cleanup and expensive reusable work explicit. Model tables are materialized so dashboards do not repeat raw-source logic on every interaction.

## Semantic resources

A **semantic model** selects model tables and exposes reusable dimensions, measures, metrics, and relationships. Dashboards and headless clients refer to names such as `orders.purchase_month`, `revenue`, or `aov` instead of embedding separate SQL expressions.

This layer is where business meaning belongs. A measure defines its aggregation, fact table, input field, empty-result behavior, and formatting once. A relationship defines a valid path between compatible table grains. Validation rejects missing or ambiguous references before users can query them.

## Presentation resources

A **dashboard** chooses one semantic model and defines filters, visual queries, tabular queries, pages, and layout. Page components reference reusable filter, visual, or table definitions by stable ID. A layout change therefore does not need to rewrite query logic, and the same semantic field behaves consistently across several report surfaces.

Charts are renderer-neutral at the dashboard contract. LeapView maps visual shapes to renderer plugins—ECharts is the first built-in chart renderer—while keeping the signal and query contracts owned by LeapView.

## Delivery and ownership

A **project** is the atomic configuration delivery unit. It discovers global connections and sources plus one or more workspaces. A **workspace** is an asset container for model tables, semantic models, dashboards, and access configuration. Agent conversations are global and principal-owned; tools select a workspace only when operating on its assets. An **environment** such as `dev`, `staging`, or `prod` selects an active validated deployment and managed-data revisions.

These boundaries solve different problems:

- Project scope keeps shared data definitions and deployments coherent.
- Workspace scope establishes product ownership and authorization.
- Environment scope separates serving state without duplicating resources.

## Runtime state

The browser does not own query truth. Go renders the initial page and signal contract. User actions become small commands. The server resolves the active deployment, authorization, filters, selections, semantic fields, and bounded query. DuckDB executes against active analytical state, and Datastar patches results back to Lit components.

That cycle gives browser interactions the responsiveness of a client application while preserving server-side authorization, cancellation, semantic resolution, and data access.

Read [Projects, workspaces, and environments](/docs/concepts/projects-workspaces-environments) next, or continue directly to [Build dashboards](/docs/guides/build) if these boundaries are already familiar.
