// Package resolver reconstructs persisted managed-data revisions for serving runtimes.
package resolver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/Yacobolo/leapview/internal/manageddata"
	"github.com/Yacobolo/leapview/internal/runtimehost"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
)

var (
	ErrInvalidMetadata     = errors.New("invalid managed data metadata")
	ErrRevisionNotReady    = errors.New("managed data revision is not ready")
	ErrAmbiguousConnection = errors.New("ambiguous managed data connection")
	ErrRepository          = errors.New("managed data repository failure")
	ErrMaterialization     = errors.New("managed data materialization failure")
)

// Repository is the read-only portion of manageddata.Repository needed to
// reconstruct a serving state's immutable managed-data bindings.
type Repository interface {
	ListServingStateBindings(context.Context, string) ([]manageddata.ServingStateBinding, error)
	CollectionByID(context.Context, string) (manageddata.Collection, error)
	RevisionByID(context.Context, string) (manageddata.Revision, error)
	ListRevisionFiles(context.Context, string) ([]manageddata.RevisionFile, error)
}

// ServingStateRepository supplies the environment that bindings must match.
type ServingStateRepository interface {
	ByID(context.Context, servingstate.ID) (servingstate.State, error)
}

// Resolver validates persisted bindings and materializes their immutable views.
type Resolver struct {
	repository    Repository
	servingStates ServingStateRepository
	materializer  manageddata.RevisionMaterializer
}

// New constructs a managed-data runtime resolver.
func New(repository Repository, servingStates ServingStateRepository, materializer manageddata.RevisionMaterializer) (*Resolver, error) {
	if repository == nil {
		return nil, fmt.Errorf("repository is required")
	}
	if servingStates == nil {
		return nil, fmt.Errorf("serving state repository is required")
	}
	if materializer == nil {
		return nil, fmt.Errorf("revision materializer is required")
	}
	return &Resolver{repository: repository, servingStates: servingStates, materializer: materializer}, nil
}

// ResolveManagedData resolves the bindings already persisted for servingStateID.
// RevisionID is a SHA-256 digest over canonical JSON containing the sorted
// (project, connection, manifest digest) tuples, including for one binding.
func (r *Resolver) ResolveManagedData(ctx context.Context, servingStateID servingstate.ID) (runtimehost.ManagedDataResolution, error) {
	bindings, err := r.loadBindings(ctx, servingStateID)
	if err != nil {
		return runtimehost.ManagedDataResolution{}, err
	}
	return r.resolveBindings(ctx, servingStateID, bindings)
}

type resolvedBinding struct {
	project        string
	connection     string
	manifestDigest string
	manifest       manageddata.Manifest
}

func (r *Resolver) resolveBindings(ctx context.Context, servingStateID servingstate.ID, bindings []manageddata.ServingStateBinding) (runtimehost.ManagedDataResolution, error) {
	if !canonicalIdentifier(string(servingStateID)) {
		return runtimehost.ManagedDataResolution{}, invalidMetadata("serving state id is invalid")
	}
	state, err := r.servingStates.ByID(ctx, servingStateID)
	if err != nil {
		return runtimehost.ManagedDataResolution{}, sanitizeRepositoryError(ctx, "load serving state", err)
	}
	stateEnvironment, normalizeErr := manageddata.NormalizeEnvironment(string(state.Environment))
	if state.ID != servingStateID || normalizeErr != nil || string(stateEnvironment) != string(state.Environment) {
		return runtimehost.ManagedDataResolution{}, invalidMetadata("serving state relationship or environment is invalid")
	}
	if len(bindings) == 0 {
		return runtimehost.ManagedDataResolution{Roots: map[string]string{}}, nil
	}

	resolved := make([]resolvedBinding, 0, len(bindings))
	connections := make(map[string]struct{}, len(bindings))
	collections := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		if binding.ServingStateID != string(servingStateID) || !canonicalIdentifier(binding.CollectionID) || !canonicalIdentifier(binding.RevisionID) {
			return runtimehost.ManagedDataResolution{}, invalidMetadata("binding relationship is invalid")
		}
		if _, duplicate := collections[binding.CollectionID]; duplicate {
			return runtimehost.ManagedDataResolution{}, invalidMetadata("collection has duplicate bindings")
		}
		collections[binding.CollectionID] = struct{}{}

		normalizedEnvironment, normalizeErr := manageddata.NormalizeEnvironment(string(binding.Environment))
		if normalizeErr != nil || normalizedEnvironment != binding.Environment {
			return runtimehost.ManagedDataResolution{}, invalidMetadata("binding environment is invalid")
		}
		if binding.Environment != stateEnvironment {
			return runtimehost.ManagedDataResolution{}, invalidMetadata("binding environment does not match serving state")
		}

		collection, loadErr := r.repository.CollectionByID(ctx, binding.CollectionID)
		if loadErr != nil {
			return runtimehost.ManagedDataResolution{}, sanitizeRepositoryError(ctx, "load collection", loadErr)
		}
		if collection.ID != binding.CollectionID || !validAuthoredIdentity(collection.ProjectID) || !validAuthoredIdentity(collection.ConnectionName) {
			return runtimehost.ManagedDataResolution{}, invalidMetadata("collection relationship or identity is invalid")
		}
		if _, duplicate := connections[collection.ConnectionName]; duplicate {
			return runtimehost.ManagedDataResolution{}, fmt.Errorf("%w: authored connection name %q is bound more than once", ErrAmbiguousConnection, collection.ConnectionName)
		}
		connections[collection.ConnectionName] = struct{}{}

		revision, loadErr := r.repository.RevisionByID(ctx, binding.RevisionID)
		if loadErr != nil {
			return runtimehost.ManagedDataResolution{}, sanitizeRepositoryError(ctx, "load revision", loadErr)
		}
		if revision.ID != binding.RevisionID || revision.CollectionID != collection.ID {
			return runtimehost.ManagedDataResolution{}, invalidMetadata("revision relationship is invalid")
		}
		if revision.Status != manageddata.RevisionStatusReady {
			return runtimehost.ManagedDataResolution{}, ErrRevisionNotReady
		}
		manifest, manifestErr := validateRevisionManifest(revision)
		if manifestErr != nil {
			return runtimehost.ManagedDataResolution{}, manifestErr
		}
		files, loadErr := r.repository.ListRevisionFiles(ctx, revision.ID)
		if loadErr != nil {
			return runtimehost.ManagedDataResolution{}, sanitizeRepositoryError(ctx, "load revision files", loadErr)
		}
		if metadataErr := validateRevisionFiles(revision, manifest, files); metadataErr != nil {
			return runtimehost.ManagedDataResolution{}, metadataErr
		}
		resolved = append(resolved, resolvedBinding{
			project: collection.ProjectID, connection: collection.ConnectionName,
			manifestDigest: revision.Digest, manifest: manifest,
		})
	}

	sort.Slice(resolved, func(i, j int) bool {
		if resolved[i].project != resolved[j].project {
			return resolved[i].project < resolved[j].project
		}
		if resolved[i].connection != resolved[j].connection {
			return resolved[i].connection < resolved[j].connection
		}
		return resolved[i].manifestDigest < resolved[j].manifestDigest
	})

	roots := make(map[string]string, len(resolved))
	leases := make([]manageddata.RevisionLease, 0, len(resolved))
	for _, binding := range resolved {
		lease, materializeErr := r.materializer.MaterializeRevision(ctx, binding.manifestDigest, binding.manifest)
		if materializeErr != nil {
			_ = (&managedDataLifetime{leases: leases}).Release()
			return runtimehost.ManagedDataResolution{}, sanitizeMaterializationError(ctx, materializeErr)
		}
		if lease == nil || strings.TrimSpace(lease.Root()) == "" {
			_ = (&managedDataLifetime{leases: append(leases, lease)}).Release()
			return runtimehost.ManagedDataResolution{}, ErrMaterialization
		}
		leases = append(leases, lease)
		roots[binding.connection] = lease.Root()
	}
	return runtimehost.ManagedDataResolution{
		RevisionID: aggregateRevisionID(resolved), Roots: roots, Lifetime: &managedDataLifetime{leases: leases},
	}, nil
}

type managedDataLifetime struct {
	leases []manageddata.RevisionLease
	once   sync.Once
	err    error
}

func (l *managedDataLifetime) Release() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		for index := len(l.leases) - 1; index >= 0; index-- {
			if l.leases[index] != nil {
				l.err = errors.Join(l.err, l.leases[index].Release())
			}
		}
		l.leases = nil
	})
	return l.err
}

func (r *Resolver) loadBindings(ctx context.Context, servingStateID servingstate.ID) ([]manageddata.ServingStateBinding, error) {
	if !canonicalIdentifier(string(servingStateID)) {
		return nil, invalidMetadata("serving state id is invalid")
	}
	bindings, err := r.repository.ListServingStateBindings(ctx, string(servingStateID))
	if err != nil {
		return nil, sanitizeRepositoryError(ctx, "load serving state bindings", err)
	}
	return append([]manageddata.ServingStateBinding(nil), bindings...), nil
}

func validateRevisionManifest(revision manageddata.Revision) (manageddata.Manifest, error) {
	decoder := json.NewDecoder(strings.NewReader(revision.ManifestJSON))
	decoder.DisallowUnknownFields()
	var manifest manageddata.Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return manageddata.Manifest{}, invalidMetadata("stored manifest is invalid")
	}
	if err := requireJSONEnd(decoder); err != nil {
		return manageddata.Manifest{}, invalidMetadata("stored manifest is invalid")
	}
	canonical, err := manifest.CanonicalJSON()
	if err != nil {
		return manageddata.Manifest{}, invalidMetadata("stored manifest is invalid")
	}
	if !bytes.Equal(canonical, []byte(revision.ManifestJSON)) {
		return manageddata.Manifest{}, invalidMetadata("stored manifest is not canonical")
	}
	if revision.Digest != manifest.RevisionID() {
		return manageddata.Manifest{}, invalidMetadata("stored manifest digest does not match revision")
	}
	if revision.FileCount != int64(len(manifest.Files)) {
		return manageddata.Manifest{}, invalidMetadata("stored manifest file count does not match revision")
	}
	var size int64
	for _, file := range manifest.Files {
		size += file.Size
	}
	if revision.SizeBytes != size {
		return manageddata.Manifest{}, invalidMetadata("stored manifest size does not match revision")
	}
	return manifest, nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func validateRevisionFiles(revision manageddata.Revision, manifest manageddata.Manifest, files []manageddata.RevisionFile) error {
	if len(files) != len(manifest.Files) {
		return invalidMetadata("revision file count does not match manifest")
	}
	expected := make(map[string]manageddata.File, len(manifest.Files))
	for _, file := range manifest.Files {
		expected[file.Path] = file
	}
	seen := make(map[string]struct{}, len(files))
	for _, stored := range files {
		if stored.RevisionID != revision.ID || strings.TrimSpace(stored.StorageKey) == "" {
			return invalidMetadata("revision file relationship is invalid")
		}
		if _, duplicate := seen[stored.Path]; duplicate {
			return invalidMetadata("revision contains duplicate file metadata")
		}
		seen[stored.Path] = struct{}{}
		want, ok := expected[stored.Path]
		if !ok || stored.File != want {
			return invalidMetadata("revision file metadata does not match manifest")
		}
	}
	return nil
}

type aggregateBinding struct {
	Project        string `json:"project"`
	Connection     string `json:"connection"`
	ManifestDigest string `json:"manifest_digest"`
}

func aggregateRevisionID(bindings []resolvedBinding) string {
	payload := make([]aggregateBinding, 0, len(bindings))
	for _, binding := range bindings {
		payload = append(payload, aggregateBinding{
			Project: binding.project, Connection: binding.connection, ManifestDigest: binding.manifestDigest,
		})
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		panic("marshal managed-data aggregate: " + err.Error())
	}
	digest := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func sanitizeRepositoryError(ctx context.Context, operation string, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(err, manageddata.ErrNotFound) {
		return fmt.Errorf("%w: %s returned no record", ErrInvalidMetadata, operation)
	}
	return fmt.Errorf("%w: %s failed", ErrRepository, operation)
}

func sanitizeMaterializationError(ctx context.Context, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return ErrMaterialization
}

func invalidMetadata(reason string) error {
	return fmt.Errorf("%w: %s", ErrInvalidMetadata, reason)
}

func canonicalIdentifier(value string) bool {
	return value != "" && strings.TrimSpace(value) == value
}

func validAuthoredIdentity(value string) bool {
	if !canonicalIdentifier(value) {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

var _ runtimehost.ManagedDataResolver = (*Resolver)(nil)
