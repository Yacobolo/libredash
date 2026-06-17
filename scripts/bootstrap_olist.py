#!/usr/bin/env python3
from __future__ import annotations

import os
import shutil
import warnings
from pathlib import Path

warnings.filterwarnings("ignore", message="urllib3 v2 only supports OpenSSL.*")

import kagglehub


DATASET = "olistbr/brazilian-ecommerce"
EXPECTED_CSVS = (
    "olist_orders_dataset.csv",
    "olist_order_items_dataset.csv",
    "olist_order_payments_dataset.csv",
    "olist_products_dataset.csv",
    "olist_customers_dataset.csv",
    "olist_order_reviews_dataset.csv",
    "product_category_name_translation.csv",
)


def missing_csvs(target: Path) -> list[str]:
    return [
        filename
        for filename in EXPECTED_CSVS
        if not (target / filename).is_file()
    ]


def main() -> None:
    target = Path(os.environ.get("LIBREDASH_DATA_DIR", ".data/olist")).resolve()
    target.mkdir(parents=True, exist_ok=True)

    force = os.environ.get("LIBREDASH_BOOTSTRAP_FORCE", "").lower() in {"1", "true", "yes"}
    missing = missing_csvs(target)
    if not missing and not force:
        print(f"Olist CSVs already available in {target}")
        return

    source = Path(kagglehub.dataset_download(DATASET))
    copied = 0

    for filename in EXPECTED_CSVS:
        csv = source / filename
        if not csv.is_file():
            raise FileNotFoundError(f"Expected {filename} in downloaded dataset at {source}")
        destination = target / csv.name
        shutil.copy2(csv, destination)
        copied += 1

    if missing:
        print(f"Missing CSVs: {', '.join(missing)}")
    if force:
        print("Force refresh requested")
    print(f"Bootstrapped {DATASET}")
    print(f"Source: {source}")
    print(f"Copied {copied} CSV files to {target}")


if __name__ == "__main__":
    main()
