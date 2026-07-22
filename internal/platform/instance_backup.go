package platform

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Yacobolo/leapview/internal/securefs"
	securejoin "github.com/cyphar/filepath-securejoin"
)

const (
	instanceBackupManifestName = "leapview-backup.json"
	instanceBackupDBName       = "leapview.db"
	instanceBackupVersion      = 1
	instanceRestoreDirMode     = securefs.PrivateDirMode
	instanceRestoreFileMode    = securefs.PrivateFileMode
	instanceRestoreDBMode      = securefs.PrivateFileMode
)

type InstanceBackupOptions struct {
	HomeDir string
	DBPath  string
	OutPath string
}

type InstanceRestoreOptions struct {
	TargetHomeDir        string
	BackupPath           string
	CurrentBackupOut     string
	ExpectedEnvironment  string
	PreserveRelativeFile string
}

type instanceBackupManifest struct {
	Version   int       `json:"version"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"createdAt"`
	DBPath    string    `json:"dbPath"`
}

func BackupInstance(ctx context.Context, options InstanceBackupOptions) error {
	outPath := strings.TrimSpace(options.OutPath)
	if outPath == "" {
		return fmt.Errorf("instance backup output path is required")
	}
	outAbs, err := filepath.Abs(outPath)
	if err != nil {
		return err
	}
	homeAbs, _, err := validateInstanceBackupSource(options.HomeDir, options.DBPath)
	if err != nil {
		return err
	}
	if pathWithin(homeAbs, outAbs) {
		return fmt.Errorf("instance backup output path must not be inside home dir")
	}
	if _, err := os.Stat(outAbs); err == nil {
		return fmt.Errorf("instance backup output path %q already exists", outPath)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outAbs), 0o755); err != nil {
		return err
	}
	tmpArchive, err := os.CreateTemp(filepath.Dir(outAbs), ".leapview-instance-backup-*.tar.gz")
	if err != nil {
		return err
	}
	tmpArchivePath := tmpArchive.Name()
	cleanupArchive := true
	defer func() {
		if cleanupArchive {
			_ = os.Remove(tmpArchivePath)
		}
	}()
	if err := writeInstanceBackup(ctx, options.HomeDir, options.DBPath, tmpArchive); err != nil {
		_ = tmpArchive.Close()
		return err
	}
	if err := tmpArchive.Sync(); err != nil {
		_ = tmpArchive.Close()
		return err
	}
	if err := tmpArchive.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpArchivePath, outAbs); err != nil {
		return err
	}
	cleanupArchive = false
	return nil
}

// BackupInstanceToWriter writes a validated full-instance archive directly to
// out. Callers own atomic destination handling and must stop the serving process.
func BackupInstanceToWriter(ctx context.Context, homeDir, dbPath string, out io.Writer) error {
	if out == nil {
		return fmt.Errorf("instance backup output is required")
	}
	return writeInstanceBackup(ctx, homeDir, dbPath, out)
}

func writeInstanceBackup(ctx context.Context, homeDir, dbPath string, out io.Writer) error {
	homeAbs, dbAbs, err := validateInstanceBackupSource(homeDir, dbPath)
	if err != nil {
		return err
	}
	parent := filepath.Dir(homeAbs)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp(parent, ".leapview-instance-backup-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	dbCopy := filepath.Join(tmpDir, instanceBackupDBName)
	store, err := Open(ctx, dbAbs)
	if err != nil {
		return err
	}
	if err := store.Backup(ctx, dbCopy); err != nil {
		_ = store.Close()
		return err
	}
	if err := store.Close(); err != nil {
		return err
	}

	gzw := gzip.NewWriter(out)
	tw := tar.NewWriter(gzw)
	manifest := instanceBackupManifest{
		Version:   instanceBackupVersion,
		Kind:      "leapview-instance",
		CreatedAt: time.Now().UTC(),
		DBPath:    instanceBackupDBName,
	}
	if err := addJSONToTar(tw, instanceBackupManifestName, manifest); err != nil {
		_ = closeArchiveStreamWriters(tw, gzw)
		return err
	}
	if err := addFileToTar(tw, dbCopy, instanceBackupDBName); err != nil {
		_ = closeArchiveStreamWriters(tw, gzw)
		return err
	}
	if err := filepath.WalkDir(homeAbs, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		pathAbs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if samePath(pathAbs, homeAbs) {
			return nil
		}
		if samePath(pathAbs, dbAbs) || samePath(pathAbs, dbAbs+"-wal") || samePath(pathAbs, dbAbs+"-shm") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(homeAbs, pathAbs)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || rel == "" {
			return nil
		}
		if rel == instanceBackupManifestName {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = rel
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(pathAbs)
			if err != nil {
				return err
			}
			if err := validateInstanceBackupSymlink(rel, target); err != nil {
				return err
			}
			return fmt.Errorf("instance backup symlink entries are not supported: %s", rel)
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(pathAbs)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}); err != nil {
		_ = closeArchiveStreamWriters(tw, gzw)
		return err
	}
	return closeArchiveStreamWriters(tw, gzw)
}

func validateInstanceBackupSource(homeDir, dbPath string) (string, string, error) {
	homeDir = strings.TrimSpace(homeDir)
	dbPath = strings.TrimSpace(dbPath)
	if homeDir == "" {
		return "", "", fmt.Errorf("instance backup home dir is required")
	}
	if dbPath == "" {
		return "", "", fmt.Errorf("instance backup database path is required")
	}
	homeAbs, err := filepath.Abs(homeDir)
	if err != nil {
		return "", "", err
	}
	dbAbs, err := filepath.Abs(dbPath)
	if err != nil {
		return "", "", err
	}
	if !pathWithin(homeAbs, dbAbs) {
		return "", "", fmt.Errorf("instance backup database path must be inside home dir")
	}
	return homeAbs, dbAbs, nil
}

func RestoreInstance(ctx context.Context, options InstanceRestoreOptions) error {
	backupPath := strings.TrimSpace(options.BackupPath)
	if backupPath == "" {
		return fmt.Errorf("instance restore backup path is required")
	}
	targetHome := strings.TrimSpace(options.TargetHomeDir)
	if targetHome == "" {
		return fmt.Errorf("instance restore target home dir is required")
	}
	targetAbs, err := filepath.Abs(targetHome)
	if err != nil {
		return err
	}
	backupAbs, err := filepath.Abs(backupPath)
	if err != nil {
		return err
	}
	if pathWithin(targetAbs, backupAbs) {
		return fmt.Errorf("instance restore backup path must not be inside target home dir")
	}
	file, err := os.Open(backupAbs)
	if err != nil {
		return err
	}
	defer file.Close()
	return restoreInstanceFromReader(ctx, options, file)
}

// RestoreInstanceFromReader validates and restores a full-instance archive
// directly from in. The target is replaced only after extraction succeeds.
func RestoreInstanceFromReader(ctx context.Context, options InstanceRestoreOptions, in io.Reader) error {
	if in == nil {
		return fmt.Errorf("instance restore input is required")
	}
	return restoreInstanceFromReader(ctx, options, in)
}

func restoreInstanceFromReader(ctx context.Context, options InstanceRestoreOptions, in io.Reader) error {
	targetHome := strings.TrimSpace(options.TargetHomeDir)
	currentBackupOut := strings.TrimSpace(options.CurrentBackupOut)
	preserveRelativeFile, err := validatePreservedRelativeFile(options.PreserveRelativeFile)
	if err != nil {
		return err
	}
	if targetHome == "" {
		return fmt.Errorf("instance restore target home dir is required")
	}
	targetAbs, err := filepath.Abs(targetHome)
	if err != nil {
		return err
	}
	if currentBackupOut != "" {
		currentBackupAbs, err := filepath.Abs(currentBackupOut)
		if err != nil {
			return err
		}
		if pathWithin(targetAbs, currentBackupAbs) {
			return fmt.Errorf("current instance backup path must not be inside target home dir")
		}
	}

	exists, nonEmpty, err := dirExistsNonEmptyExcept(targetAbs, preserveRelativeFile)
	if err != nil {
		return err
	}
	parent := filepath.Dir(targetAbs)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmpRestore, err := os.MkdirTemp(parent, ".leapview-restore-*")
	if err != nil {
		return err
	}
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.RemoveAll(tmpRestore)
		}
	}()
	if err := extractInstanceBackupReader(ctx, in, tmpRestore); err != nil {
		return err
	}
	if environment := strings.TrimSpace(options.ExpectedEnvironment); environment != "" {
		restored, err := Open(ctx, filepath.Join(tmpRestore, instanceBackupDBName))
		if err != nil {
			return fmt.Errorf("open restored instance environment: %w", err)
		}
		bindErr := restored.BindInstanceEnvironment(ctx, environment)
		closeErr := restored.Close()
		if bindErr != nil {
			return fmt.Errorf("validate restored instance environment: %w", bindErr)
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if exists && nonEmpty {
		if currentBackupOut == "" {
			return fmt.Errorf("current instance backup path is required when restoring over an existing home dir")
		}
		if err := BackupInstance(ctx, InstanceBackupOptions{
			HomeDir: targetAbs,
			DBPath:  filepath.Join(targetAbs, instanceBackupDBName),
			OutPath: currentBackupOut,
		}); err != nil {
			return fmt.Errorf("backup current instance: %w", err)
		}
	}
	if preserveRelativeFile != "" {
		if err := preserveFileAcrossRestore(targetAbs, tmpRestore, preserveRelativeFile); err != nil {
			return err
		}
	}

	oldTarget := ""
	if exists {
		oldTarget = filepath.Join(parent, ".leapview-restore-old-"+time.Now().UTC().Format("20060102150405.000000000"))
		if err := os.Rename(targetAbs, oldTarget); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpRestore, targetAbs); err != nil {
		if oldTarget != "" {
			_ = os.Rename(oldTarget, targetAbs)
		}
		return err
	}
	cleanupTmp = false
	if oldTarget != "" {
		_ = os.RemoveAll(oldTarget)
	}
	return nil
}

func validatePreservedRelativeFile(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	clean := filepath.Clean(value)
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("preserved restore file must be a relative path inside the instance home")
	}
	return clean, nil
}

func preserveFileAcrossRestore(currentHome, restoredHome, relativePath string) error {
	currentPath := filepath.Join(currentHome, relativePath)
	info, err := os.Lstat(currentPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("preserved restore path %q is not a regular file", relativePath)
	}
	restoredPath := filepath.Join(restoredHome, relativePath)
	if err := os.Remove(restoredPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(restoredPath), instanceRestoreDirMode); err != nil {
		return err
	}
	if err := os.Link(currentPath, restoredPath); err != nil {
		return fmt.Errorf("preserve restore file %q: %w", relativePath, err)
	}
	return nil
}

func dirExistsNonEmptyExcept(path, ignoredRelativeFile string) (bool, bool, error) {
	exists, _, err := dirExistsNonEmpty(path)
	if err != nil || !exists {
		return exists, false, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, false, err
	}
	for _, entry := range entries {
		if ignoredRelativeFile != "" && entry.Name() == ignoredRelativeFile {
			continue
		}
		return true, true, nil
	}
	return true, false, nil
}

func extractInstanceBackupReader(ctx context.Context, archive io.Reader, targetDir string) error {
	gzr, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("open instance backup gzip: %w", err)
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	seenManifest := false
	seenDB := false
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read instance backup: %w", err)
		}
		name := filepath.Clean(filepath.FromSlash(header.Name))
		if name == "." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) || name == ".." {
			return fmt.Errorf("instance backup contains unsafe path %q", header.Name)
		}
		if strings.HasPrefix(filepath.ToSlash(name), ".leapview-restore-old-") {
			return fmt.Errorf("instance backup contains reserved path %q", header.Name)
		}
		target, err := securejoin.SecureJoin(targetDir, name)
		if err != nil {
			return fmt.Errorf("resolve instance backup path %q: %w", header.Name, err)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, instanceRestoreDirMode); err != nil {
				return err
			}
			if err := os.Chmod(target, instanceRestoreDirMode); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), instanceRestoreDirMode); err != nil {
				return err
			}
			fileMode := instanceRestoreModeForFile(header.Name)
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			if err := os.Chmod(target, fileMode); err != nil {
				return err
			}
		case tar.TypeSymlink:
			return fmt.Errorf("instance backup symlink entries are not supported: %s", header.Name)
		default:
			return fmt.Errorf("instance backup contains unsupported entry %q", header.Name)
		}
		if header.Name == instanceBackupManifestName {
			seenManifest = true
			if err := validateInstanceBackupManifest(target); err != nil {
				return err
			}
		}
		if header.Name == instanceBackupDBName {
			seenDB = true
		}
	}
	if !seenManifest {
		return fmt.Errorf("instance backup is missing %s", instanceBackupManifestName)
	}
	if !seenDB {
		return fmt.Errorf("instance backup is missing %s", instanceBackupDBName)
	}
	return validateBackupDatabase(ctx, filepath.Join(targetDir, instanceBackupDBName))
}

func instanceRestoreModeForFile(name string) os.FileMode {
	if filepath.ToSlash(filepath.Clean(name)) == instanceBackupDBName {
		return instanceRestoreDBMode
	}
	return instanceRestoreFileMode
}

func validateInstanceBackupSymlink(name, linkname string) error {
	cleanLink := path.Clean(filepath.ToSlash(linkname))
	if filepath.IsAbs(linkname) || cleanLink == ".." || strings.HasPrefix(cleanLink, "../") {
		return fmt.Errorf("instance backup contains unsafe symlink %q", name)
	}
	return nil
}

func validateInstanceBackupManifest(path string) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var manifest instanceBackupManifest
	if err := json.Unmarshal(bytes, &manifest); err != nil {
		return fmt.Errorf("read instance backup manifest: %w", err)
	}
	if manifest.Kind != "leapview-instance" {
		return fmt.Errorf("instance backup manifest kind = %q", manifest.Kind)
	}
	if manifest.Version != instanceBackupVersion {
		return fmt.Errorf("unsupported instance backup version %d", manifest.Version)
	}
	if manifest.DBPath != instanceBackupDBName {
		return fmt.Errorf("instance backup manifest database path = %q", manifest.DBPath)
	}
	return nil
}

func addJSONToTar(tw *tar.Writer, name string, value any) error {
	bytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	bytes = append(bytes, '\n')
	header := &tar.Header{
		Name:    name,
		Mode:    0o644,
		Size:    int64(len(bytes)),
		ModTime: time.Now().UTC(),
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err = tw.Write(bytes)
	return err
}

func addFileToTar(tw *tar.Writer, sourcePath, name string) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = name
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(tw, file)
	return err
}

func closeArchiveStreamWriters(tw *tar.Writer, gzw *gzip.Writer) error {
	if err := tw.Close(); err != nil {
		_ = gzw.Close()
		return err
	}
	if err := gzw.Close(); err != nil {
		return err
	}
	return nil
}

func dirExistsNonEmpty(path string) (bool, bool, error) {
	entries, err := os.ReadDir(path)
	if err == nil {
		return true, len(entries) > 0, nil
	}
	if os.IsNotExist(err) {
		return false, false, nil
	}
	return false, false, err
}

func pathWithin(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if samePath(parent, child) {
		return true
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
