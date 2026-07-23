package mapassethttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	visualizationmapasset "github.com/Yacobolo/leapview/internal/visualization/mapasset"
)

const (
	deliveryFullFileMaximum = int64(1024 * 1024)
	deliveryRangeSize       = int64(64 * 1024)
)

var ErrDeliveryValidation = errors.New("map asset delivery validation failed")

type DeliverySummary struct {
	Files          int
	FullResponses  int
	RangeResponses int
	Bytes          int64
}

type deliveryLimits struct {
	fullFileMaximum int64
	rangeSize       int64
}

// VerifyDelivery proves that the installed package is available through the
// exact same-origin URLs used by MapLibre. Small objects are fully hashed;
// large archives are checked with exact first and last byte ranges after the
// local package has passed complete digest verification.
func VerifyDelivery(ctx context.Context, root, baseURL string, client *http.Client) (DeliverySummary, error) {
	if err := visualizationmapasset.VerifyInstalled(root); err != nil {
		return DeliverySummary{}, err
	}
	return verifyDelivery(ctx, root, baseURL, visualizationmapasset.ExpectedFiles(), client, deliveryLimits{
		fullFileMaximum: deliveryFullFileMaximum,
		rangeSize:       deliveryRangeSize,
	})
}

func verifyDelivery(ctx context.Context, root, baseURL string, files []visualizationmapasset.File, baseClient *http.Client, limits deliveryLimits) (DeliverySummary, error) {
	origin, err := parseDeliveryOrigin(baseURL)
	if err != nil {
		return DeliverySummary{}, err
	}
	if strings.TrimSpace(root) == "" {
		return DeliverySummary{}, fmt.Errorf("%w: local map asset root is required", ErrDeliveryValidation)
	}
	if limits.fullFileMaximum < 1 || limits.rangeSize < 1 || limits.rangeSize > limits.fullFileMaximum {
		return DeliverySummary{}, fmt.Errorf("%w: delivery verification limits are invalid", ErrDeliveryValidation)
	}
	client := sameOriginDeliveryClient(baseClient, origin)
	var summary DeliverySummary
	for _, expected := range files {
		if err := ctx.Err(); err != nil {
			return DeliverySummary{}, err
		}
		name := filepath.Join(root, filepath.FromSlash(expected.Path))
		info, err := os.Stat(name)
		if err != nil {
			return DeliverySummary{}, fmt.Errorf("%w: stat local asset %q: %v", ErrDeliveryValidation, expected.Path, err)
		}
		target := *origin
		target.Path = "/map-assets/" + expected.Path
		if err := verifyDeliveryHead(ctx, client, &target, expected, info.Size()); err != nil {
			return DeliverySummary{}, err
		}
		if info.Size() <= limits.fullFileMaximum {
			count, err := verifyFullDelivery(ctx, client, &target, expected, info.Size())
			if err != nil {
				return DeliverySummary{}, err
			}
			summary.FullResponses++
			summary.Bytes += count
		} else {
			count, err := verifyRangedDelivery(ctx, client, &target, expected, name, info.Size(), limits.rangeSize)
			if err != nil {
				return DeliverySummary{}, err
			}
			summary.RangeResponses += 2
			summary.Bytes += count
		}
		summary.Files++
	}
	return summary, nil
}

func parseDeliveryOrigin(value string) (*url.URL, error) {
	origin, err := url.Parse(strings.TrimSpace(value))
	if err != nil || origin == nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" || origin.User != nil || (origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.Fragment != "" {
		return nil, fmt.Errorf("%w: base URL must be an HTTP(S) origin", ErrDeliveryValidation)
	}
	origin.Path = ""
	return origin, nil
}

func sameOriginDeliveryClient(base *http.Client, origin *url.URL) *http.Client {
	if base == nil {
		base = http.DefaultClient
	}
	next := *base
	configuredRedirect := base.CheckRedirect
	next.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request.URL.Scheme != origin.Scheme || request.URL.Host != origin.Host {
			return fmt.Errorf("%w: cross-origin redirect to %s", ErrDeliveryValidation, request.URL.Redacted())
		}
		if configuredRedirect != nil {
			return configuredRedirect(request, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("%w: too many redirects", ErrDeliveryValidation)
		}
		return nil
	}
	return &next
}

func verifyDeliveryHead(ctx context.Context, client *http.Client, target *url.URL, expected visualizationmapasset.File, size int64) error {
	response, err := doDeliveryRequest(ctx, client, http.MethodHead, target, "")
	if err != nil {
		return fmt.Errorf("%w: HEAD %q: %v", ErrDeliveryValidation, expected.Path, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: HEAD %q returned HTTP %d", ErrDeliveryValidation, expected.Path, response.StatusCode)
	}
	return validateDeliveryHeaders(response, expected, size, size, "")
}

func verifyFullDelivery(ctx context.Context, client *http.Client, target *url.URL, expected visualizationmapasset.File, size int64) (int64, error) {
	response, err := doDeliveryRequest(ctx, client, http.MethodGet, target, "")
	if err != nil {
		return 0, fmt.Errorf("%w: GET %q: %v", ErrDeliveryValidation, expected.Path, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%w: GET %q returned HTTP %d", ErrDeliveryValidation, expected.Path, response.StatusCode)
	}
	if err := validateDeliveryHeaders(response, expected, size, size, ""); err != nil {
		return 0, err
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, size+1))
	if err != nil {
		return 0, fmt.Errorf("%w: read %q: %v", ErrDeliveryValidation, expected.Path, err)
	}
	if int64(len(content)) != size {
		return 0, fmt.Errorf("%w: %q returned %d bytes, want %d", ErrDeliveryValidation, expected.Path, len(content), size)
	}
	actual := fmt.Sprintf("%x", sha256.Sum256(content))
	if actual != expected.Digest {
		return 0, fmt.Errorf("%w: %q digest mismatch: got %s", ErrDeliveryValidation, expected.Path, actual)
	}
	return size, nil
}

func verifyRangedDelivery(ctx context.Context, client *http.Client, target *url.URL, expected visualizationmapasset.File, name string, size, rangeSize int64) (int64, error) {
	file, err := os.Open(name)
	if err != nil {
		return 0, fmt.Errorf("%w: open local asset %q: %v", ErrDeliveryValidation, expected.Path, err)
	}
	defer file.Close()
	ranges := [][2]int64{{0, rangeSize - 1}, {size - rangeSize, size - 1}}
	var transferred int64
	for _, bounds := range ranges {
		start, end := bounds[0], bounds[1]
		local := make([]byte, end-start+1)
		if _, err := file.ReadAt(local, start); err != nil {
			return 0, fmt.Errorf("%w: read local range for %q: %v", ErrDeliveryValidation, expected.Path, err)
		}
		rangeHeader := fmt.Sprintf("bytes=%d-%d", start, end)
		response, err := doDeliveryRequest(ctx, client, http.MethodGet, target, rangeHeader)
		if err != nil {
			return 0, fmt.Errorf("%w: GET range for %q: %v", ErrDeliveryValidation, expected.Path, err)
		}
		if response.StatusCode != http.StatusPartialContent {
			response.Body.Close()
			return 0, fmt.Errorf("%w: range %q for %q returned HTTP %d", ErrDeliveryValidation, rangeHeader, expected.Path, response.StatusCode)
		}
		contentRange := fmt.Sprintf("bytes %d-%d/%d", start, end, size)
		if err := validateDeliveryHeaders(response, expected, end-start+1, size, contentRange); err != nil {
			response.Body.Close()
			return 0, err
		}
		remote, readErr := io.ReadAll(io.LimitReader(response.Body, end-start+2))
		closeErr := response.Body.Close()
		if readErr != nil {
			return 0, fmt.Errorf("%w: read remote range for %q: %v", ErrDeliveryValidation, expected.Path, readErr)
		}
		if closeErr != nil {
			return 0, fmt.Errorf("%w: close remote range for %q: %v", ErrDeliveryValidation, expected.Path, closeErr)
		}
		if !bytes.Equal(remote, local) {
			return 0, fmt.Errorf("%w: remote range %q for %q differs from the verified package", ErrDeliveryValidation, rangeHeader, expected.Path)
		}
		transferred += int64(len(remote))
	}
	return transferred, nil
}

func doDeliveryRequest(ctx context.Context, client *http.Client, method string, target *url.URL, byteRange string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept-Encoding", "identity")
	if byteRange != "" {
		request.Header.Set("Range", byteRange)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.Request == nil || response.Request.URL.Scheme != target.Scheme || response.Request.URL.Host != target.Host {
		response.Body.Close()
		return nil, fmt.Errorf("%w: response escaped the configured origin", ErrDeliveryValidation)
	}
	return response, nil
}

func validateDeliveryHeaders(response *http.Response, expected visualizationmapasset.File, responseSize, objectSize int64, contentRange string) error {
	if response.ContentLength != responseSize {
		return fmt.Errorf("%w: %q content length is %d, want %d", ErrDeliveryValidation, expected.Path, response.ContentLength, responseSize)
	}
	if strings.TrimSpace(response.Header.Get("Cache-Control")) != visualizationmapasset.ImmutableCacheControl {
		return fmt.Errorf("%w: %q cache control is %q", ErrDeliveryValidation, expected.Path, response.Header.Get("Cache-Control"))
	}
	if strings.TrimSpace(response.Header.Get("Accept-Ranges")) != "bytes" {
		return fmt.Errorf("%w: %q does not advertise byte ranges", ErrDeliveryValidation, expected.Path)
	}
	if response.Header.Get("Content-Encoding") != "" {
		return fmt.Errorf("%w: %q must be served without content encoding", ErrDeliveryValidation, expected.Path)
	}
	contentType := strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	if contentType != visualizationmapasset.ContentType(expected.Path) {
		return fmt.Errorf("%w: %q content type is %q, want %q", ErrDeliveryValidation, expected.Path, contentType, visualizationmapasset.ContentType(expected.Path))
	}
	if contentRange == "" {
		if response.Header.Get("Content-Range") != "" {
			return fmt.Errorf("%w: %q unexpectedly returned Content-Range", ErrDeliveryValidation, expected.Path)
		}
	} else if response.Header.Get("Content-Range") != contentRange {
		return fmt.Errorf("%w: %q content range is %q, want %q", ErrDeliveryValidation, expected.Path, response.Header.Get("Content-Range"), contentRange)
	}
	if objectSize < responseSize {
		return fmt.Errorf("%w: %q response exceeds object size", ErrDeliveryValidation, expected.Path)
	}
	return nil
}
