package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/quizdock/quizdock/internal/database"
	"github.com/quizdock/quizdock/internal/qbank"
	"github.com/quizdock/quizdock/internal/releases"
)

func (s *Server) updateCatalog(ctx context.Context) (releases.Catalog, error) {
	banks, err := s.store.Banks(ctx, true)
	if err != nil {
		return releases.Catalog{}, err
	}
	installed := make(map[string]string, len(banks))
	for _, bank := range banks {
		installed[bank.ID] = bank.Version
	}
	return s.releases.Check(ctx, s.version, installed)
}

func (s *Server) updates(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Query().Get("refresh") == "true" {
		s.releases.Invalidate()
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	catalog, err := s.updateCatalog(ctx)
	if err != nil {
		writeError(writer, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, catalog)
}

func (s *Server) installOfficialBank(writer http.ResponseWriter, request *http.Request) {
	s.importMutex.Lock()
	defer s.importMutex.Unlock()

	slug := pathParam(request, "slug")
	catalog, err := s.updateCatalog(request.Context())
	if err != nil {
		writeError(writer, http.StatusBadGateway, err.Error())
		return
	}
	var selected *releases.BankUpdate
	for index := range catalog.Banks {
		if catalog.Banks[index].Slug == slug {
			selected = &catalog.Banks[index]
			break
		}
	}
	if selected == nil {
		writeError(writer, http.StatusNotFound, "官方题库不存在")
		return
	}
	if !selected.InstallAvailable {
		writeError(writer, http.StatusNotFound, "官方题库暂时没有可安装的发布包")
		return
	}

	directory := filepath.Join(s.dataDir, "tmp")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	file, err := os.CreateTemp(directory, "official-*.qbank")
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	filename := file.Name()
	defer os.Remove(filename)
	if err := s.releases.Download(request.Context(), selected.AssetURL, file); err != nil {
		file.Close()
		writeError(writer, http.StatusBadGateway, err.Error())
		return
	}
	if err := file.Close(); err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	pkg, err := qbank.Load(filename)
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, "官方题库包校验失败："+err.Error())
		return
	}
	if pkg.Manifest.ID != selected.ID || pkg.Manifest.Version != selected.LatestVersion {
		writeError(writer, http.StatusUnprocessableEntity, fmt.Sprintf(
			"官方题库包身份不匹配：期望 %s %s", selected.ID, selected.LatestVersion,
		))
		return
	}
	result, err := s.store.ImportPackage(request.Context(), pkg)
	if handleError(writer, err) {
		return
	}
	writeJSON(writer, http.StatusCreated, database.ImportResult{
		BankID: result.BankID, Name: result.Name, Version: result.Version,
		Questions: result.Questions, Assets: result.Assets, Updated: result.Updated,
	})
}
