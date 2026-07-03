-- +goose Up
-- Store the DuckLake snapshot that contains a deployment's committed
-- analytical table state. The active deployment pointer remains the activation
-- switch; this column pins that deployment to one immutable DuckLake snapshot.

ALTER TABLE deployments ADD COLUMN ducklake_snapshot_id INTEGER NOT NULL DEFAULT 0;
