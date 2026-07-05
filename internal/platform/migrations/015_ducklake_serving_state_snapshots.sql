-- +goose Up
-- Store the DuckLake snapshot that contains a serving state's committed
-- analytical table state. The active serving state pointer remains the activation
-- switch; this column pins that serving state to one immutable DuckLake snapshot.

ALTER TABLE serving_states ADD COLUMN ducklake_snapshot_id INTEGER NOT NULL DEFAULT 0;
