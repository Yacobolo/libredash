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

	visualizationmapasset "github.com/Yacobolo/leapview/internal/visualization/mapasset"
	visualizationmapassethttp "github.com/Yacobolo/leapview/internal/visualization/mapasset/http"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	planetURL           = "https://build.protomaps.com/20260720.pmtiles"
	archiveDigest       = visualizationmapasset.ArchiveSHA256
	globalArchiveDigest = "2d97ee8907670936ab722da7ca06eafec0734392f73fa1cd337d4debd85d676f"
	regionalBounds      = "-82,-56,-30,14"
	regionalMinimumZoom = "7"
	regionalMaximumZoom = "10"
	basemapAssetsSHA    = visualizationmapasset.BasemapAssetsRevision
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
	publishBucket := flag.String("publish-s3-bucket", "", "publish verified assets to this S3 bucket")
	publishPrefix := flag.String("publish-s3-prefix", "map-assets", "S3 key prefix used for published assets")
	publishRegion := flag.String("publish-s3-region", "", "AWS region override for map asset publication")
	publishEndpoint := flag.String("publish-s3-endpoint", "", "S3-compatible endpoint override")
	publishPathStyle := flag.Bool("publish-s3-path-style", false, "use path-style S3 addressing")
	verifyBaseURL := flag.String("verify-base-url", "", "verify the installed package through this same-origin HTTP(S) endpoint")
	flag.Parse()
	ctx := context.Background()
	if err := install(ctx, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if strings.TrimSpace(*publishBucket) != "" {
		summary, err := publishS3(ctx, *out, *publishBucket, *publishPrefix, *publishRegion, *publishEndpoint, *publishPathStyle)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("published map assets: uploaded=%d reused=%d bytes=%d\n", summary.Uploaded, summary.Reused, summary.Bytes)
	}
	if strings.TrimSpace(*verifyBaseURL) != "" {
		client := &http.Client{Timeout: 2 * time.Minute}
		summary, err := visualizationmapassethttp.VerifyDelivery(ctx, *out, *verifyBaseURL, client)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("verified map asset delivery: files=%d full=%d ranges=%d bytes=%d\n", summary.Files, summary.FullResponses, summary.RangeResponses, summary.Bytes)
	}
}

func publishS3(ctx context.Context, root, bucket, prefix, region, endpoint string, pathStyle bool) (visualizationmapasset.PublicationSummary, error) {
	loadOptions := []func(*awsconfig.LoadOptions) error{}
	if strings.TrimSpace(region) != "" {
		loadOptions = append(loadOptions, awsconfig.WithRegion(strings.TrimSpace(region)))
	}
	config, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return visualizationmapasset.PublicationSummary{}, fmt.Errorf("load AWS configuration for map asset publication: %w", err)
	}
	client := awss3.NewFromConfig(config, func(options *awss3.Options) {
		options.UsePathStyle = pathStyle
		if strings.TrimSpace(endpoint) != "" {
			options.BaseEndpoint = aws.String(strings.TrimRight(strings.TrimSpace(endpoint), "/"))
		}
	})
	store, err := visualizationmapasset.NewS3PublicationStore(client, visualizationmapasset.S3PublicationConfig{Bucket: bucket, Prefix: prefix})
	if err != nil {
		return visualizationmapasset.PublicationSummary{}, err
	}
	return visualizationmapasset.PublishInstalled(ctx, root, store)
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
	legacyArchive := filepath.Join(out, "leapview-streets", "basemap.pmtiles")
	if err := ensureArchive(ctx, archive, legacyArchive); err != nil {
		return err
	}
	style, err := assetTarget(out, asset.StyleURL)
	if err != nil {
		return err
	}
	if err := copyFile("static/map-assets/leapview-streets/style.json", style); err != nil {
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
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	build, err := os.MkdirTemp(filepath.Dir(target), ".map-build-")
	if err != nil {
		return fmt.Errorf("create map archive build directory: %w", err)
	}
	defer os.RemoveAll(build)
	global := filepath.Join(build, "global-z0-z6.pmtiles")
	installedGlobal := filepath.Join(filepath.Dir(filepath.Dir(target)), globalArchiveDigest, "basemap.pmtiles")
	if err := reuseVerifiedArchive(installedGlobal, legacy, globalArchiveDigest, global); err != nil {
		if err := runPMTiles(ctx, "extract", planetURL, global, "--maxzoom=6", "--download-threads=8", "--quiet"); err != nil {
			return fmt.Errorf("extract pinned global PMTiles: %w", err)
		}
		if err := verifyFile(global, globalArchiveDigest); err != nil {
			return err
		}
	}
	regional := filepath.Join(build, "south-america-z7-z10.pmtiles")
	if err := runPMTiles(ctx, "extract", planetURL, regional, "--bbox="+regionalBounds, "--minzoom="+regionalMinimumZoom, "--maxzoom="+regionalMaximumZoom, "--download-threads=8", "--quiet"); err != nil {
		return fmt.Errorf("extract pinned regional PMTiles: %w", err)
	}
	temporary := target + ".partial"
	if err := os.Remove(temporary); err != nil && !os.IsNotExist(err) {
		return err
	}
	defer os.Remove(temporary)
	if err := runPMTiles(ctx, "merge", global, regional, temporary, "--quiet"); err != nil {
		return fmt.Errorf("merge global and regional PMTiles: %w", err)
	}
	if err := verifyFile(temporary, archiveDigest); err != nil {
		return err
	}
	if err := os.Rename(temporary, target); err != nil {
		return err
	}
	return nil
}

func reuseVerifiedArchive(primary, legacy, digest, target string) error {
	for _, candidate := range []string{primary, legacy} {
		if candidate == "" {
			continue
		}
		if err := verifyFile(candidate, digest); err == nil {
			if err := copyFile(candidate, target); err != nil {
				return fmt.Errorf("reuse verified map archive: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("verified map archive %s is not installed", digest)
}

func runPMTiles(ctx context.Context, arguments ...string) error {
	command := exec.CommandContext(ctx, "go", append([]string{"run", "github.com/protomaps/go-pmtiles@v1.31.1"}, arguments...)...)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
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
