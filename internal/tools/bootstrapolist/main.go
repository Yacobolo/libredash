package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/configspec"
)

const (
	datasetHandle  = "olistbr/brazilian-ecommerce"
	datasetVersion = "2"
	archiveName    = "olistbr-brazilian-ecommerce-v2.zip"
	downloadURL    = "https://www.kaggle.com/api/v1/datasets/download/" + datasetHandle + "?dataset_version_number=" + datasetVersion
)

var expectedCSVs = []string{
	"olist_orders_dataset.csv",
	"olist_order_items_dataset.csv",
	"olist_order_payments_dataset.csv",
	"olist_products_dataset.csv",
	"olist_customers_dataset.csv",
	"olist_order_reviews_dataset.csv",
	"product_category_name_translation.csv",
}

func main() {
	out := flag.String("out", "", "directory for downloaded Olist CSV files")
	flag.Parse()
	client := &http.Client{Timeout: 10 * time.Minute}
	if err := run(client, *out); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap olist: %v\n", err)
		os.Exit(1)
	}
}

func run(client *http.Client, out string) error {
	target, err := targetDir(out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("create data directory %s: %w", target, err)
	}

	force := truthy(os.Getenv(configspec.EnvLIBREDASH_BOOTSTRAP_FORCE))
	missing := missingCSVs(target)
	if len(missing) == 0 && !force {
		fmt.Printf("Olist CSVs already available in %s\n", target)
		return nil
	}

	cacheDir, err := cacheDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("create bootstrap cache %s: %w", cacheDir, err)
	}

	archivePath := filepath.Join(cacheDir, archiveName)
	if force || !fileExists(archivePath) {
		if err := downloadArchive(client, archivePath); err != nil {
			return err
		}
	}

	copied, err := extractExpectedCSVs(archivePath, target)
	if err != nil {
		return err
	}

	if len(missing) > 0 {
		fmt.Printf("Missing CSVs: %s\n", strings.Join(missing, ", "))
	}
	if force {
		fmt.Println("Force refresh requested")
	}
	fmt.Printf("Bootstrapped %s version %s\n", datasetHandle, datasetVersion)
	fmt.Printf("Source archive: %s\n", archivePath)
	fmt.Printf("Copied %d CSV files to %s\n", copied, target)
	return nil
}

func targetDir(out string) (string, error) {
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("out is required")
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", fmt.Errorf("resolve output directory %s: %w", out, err)
	}
	return abs, nil
}

func cacheDir() (string, error) {
	if dir := os.Getenv(configspec.EnvLIBREDASH_BOOTSTRAP_CACHE_DIR); dir != "" {
		return filepath.Abs(dir)
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find user cache directory: %w", err)
	}
	return filepath.Join(base, "libredash", "olist"), nil
}

func missingCSVs(target string) []string {
	var missing []string
	for _, filename := range expectedCSVs {
		if !fileExists(filepath.Join(target, filename)) {
			missing = append(missing, filename)
		}
	}
	return missing
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func downloadArchive(client *http.Client, archivePath string) error {
	tmp, err := os.CreateTemp(filepath.Dir(archivePath), filepath.Base(archivePath)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary archive: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	req, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("create Kaggle request: %w", err)
	}
	req.Header.Set("User-Agent", "LibreDash bootstrap")

	resp, err := client.Do(req)
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("download Olist dataset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = tmp.Close()
		return fmt.Errorf("download Olist dataset: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write Olist archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close Olist archive: %w", err)
	}

	if err := os.Rename(tmpPath, archivePath); err != nil {
		return fmt.Errorf("store Olist archive at %s: %w", archivePath, err)
	}
	return nil
}

func extractExpectedCSVs(archivePath, target string) (int, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return 0, fmt.Errorf("open Olist archive %s: %w", archivePath, err)
	}
	defer reader.Close()

	remaining := make(map[string]struct{}, len(expectedCSVs))
	for _, filename := range expectedCSVs {
		remaining[filename] = struct{}{}
	}

	copied := 0
	for _, file := range reader.File {
		name := path.Clean(file.Name)
		if _, ok := remaining[name]; !ok || file.FileInfo().IsDir() {
			continue
		}
		if err := extractZipFile(file, filepath.Join(target, name)); err != nil {
			return copied, err
		}
		delete(remaining, name)
		copied++
	}

	var missing []string
	for _, filename := range expectedCSVs {
		if _, ok := remaining[filename]; ok {
			missing = append(missing, filename)
		}
	}
	if len(missing) > 0 {
		return copied, fmt.Errorf("expected CSVs missing from downloaded dataset: %s", strings.Join(missing, ", "))
	}
	return copied, nil
}

func extractZipFile(file *zip.File, destination string) error {
	source, err := file.Open()
	if err != nil {
		return fmt.Errorf("open %s from Olist archive: %w", file.Name, err)
	}
	defer source.Close()

	tmp, err := os.CreateTemp(filepath.Dir(destination), filepath.Base(destination)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary CSV %s: %w", destination, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, source); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("copy %s from Olist archive: %w", file.Name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary CSV %s: %w", destination, err)
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		return fmt.Errorf("store CSV %s: %w", destination, err)
	}
	return nil
}
