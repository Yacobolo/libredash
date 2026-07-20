package main

import (
	"archive/zip"
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Yacobolo/leapview/internal/configspec"
)

const (
	datasetName = "MovieLens 32M"
	archiveName = "ml-32m.zip"
	downloadURL = "https://files.grouplens.org/datasets/movielens/" + archiveName
)

type expectedFile struct {
	Name string
	MD5  string
}

var expectedFiles = []expectedFile{
	{Name: "links.csv", MD5: "8f033867bcb4e6be8792b21468b4fa6e"},
	{Name: "movies.csv", MD5: "0df90835c19151f9d819d0822e190797"},
	{Name: "ratings.csv", MD5: "cf12b74f9ad4b94a011f079e26d4270a"},
	{Name: "tags.csv", MD5: "963bf4fa4de6b8901868fddd3eb54567"},
}

func main() {
	out := flag.String("out", "", "directory for downloaded MovieLens CSV files")
	flag.Parse()
	client := &http.Client{Timeout: 30 * time.Minute}
	if err := run(client, *out); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap movielens: %v\n", err)
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

	force := truthy(os.Getenv(configspec.EnvLEAPVIEW_BOOTSTRAP_FORCE))
	missing := missingFiles(target)
	if !refreshRequired(missing, force) {
		if err := verifyExpectedFileChecksums(target); err == nil {
			fmt.Printf("%s CSVs already available in %s\n", datasetName, target)
			return nil
		} else {
			fmt.Printf("Checksum refresh required: %v\n", err)
		}
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

	copied, err := extractExpectedFiles(archivePath, target)
	if err != nil {
		return err
	}
	if err := verifyExpectedFileChecksums(target); err != nil {
		return err
	}

	if len(missing) > 0 {
		fmt.Printf("Missing CSVs: %s\n", strings.Join(missing, ", "))
	}
	if force {
		fmt.Println("Force refresh requested")
	}
	fmt.Printf("Bootstrapped %s\n", datasetName)
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
	if dir := os.Getenv(configspec.EnvLEAPVIEW_BOOTSTRAP_CACHE_DIR); dir != "" {
		return filepath.Abs(dir)
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find user cache directory: %w", err)
	}
	return filepath.Join(base, "leapview", "movielens"), nil
}

func missingFiles(target string) []string {
	var missing []string
	for _, file := range expectedFiles {
		if !fileExists(filepath.Join(target, file.Name)) {
			missing = append(missing, file.Name)
		}
	}
	return missing
}

func refreshRequired(missing []string, force bool) bool {
	return force || len(missing) > 0
}

func fileExists(file string) bool {
	info, err := os.Stat(file)
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
		return fmt.Errorf("create MovieLens request: %w", err)
	}
	req.Header.Set("User-Agent", "LeapView bootstrap")

	resp, err := client.Do(req)
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("download MovieLens dataset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = tmp.Close()
		return fmt.Errorf("download MovieLens dataset: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write MovieLens archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close MovieLens archive: %w", err)
	}

	if err := os.Rename(tmpPath, archivePath); err != nil {
		return fmt.Errorf("store MovieLens archive at %s: %w", archivePath, err)
	}
	return nil
}

func extractExpectedFiles(archivePath, target string) (int, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return 0, fmt.Errorf("open MovieLens archive %s: %w", archivePath, err)
	}
	defer reader.Close()

	remaining := make(map[string]struct{}, len(expectedFiles))
	for _, file := range expectedFiles {
		remaining[file.Name] = struct{}{}
	}

	copied := 0
	for _, file := range reader.File {
		name := expectedArchiveFileName(file)
		if _, ok := remaining[name]; !ok {
			continue
		}
		if err := extractZipFile(file, filepath.Join(target, name)); err != nil {
			return copied, err
		}
		delete(remaining, name)
		copied++
	}

	var missing []string
	for _, file := range expectedFiles {
		if _, ok := remaining[file.Name]; ok {
			missing = append(missing, file.Name)
		}
	}
	if len(missing) > 0 {
		return copied, fmt.Errorf("expected CSVs missing from downloaded dataset: %s", strings.Join(missing, ", "))
	}
	return copied, nil
}

func expectedArchiveFileName(file *zip.File) string {
	if file.FileInfo().IsDir() {
		return ""
	}
	name := path.Clean(file.Name)
	if strings.HasPrefix(name, "../") || name == ".." || path.IsAbs(name) {
		return ""
	}
	return path.Base(name)
}

func extractZipFile(file *zip.File, destination string) error {
	source, err := file.Open()
	if err != nil {
		return fmt.Errorf("open %s from MovieLens archive: %w", file.Name, err)
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
		return fmt.Errorf("copy %s from MovieLens archive: %w", file.Name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary CSV %s: %w", destination, err)
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		return fmt.Errorf("store CSV %s: %w", destination, err)
	}
	return nil
}

func verifyExpectedFileChecksums(target string) error {
	for _, file := range expectedFiles {
		got, err := md5File(filepath.Join(target, file.Name))
		if err != nil {
			return err
		}
		if got != file.MD5 {
			return fmt.Errorf("%s checksum = %s, want %s", file.Name, got, file.MD5)
		}
	}
	return nil
}

func md5File(file string) (string, error) {
	in, err := os.Open(file)
	if err != nil {
		return "", fmt.Errorf("open %s for checksum: %w", file, err)
	}
	defer in.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, in); err != nil {
		return "", fmt.Errorf("read %s for checksum: %w", file, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
