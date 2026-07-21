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
	"strings"
	"time"

	visualizationmapasset "github.com/Yacobolo/libredash/internal/visualization/mapasset"
)

const (
	planetURL        = "https://build.protomaps.com/20260720.pmtiles"
	archiveDigest    = visualizationmapasset.ArchiveSHA256
	basemapAssetsSHA = visualizationmapasset.BasemapAssetsRevision
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
	out := flag.String("out", ".data/map-assets", "map asset root directory")
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
	asset, err := visualizationmapasset.Resolve("streets")
	if err != nil {
		return err
	}
	archive, err := assetTarget(out, asset.ArchiveURL)
	if err != nil {
		return err
	}
	legacyArchive := filepath.Join(out, "libredash-streets", "basemap.pmtiles")
	if err := ensureArchive(ctx, archive, legacyArchive); err != nil {
		return err
	}
	style, err := assetTarget(out, asset.StyleURL)
	if err != nil {
		return err
	}
	if err := copyFile("static/map-assets/libredash-streets/style.json", style); err != nil {
		return fmt.Errorf("install map style: %w", err)
	}
	if err := verifyFile(style, visualizationmapasset.StyleSHA256); err != nil {
		return err
	}
	client := &http.Client{Timeout: 45 * time.Second}
	for _, font := range []string{"Noto Sans Regular", "Noto Sans Medium", "Noto Sans Italic"} {
		for _, glyphRange := range glyphRanges {
			assetURL := strings.ReplaceAll(strings.ReplaceAll(asset.GlyphsURL, "{fontstack}", url.PathEscape(font)), "{range}", glyphRange)
			target, err := assetTarget(out, assetURL)
			if err != nil {
				return err
			}
			expected, err := expectedDigest(assetURL)
			if err != nil {
				return err
			}
			remote := fmt.Sprintf("https://raw.githubusercontent.com/protomaps/basemaps-assets/%s/fonts/%s/%s.pbf", basemapAssetsSHA, url.PathEscape(font), glyphRange)
			if err := downloadIfMissing(ctx, client, remote, target, expected); err != nil {
				return err
			}
		}
	}
	for _, suffix := range []string{".json", ".png", "@2x.json", "@2x.png"} {
		assetURL := asset.SpriteURL + suffix
		target, err := assetTarget(out, assetURL)
		if err != nil {
			return err
		}
		expected, err := expectedDigest(assetURL)
		if err != nil {
			return err
		}
		remote := fmt.Sprintf("https://raw.githubusercontent.com/protomaps/basemaps-assets/%s/sprites/v4/light%s", basemapAssetsSHA, suffix)
		if err := downloadIfMissing(ctx, client, remote, target, expected); err != nil {
			return err
		}
	}
	return visualizationmapasset.VerifyInstalled(out)
}

func ensureArchive(ctx context.Context, target, legacy string) error {
	if _, err := os.Stat(target); err == nil {
		return verifyFile(target, archiveDigest)
	} else if !os.IsNotExist(err) {
		return err
	}
	if legacy != "" && legacy != target {
		if err := verifyFile(legacy, archiveDigest); err == nil {
			if err := copyFile(legacy, target); err != nil {
				return fmt.Errorf("reuse verified map archive: %w", err)
			}
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
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

func downloadIfMissing(ctx context.Context, client *http.Client, remote, target, expected string) error {
	if info, err := os.Stat(target); err == nil && info.Size() > 0 {
		if err := verifyFile(target, expected); err == nil {
			return nil
		}
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
	if err := verifyFile(temporary, expected); err != nil {
		return err
	}
	return os.Rename(temporary, target)
}

func assetTarget(root, value string) (string, error) {
	if !visualizationmapasset.IsContentAddressedURLPath(value) {
		return "", fmt.Errorf("map asset URL is not content addressed: %q", value)
	}
	decoded, err := url.PathUnescape(strings.TrimPrefix(value, "/map-assets/"))
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, filepath.FromSlash(decoded))
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("map asset target escapes root")
	}
	return target, nil
}

func expectedDigest(value string) (string, error) {
	decoded, err := url.PathUnescape(strings.TrimPrefix(value, "/map-assets/"))
	if err != nil {
		return "", err
	}
	for _, file := range visualizationmapasset.ExpectedFiles() {
		if file.Path == decoded {
			return file.Digest, nil
		}
	}
	return "", fmt.Errorf("map asset %q is not in the compiled inventory", value)
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
