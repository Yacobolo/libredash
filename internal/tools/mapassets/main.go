package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	planetURL        = "https://build.protomaps.com/20260720.pmtiles"
	archiveDigest    = "2d97ee8907670936ab722da7ca06eafec0734392f73fa1cd337d4debd85d676f"
	basemapAssetsSHA = "028c18f713baecad011301ff7a69acc39bcc2ae7"
)

var glyphRanges = []string{
	"0-255",
	"256-511",
	"512-767",
	"768-1023",
	"1024-1279",
	"1280-1535",
	"1536-1791",
	"4096-4351",
	"11520-11775",
}

func main() {
	out := flag.String("out", ".data/map-assets/libredash-streets", "map asset output directory")
	flag.Parse()
	if err := install(context.Background(), *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func install(ctx context.Context, out string) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	archive := filepath.Join(out, "basemap.pmtiles")
	if err := ensureArchive(ctx, archive); err != nil {
		return err
	}
	if err := copyFile("static/map-assets/libredash-streets/style.json", filepath.Join(out, "style.json")); err != nil {
		return fmt.Errorf("install map style: %w", err)
	}
	client := &http.Client{Timeout: 45 * time.Second}
	for _, font := range []string{"Noto Sans Regular", "Noto Sans Medium", "Noto Sans Italic"} {
		for _, glyphRange := range glyphRanges {
			rel := filepath.Join("glyphs", font, glyphRange+".pbf")
			remote := fmt.Sprintf("https://raw.githubusercontent.com/protomaps/basemaps-assets/%s/fonts/%s/%s.pbf", basemapAssetsSHA, url.PathEscape(font), glyphRange)
			if err := downloadIfMissing(ctx, client, remote, filepath.Join(out, rel)); err != nil {
				return err
			}
		}
	}
	for _, suffix := range []string{".json", ".png", "@2x.json", "@2x.png"} {
		remote := fmt.Sprintf("https://raw.githubusercontent.com/protomaps/basemaps-assets/%s/sprites/v4/light%s", basemapAssetsSHA, suffix)
		if err := downloadIfMissing(ctx, client, remote, filepath.Join(out, "sprites", "libredash"+suffix)); err != nil {
			return err
		}
	}
	return nil
}

func ensureArchive(ctx context.Context, target string) error {
	if _, err := os.Stat(target); err == nil {
		return verifyFile(target, archiveDigest)
	} else if !os.IsNotExist(err) {
		return err
	}
	temporary := target + ".partial"
	command := exec.CommandContext(ctx, "go", "run", "github.com/protomaps/go-pmtiles@v1.31.1", "extract", planetURL, temporary, "--maxzoom=6", "--download-threads=8", "--quiet")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("extract pinned PMTiles: %w", err)
	}
	if err := verifyFile(temporary, archiveDigest); err != nil {
		return err
	}
	if err := os.Rename(temporary, target); err != nil {
		return err
	}
	return nil
}

func downloadIfMissing(ctx context.Context, client *http.Client, remote, target string) error {
	if info, err := os.Stat(target); err == nil && info.Size() > 0 {
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, remote, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download %s: %w", remote, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", remote, response.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	temporary := target + ".partial"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temporary, target)
}

func verifyFile(name, expected string) error {
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := fmt.Sprintf("%x", hash.Sum(nil))
	if actual != expected {
		return fmt.Errorf("map asset %s digest mismatch: got %s", name, actual)
	}
	return nil
}

func copyFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
