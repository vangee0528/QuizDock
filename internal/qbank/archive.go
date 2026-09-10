package qbank

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxArchiveSize     = int64(512 << 20)
	maxUncompressed    = int64(1 << 30)
	maxEntrySize       = int64(64 << 20)
	maxEntryCount      = 50_000
	maximumQuestionLen = int64(2 << 20)
)

var allowedAssetExtensions = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp",
}

func Load(filename string) (*Package, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if stat.Size() > maxArchiveSize {
		return nil, fmt.Errorf("archive exceeds %d MiB", maxArchiveSize>>20)
	}
	return Read(file, stat.Size())
}

func Read(reader io.ReaderAt, size int64) (*Package, error) {
	if size <= 0 || size > maxArchiveSize {
		return nil, fmt.Errorf("invalid archive size")
	}
	archive, err := zip.NewReader(reader, size)
	if err != nil {
		return nil, fmt.Errorf("invalid ZIP archive: %w", err)
	}
	if len(archive.File) == 0 || len(archive.File) > maxEntryCount {
		return nil, fmt.Errorf("archive contains an invalid number of files")
	}
	entries := make(map[string][]byte)
	var total int64
	for _, item := range archive.File {
		name, err := safePath(item.Name)
		if err != nil {
			return nil, err
		}
		if name == "" || strings.HasSuffix(item.Name, "/") {
			continue
		}
		if item.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symlinks are not allowed: %s", name)
		}
		if item.Flags&0x1 != 0 {
			return nil, fmt.Errorf("encrypted entries are not allowed: %s", name)
		}
		if _, exists := entries[name]; exists {
			return nil, fmt.Errorf("duplicate archive path: %s", name)
		}
		entrySize := int64(item.UncompressedSize64)
		if entrySize > maxEntrySize || total+entrySize > maxUncompressed {
			return nil, fmt.Errorf("archive expands beyond the allowed size")
		}
		if strings.HasPrefix(name, "questions/") && entrySize > maximumQuestionLen {
			return nil, fmt.Errorf("question is too large: %s", name)
		}
		data, err := readZipEntry(item, entrySize)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		entries[name] = data
		total += int64(len(data))
	}
	manifestBytes, ok := entries["manifest.json"]
	if !ok {
		return nil, errors.New("manifest.json is missing")
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("invalid manifest.json: %w", err)
	}
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}
	if checksums, ok := entries["checksums.sha256"]; ok {
		if err := verifyChecksums(checksums, entries); err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("checksums.sha256 is missing")
	}
	questionPaths := make([]string, 0)
	assetPaths := make([]string, 0)
	for name := range entries {
		switch {
		case strings.HasPrefix(name, "questions/") && strings.HasSuffix(strings.ToLower(name), ".md"):
			questionPaths = append(questionPaths, name)
		case strings.HasPrefix(name, "assets/"):
			assetPaths = append(assetPaths, name)
		case name != "manifest.json" && name != "checksums.sha256":
			return nil, fmt.Errorf("unsupported archive path: %s", name)
		}
	}
	sort.Strings(questionPaths)
	sort.Strings(assetPaths)
	questions := make([]Question, 0, len(questionPaths))
	questionIDs := make(map[string]bool)
	for _, name := range questionPaths {
		question, err := ParseMarkdown(name, entries[name])
		if err != nil {
			return nil, err
		}
		if questionIDs[question.ID] {
			return nil, fmt.Errorf("duplicate question id: %s", question.ID)
		}
		questionIDs[question.ID] = true
		questions = append(questions, question)
	}
	if manifest.QuestionCount != len(questions) {
		return nil, fmt.Errorf("manifest question_count is %d, archive contains %d", manifest.QuestionCount, len(questions))
	}
	assets := make([]Asset, 0, len(assetPaths))
	for _, name := range assetPaths {
		extension := strings.ToLower(path.Ext(name))
		mimeType, ok := allowedAssetExtensions[extension]
		if !ok {
			return nil, fmt.Errorf("unsupported asset type: %s", name)
		}
		digest := sha256.Sum256(entries[name])
		assets = append(assets, Asset{Path: name, MIME: mimeType, SHA256: hex.EncodeToString(digest[:]), Data: entries[name]})
	}
	return &Package{Manifest: manifest, Questions: questions, Assets: assets, ArchiveHash: hashEntries(entries), LoadedAt: time.Now().UTC()}, nil
}

func Pack(sourceDirectory, destination string) (Summary, error) {
	root, err := filepath.Abs(sourceDirectory)
	if err != nil {
		return Summary{}, err
	}
	manifestPath := filepath.Join(root, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return Summary{}, fmt.Errorf("read manifest.json: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return Summary{}, fmt.Errorf("invalid manifest.json: %w", err)
	}
	if err := validateManifest(manifest); err != nil {
		return Summary{}, err
	}
	entries := map[string][]byte{"manifest.json": manifestData}
	for _, directory := range []string{"questions", "assets"} {
		base := filepath.Join(root, directory)
		stat, statErr := os.Stat(base)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) && directory == "assets" {
				continue
			}
			return Summary{}, statErr
		}
		if !stat.IsDir() {
			return Summary{}, fmt.Errorf("%s is not a directory", directory)
		}
		err = filepath.WalkDir(base, func(filename string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() {
				return walkErr
			}
			relative, relErr := filepath.Rel(root, filename)
			if relErr != nil {
				return relErr
			}
			name := filepath.ToSlash(relative)
			if _, safeErr := safePath(name); safeErr != nil {
				return safeErr
			}
			data, readErr := os.ReadFile(filename)
			if readErr != nil {
				return readErr
			}
			entries[name] = data
			return nil
		})
		if err != nil {
			return Summary{}, err
		}
	}
	questionCount := 0
	assetCount := 0
	for name, data := range entries {
		if strings.HasPrefix(name, "questions/") && strings.HasSuffix(strings.ToLower(name), ".md") {
			if _, err := ParseMarkdown(name, data); err != nil {
				return Summary{}, err
			}
			questionCount++
		}
		if strings.HasPrefix(name, "assets/") {
			if _, ok := allowedAssetExtensions[strings.ToLower(path.Ext(name))]; !ok {
				return Summary{}, fmt.Errorf("unsupported asset type: %s", name)
			}
			assetCount++
		}
	}
	if questionCount != manifest.QuestionCount {
		return Summary{}, fmt.Errorf("manifest question_count is %d, directory contains %d", manifest.QuestionCount, questionCount)
	}
	entries["checksums.sha256"] = checksumFile(entries)
	if err := writeArchive(destination, entries); err != nil {
		return Summary{}, err
	}
	loaded, err := Load(destination)
	if err != nil {
		return Summary{}, fmt.Errorf("verify generated package: %w", err)
	}
	return Summary{Manifest: manifest, Questions: questionCount, Assets: assetCount, ArchiveHash: loaded.ArchiveHash}, nil
}

func safePath(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	cleaned := path.Clean(name)
	if cleaned == "." {
		return "", nil
	}
	if strings.HasPrefix(cleaned, "/") || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.ContainsRune(cleaned, '\x00') {
		return "", fmt.Errorf("unsafe archive path: %s", name)
	}
	return cleaned, nil
}

func readZipEntry(item *zip.File, expected int64) ([]byte, error) {
	reader, err := item.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, expected+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != expected {
		return nil, fmt.Errorf("declared and actual sizes differ")
	}
	return data, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version %d", manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.ID) == "" || strings.TrimSpace(manifest.Name) == "" || strings.TrimSpace(manifest.Version) == "" {
		return errors.New("manifest id, name and version are required")
	}
	validID := true
	for _, character := range manifest.ID {
		if !(character >= 'a' && character <= 'z') && !(character >= '0' && character <= '9') && character != '.' && character != '-' {
			validID = false
		}
	}
	if !validID || len(manifest.ID) > 120 {
		return errors.New("manifest id must use lowercase letters, digits, dots and hyphens")
	}
	if manifest.QuestionCount < 0 || manifest.QuestionCount > maxEntryCount {
		return errors.New("manifest question_count is invalid")
	}
	return nil
}

func verifyChecksums(checksumData []byte, entries map[string][]byte) error {
	expected := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(checksumData))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 || len(parts[0]) != 64 {
			return fmt.Errorf("invalid checksum line")
		}
		name, err := safePath(parts[1])
		if err != nil {
			return err
		}
		expected[name] = strings.ToLower(parts[0])
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for name, data := range entries {
		if name == "checksums.sha256" {
			continue
		}
		want, ok := expected[name]
		if !ok {
			return fmt.Errorf("checksum is missing for %s", name)
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != want {
			return fmt.Errorf("checksum mismatch for %s", name)
		}
		delete(expected, name)
	}
	if len(expected) != 0 {
		return fmt.Errorf("checksum list contains files not present in archive")
	}
	return nil
}

func checksumFile(entries map[string][]byte) []byte {
	names := make([]string, 0, len(entries))
	for name := range entries {
		if name != "checksums.sha256" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var builder strings.Builder
	for _, name := range names {
		digest := sha256.Sum256(entries[name])
		fmt.Fprintf(&builder, "%x  %s\n", digest, name)
	}
	return []byte(builder.String())
}

func writeArchive(filename string, entries map[string][]byte) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	succeeded := false
	defer func() {
		file.Close()
		if !succeeded {
			_ = os.Remove(filename)
		}
	}()
	writer := zip.NewWriter(file)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetModTime(fixedTime)
		header.SetMode(0o644)
		entry, createErr := writer.CreateHeader(header)
		if createErr != nil {
			return createErr
		}
		if _, writeErr := entry.Write(entries[name]); writeErr != nil {
			return writeErr
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	succeeded = true
	return nil
}

func hashEntries(entries map[string][]byte) string {
	hash := sha256.New()
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		io.WriteString(hash, name)
		hash.Write([]byte{0})
		hash.Write(entries[name])
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func DetectMIME(name string) string {
	if value := allowedAssetExtensions[strings.ToLower(path.Ext(name))]; value != "" {
		return value
	}
	return mime.TypeByExtension(path.Ext(name))
}
