package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/Yacobolo/libredash/internal/config"
	"github.com/Yacobolo/libredash/internal/manageddata"
	"github.com/Yacobolo/libredash/internal/manageddata/control"
	"github.com/Yacobolo/libredash/internal/manageddata/maintenance"
	maintenancesqlite "github.com/Yacobolo/libredash/internal/manageddata/maintenance/sqlite"
	"github.com/Yacobolo/libredash/internal/manageddata/runtimeview"
	"github.com/Yacobolo/libredash/internal/manageddata/s3multipart"
	"github.com/Yacobolo/libredash/internal/manageddata/storage"
	managedfilesystem "github.com/Yacobolo/libredash/internal/manageddata/storage/filesystem"
	manageds3 "github.com/Yacobolo/libredash/internal/manageddata/storage/s3"
	managedtus "github.com/Yacobolo/libredash/internal/manageddata/storage/tus"
	"github.com/Yacobolo/libredash/internal/securefs"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	managedDataTusPath             = "/upload-protocols/tus"
	managedDataS3MultipartTemplate = "/api/v1/projects/{project}/connections/{connection}/upload-sessions/{uploadSession}/s3-multipart-uploads"
)

type managedDataStorage struct {
	blobs        storage.BlobStore
	inventory    storage.BlobInventory
	transport    control.Transport
	materializer manageddata.RevisionMaterializer
	runtimeCache *runtimeview.Cache
	tus          http.Handler
	s3           *manageds3.Store
}

func newManagedDataStorage(ctx context.Context, cfg config.Config) (managedDataStorage, error) {
	root, err := filepath.Abs(strings.TrimSpace(cfg.ManagedDataDir))
	if err != nil || strings.TrimSpace(cfg.ManagedDataDir) == "" {
		return managedDataStorage{}, fmt.Errorf("%w: managed-data directory is required", storage.ErrInvalid)
	}
	if err := securefs.EnsurePrivateDir(root); err != nil {
		return managedDataStorage{}, err
	}

	var result managedDataStorage
	switch strings.TrimSpace(cfg.ManagedDataBackend) {
	case "local":
		blobs, err := managedfilesystem.New(filepath.Join(root, "objects"))
		if err != nil {
			return managedDataStorage{}, err
		}
		engine, err := managedtus.New(filepath.Join(root, "uploads"), blobs)
		if err != nil {
			return managedDataStorage{}, err
		}
		transport, err := control.NewTusTransport("local", managedDataTusPath, engine)
		if err != nil {
			return managedDataStorage{}, err
		}
		handler, err := engine.HTTPHandler(managedtus.HTTPConfig{BasePath: managedDataTusPath, MaxSize: cfg.ManagedDataMaxFileBytes})
		if err != nil {
			return managedDataStorage{}, err
		}
		capacity, err := maintenance.NewCapacityChecker(root, cfg.ManagedDataMinFreeBytes)
		if err != nil {
			return managedDataStorage{}, err
		}
		result.blobs, result.transport, result.materializer, result.tus = blobs, transport, blobs, capacityProtectedTus(handler, capacity)
	case "s3":
		store, err := newManagedDataS3Store(ctx, cfg)
		if err != nil {
			return managedDataStorage{}, err
		}
		transport, err := control.NewS3MultipartTransport("s3", control.S3MultipartDescription{
			CreateEndpoint:  managedDataS3MultipartTemplate,
			MinimumPartSize: s3multipart.MinimumPartSize,
			MaximumPartSize: s3multipart.MaximumPartSize,
			MaximumParts:    s3multipart.MaximumParts,
		})
		if err != nil {
			return managedDataStorage{}, err
		}
		cache, err := runtimeview.New(filepath.Join(root, "runtime"), store)
		if err != nil {
			return managedDataStorage{}, err
		}
		result.blobs, result.transport, result.materializer, result.runtimeCache, result.s3 = store, transport, cache, cache, store
	default:
		return managedDataStorage{}, fmt.Errorf("%w: managed-data backend must be local or s3", storage.ErrInvalid)
	}
	inventory, ok := result.blobs.(storage.BlobInventory)
	if !ok {
		return managedDataStorage{}, fmt.Errorf("%w: managed-data backend has no blob inventory", storage.ErrInvalid)
	}
	result.inventory = inventory
	return result, nil
}

func newManagedDataCollector(db *sql.DB, services managedDataStorage, cfg config.Config) (*maintenance.BlobCollector, error) {
	reachability, err := maintenancesqlite.New(db)
	if err != nil {
		return nil, err
	}
	return maintenance.NewBlobCollector(services.inventory, reachability, maintenance.BlobGCConfig{
		GraceAge: cfg.ManagedDataGCGracePeriod,
	})
}

func newManagedDataRuntimeCollector(services managedDataStorage, cfg config.Config) (*maintenance.RuntimeViewCollector, error) {
	if services.runtimeCache == nil {
		return nil, nil
	}
	return maintenance.NewRuntimeViewCollector(services.runtimeCache, maintenance.RuntimeViewGCConfig{
		GraceAge: cfg.ManagedDataGCGracePeriod,
		Limit:    100,
	})
}

func capacityProtectedTus(next http.Handler, capacity *maintenance.CapacityChecker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			next.ServeHTTP(w, r)
			return
		}
		if r.ContentLength < 0 {
			http.Error(w, "Content-Length is required", http.StatusLengthRequired)
			return
		}
		reservation, err := capacity.Reserve(r.Context(), r.ContentLength)
		if err != nil {
			status := http.StatusServiceUnavailable
			if errors.Is(err, maintenance.ErrInsufficientCapacity) {
				status = http.StatusInsufficientStorage
			}
			http.Error(w, http.StatusText(status), status)
			return
		}
		defer reservation.Release()
		next.ServeHTTP(w, r)
	})
}

func newManagedDataS3Store(ctx context.Context, cfg config.Config) (*manageds3.Store, error) {
	loadOptions := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(strings.TrimSpace(cfg.ManagedDataS3Region))}
	if cfg.ManagedDataS3AccessKeyID != "" {
		provider := credentials.NewStaticCredentialsProvider(
			cfg.ManagedDataS3AccessKeyID,
			cfg.ManagedDataS3SecretAccessKey,
			cfg.ManagedDataS3SessionToken,
		)
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(provider))
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("initialize managed-data S3 client: %w", err)
	}
	client := awss3.NewFromConfig(awsConfig, func(options *awss3.Options) {
		options.UsePathStyle = cfg.ManagedDataS3PathStyle
		if endpoint := strings.TrimSpace(cfg.ManagedDataS3Endpoint); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
	})
	return manageds3.New(client, awss3.NewPresignClient(client), manageds3.Config{
		Bucket: cfg.ManagedDataS3Bucket,
		Prefix: cfg.ManagedDataS3Prefix,
	})
}

func newManagedDataControl(repo control.Repository, services managedDataStorage, cfg config.Config) (*control.Service, error) {
	return control.New(repo, services.blobs, control.Config{
		Limits: manageddata.Limits{
			MaxFiles:         cfg.ManagedDataMaxFiles,
			MaxFileBytes:     cfg.ManagedDataMaxFileBytes,
			MaxRevisionBytes: cfg.ManagedDataMaxRevisionBytes,
		},
		UploadTTL: cfg.ManagedDataUploadSessionTTL,
		Transport: services.transport,
	})
}
