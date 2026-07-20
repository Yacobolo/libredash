package cli

import (
	"context"
	"time"

	"github.com/Yacobolo/leapview/internal/manageddata/control"
	"github.com/Yacobolo/leapview/internal/manageddata/maintenance"
	"github.com/Yacobolo/leapview/internal/manageddata/s3multipart"
)

type managedDataMaintenance struct {
	uploads   *control.Service
	multipart *s3multipart.Service
	uploadTTL time.Duration
	collector *maintenance.BlobCollector
	runtime   *maintenance.RuntimeViewCollector
}

func (m managedDataMaintenance) ExpireUploads(ctx context.Context) (control.ExpireResult, error) {
	result, err := m.uploads.ExpireUploads(ctx)
	if err != nil {
		return result, err
	}
	if m.multipart != nil {
		_, err = m.multipart.RecoverOrphaned(ctx, time.Now().UTC().Add(-m.uploadTTL), 100)
		if err != nil {
			return result, err
		}
	}
	if m.collector != nil {
		_, err = m.collector.Run(ctx)
		if err != nil {
			return result, err
		}
	}
	if m.runtime != nil {
		_, err = m.runtime.Run(ctx)
	}
	return result, err
}
