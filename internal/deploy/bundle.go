package deploy

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/semantic"
)

const (
	BundleFormat = "tar.gz"
	CatalogFile  = "catalog.yaml"
)

type Manifest struct {
	Version        int            `json:"version"`
	WorkspaceID    string         `json:"workspaceId"`
	WorkspaceTitle string         `json:"workspaceTitle"`
	CatalogPath    string         `json:"catalogPath"`
	Files          []ManifestFile `json:"files"`
	SemanticModels []string       `json:"semanticModels"`
	Dashboards     []string       `json:"dashboards"`
}

type ManifestFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Validation struct {
	Manifest     Manifest
	ManifestJSON string
	Digest       string
	CatalogPath  string
	RootDir      string
	Assets       []platform.Asset
	Edges        []platform.AssetEdge
}

func PackCatalog(catalogPath string, out io.Writer) (Manifest, string, error) {
	catalogPath, err := filepath.Abs(catalogPath)
	if err != nil {
		return Manifest{}, "", err
	}
	workspace, err := semantic.LoadWorkspace(catalogPath)
	if err != nil {
		return Manifest{}, "", err
	}
	baseDir := filepath.Dir(catalogPath)
	relFiles := []string{CatalogFile}
	for _, model := range workspace.Catalog.SemanticModels {
		relFiles = append(relFiles, cleanBundlePath(model.Path))
	}
	for _, report := range workspace.Catalog.Dashboards {
		relFiles = append(relFiles, cleanBundlePath(report.Path))
	}
	sort.Strings(relFiles[1:])

	hash := sha256.New()
	mw := io.MultiWriter(out, hash)
	gz := gzip.NewWriter(mw)
	tw := tar.NewWriter(gz)
	manifest := Manifest{
		Version:        1,
		WorkspaceID:    workspaceID(workspace.Catalog.Workspace.ID),
		WorkspaceTitle: workspaceTitle(workspace.Catalog.Workspace.Title),
		CatalogPath:    CatalogFile,
		Files:          make([]ManifestFile, 0, len(relFiles)),
	}
	for _, model := range workspace.Catalog.SemanticModels {
		manifest.SemanticModels = append(manifest.SemanticModels, model.ID)
	}
	for _, report := range workspace.Catalog.Dashboards {
		manifest.Dashboards = append(manifest.Dashboards, report.ID)
	}

	seen := map[string]struct{}{}
	for _, rel := range relFiles {
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		sourcePath := filepath.Join(baseDir, rel)
		if rel == CatalogFile {
			sourcePath = catalogPath
		}
		info, err := os.Stat(sourcePath)
		if err != nil {
			return Manifest{}, "", err
		}
		if info.IsDir() {
			return Manifest{}, "", fmt.Errorf("bundle path %s is a directory", rel)
		}
		bytes, err := os.ReadFile(sourcePath)
		if err != nil {
			return Manifest{}, "", err
		}
		fileHash := sha256.Sum256(bytes)
		manifest.Files = append(manifest.Files, ManifestFile{
			Path:   rel,
			SHA256: hex.EncodeToString(fileHash[:]),
			Size:   info.Size(),
		})
		if err := tw.WriteHeader(&tar.Header{
			Name: rel,
			Mode: 0o644,
			Size: int64(len(bytes)),
		}); err != nil {
			return Manifest{}, "", err
		}
		if _, err := tw.Write(bytes); err != nil {
			return Manifest{}, "", err
		}
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, "", err
	}
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len(manifestBytes))}); err != nil {
		return Manifest{}, "", err
	}
	if _, err := tw.Write(manifestBytes); err != nil {
		return Manifest{}, "", err
	}
	if err := tw.Close(); err != nil {
		return Manifest{}, "", err
	}
	if err := gz.Close(); err != nil {
		return Manifest{}, "", err
	}
	return manifest, hex.EncodeToString(hash.Sum(nil)), nil
}

func ValidateArtifact(path, workspaceID, deploymentID string) (Validation, error) {
	digest, err := fileDigest(path)
	if err != nil {
		return Validation{}, err
	}
	root, err := os.MkdirTemp("", "libredash-deploy-*")
	if err != nil {
		return Validation{}, err
	}
	if err := ExtractArtifact(path, root); err != nil {
		os.RemoveAll(root)
		return Validation{}, err
	}
	manifest, err := readManifest(root)
	if err != nil {
		os.RemoveAll(root)
		return Validation{}, err
	}
	if err := validateManifestFiles(root, manifest); err != nil {
		os.RemoveAll(root)
		return Validation{}, err
	}
	if workspaceID == "" {
		workspaceID = workspaceIDOrDefault(manifest.WorkspaceID)
	}
	catalogPath := filepath.Join(root, manifest.CatalogPath)
	workspace, err := semantic.LoadWorkspace(catalogPath)
	if err != nil {
		os.RemoveAll(root)
		return Validation{}, err
	}
	assets, edges, err := ExtractAssets(workspaceID, deploymentID, workspace)
	if err != nil {
		os.RemoveAll(root)
		return Validation{}, err
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		os.RemoveAll(root)
		return Validation{}, err
	}
	return Validation{
		Manifest:     manifest,
		ManifestJSON: string(manifestJSON),
		Digest:       digest,
		CatalogPath:  catalogPath,
		RootDir:      root,
		Assets:       assets,
		Edges:        edges,
	}, nil
}

func ExtractArtifact(path, dest string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		rel, err := safeBundlePath(header.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) && filepath.Clean(target) != filepath.Clean(dest) {
			return fmt.Errorf("bundle path %q escapes destination", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported bundle entry %q", header.Name)
		}
	}
}

func ExtractAssets(workspaceID, deploymentID string, workspace *semantic.Workspace) ([]platform.Asset, []platform.AssetEdge, error) {
	assets := []platform.Asset{}
	edges := []platform.AssetEdge{}
	byKey := map[string]string{}
	add := func(typ, key, parentID, title, description string, content any) (string, error) {
		asset, err := platform.NewAsset(workspaceID, deploymentID, typ, key, parentID, title, description, content)
		if err != nil {
			return "", err
		}
		assets = append(assets, asset)
		byKey[typ+":"+key] = asset.ID
		return asset.ID, nil
	}
	edge := func(fromID, toID, typ string) {
		if fromID == "" || toID == "" {
			return
		}
		edges = append(edges, platform.NewAssetEdge(workspaceID, deploymentID, fromID, toID, typ))
	}

	catalogID, err := add("catalog", workspaceID, "", workspaceTitle(workspace.Catalog.Workspace.Title), workspace.Catalog.Workspace.Description, workspace.Catalog)
	if err != nil {
		return nil, nil, err
	}
	for _, modelEntry := range workspace.Catalog.SemanticModels {
		model := workspace.Models[modelEntry.ID]
		modelID, err := add("semantic_model", modelEntry.ID, catalogID, modelEntry.Title, modelEntry.Description, model)
		if err != nil {
			return nil, nil, err
		}
		edge(catalogID, modelID, "contains")
		for sourceName, source := range model.Sources {
			id, err := add("source", modelEntry.ID+"."+sourceName, modelID, sourceName, source.File, source)
			if err != nil {
				return nil, nil, err
			}
			edge(modelID, id, "contains")
		}
		for cacheName, cache := range model.Cache.Tables {
			id, err := add("cache_table", modelEntry.ID+"."+cacheName, modelID, cacheName, cache.Description, cache)
			if err != nil {
				return nil, nil, err
			}
			edge(modelID, id, "contains")
			for sourceName := range model.Sources {
				edge(id, byKey["source:"+modelEntry.ID+"."+sourceName], "reads_source")
			}
		}
		for datasetName, dataset := range model.Datasets {
			datasetID, err := add("dataset", modelEntry.ID+"."+datasetName, modelID, datasetName, "", dataset)
			if err != nil {
				return nil, nil, err
			}
			edge(datasetID, byKey["cache_table:"+modelEntry.ID+"."+dataset.Source], "uses_cache_table")
			for dimensionName, dimension := range dataset.Dimensions {
				id, err := add("dimension", modelEntry.ID+"."+datasetName+"."+dimensionName, datasetID, dimensionLabel(dimensionName, dimension.Label), "", dimension)
				if err != nil {
					return nil, nil, err
				}
				edge(datasetID, id, "contains")
			}
			for measureName, measure := range dataset.Measures {
				id, err := add("measure", modelEntry.ID+"."+datasetName+"."+measureName, datasetID, measureLabel(measureName, measure.Label), "", measure)
				if err != nil {
					return nil, nil, err
				}
				edge(datasetID, id, "contains")
			}
		}
	}
	for _, reportEntry := range workspace.Catalog.Dashboards {
		report := workspace.Dashboards[reportEntry.ID]
		reportID, err := add("dashboard", reportEntry.ID, catalogID, reportEntry.Title, reportEntry.Description, report)
		if err != nil {
			return nil, nil, err
		}
		edge(reportID, byKey["semantic_model:"+reportEntry.SemanticModel], "uses_model")
		for _, page := range report.Pages {
			pageID, err := add("page", report.ID+"."+page.ID, reportID, page.Title, page.Description, page)
			if err != nil {
				return nil, nil, err
			}
			edge(reportID, pageID, "contains")
		}
		for filterName, filter := range report.Filters {
			filterID, err := add("filter", report.ID+"."+filterName, reportID, filter.Label, "", filter)
			if err != nil {
				return nil, nil, err
			}
			edge(filterID, byKey["dimension:"+report.SemanticModel+"."+filter.Dataset+"."+filter.Dimension], "filters_dimension")
		}
		for kpiName, kpi := range report.KPIs {
			kpiID, err := add("kpi", report.ID+"."+kpiName, reportID, kpi.Title, kpi.Note, kpi)
			if err != nil {
				return nil, nil, err
			}
			edge(kpiID, byKey["measure:"+report.SemanticModel+"."+kpi.Dataset+"."+kpi.Measure], "uses_measure")
		}
		for visualName, visual := range report.Visuals {
			visualID, err := add("visual", report.ID+"."+visualName, reportID, visual.Title, "", visual)
			if err != nil {
				return nil, nil, err
			}
			edge(visualID, byKey["dataset:"+report.SemanticModel+"."+visual.Dataset], "uses_dataset")
			for _, measure := range visual.Query.Measures {
				edge(visualID, byKey["measure:"+report.SemanticModel+"."+visual.Dataset+"."+measure], "uses_measure")
			}
			for _, dimension := range visual.Query.Dimensions {
				edge(visualID, byKey["dimension:"+report.SemanticModel+"."+visual.Dataset+"."+dimension], "uses_dimension")
			}
			if visual.Query.Series != "" {
				edge(visualID, byKey["dimension:"+report.SemanticModel+"."+visual.Dataset+"."+visual.Query.Series], "uses_dimension")
			}
		}
		for tableName, table := range report.Tables {
			tableID, err := add("table", report.ID+"."+tableName, reportID, table.Title, "", table)
			if err != nil {
				return nil, nil, err
			}
			edge(tableID, byKey["dataset:"+report.SemanticModel+"."+table.Dataset], "uses_dataset")
		}
	}
	return assets, edges, nil
}

func readManifest(root string) (Manifest, error) {
	bytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(bytes, &manifest); err != nil {
		return Manifest{}, err
	}
	if manifest.CatalogPath == "" {
		manifest.CatalogPath = CatalogFile
	}
	return manifest, nil
}

func validateManifestFiles(root string, manifest Manifest) error {
	for _, file := range manifest.Files {
		path := filepath.Join(root, cleanBundlePath(file.Path))
		bytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(bytes)
		if got := hex.EncodeToString(sum[:]); got != file.SHA256 {
			return fmt.Errorf("file %s digest mismatch", file.Path)
		}
	}
	return nil
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func cleanBundlePath(path string) string {
	path = filepath.ToSlash(filepath.Clean(path))
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimPrefix(path, "../")
	return path
}

func safeBundlePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("bundle path %q must be relative", path)
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." || clean == "" {
		return "", fmt.Errorf("bundle path %q is empty", path)
	}
	for _, part := range strings.Split(clean, "/") {
		if part == ".." {
			return "", fmt.Errorf("bundle path %q escapes bundle root", path)
		}
	}
	return clean, nil
}

func workspaceID(value string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return platform.DefaultWorkspaceID
}

func workspaceIDOrDefault(value string) string {
	return workspaceID(value)
}

func workspaceTitle(value string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return "LibreDash Workspace"
}

func dimensionLabel(name, label string) string {
	if label != "" {
		return label
	}
	return name
}

func measureLabel(name, label string) string {
	if label != "" {
		return label
	}
	return name
}
