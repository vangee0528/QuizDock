package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/quizdock/quizdock/internal/database"
	"github.com/quizdock/quizdock/internal/qbank"
	"github.com/quizdock/quizdock/internal/webui"
)

const maxUploadSize = int64(512 << 20)

type Server struct {
	store       *database.Store
	version     string
	dataDir     string
	webDevURL   string
	importMutex sync.Mutex
}

func New(store *database.Store, version, dataDir, webDevURL string) http.Handler {
	server := &Server{store: store, version: version, dataDir: dataDir, webDevURL: webDevURL}
	router := chi.NewRouter()
	router.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	router.Use(server.loggingMiddleware)
	router.Route("/api/v1", func(api chi.Router) {
		api.Get("/health", server.health)
		api.Get("/meta", server.meta)
		api.Get("/banks", server.banks)
		api.Post("/banks/import", server.importBank)
		api.Put("/banks/{bankID}/enabled", server.setBankEnabled)
		api.Delete("/banks/{bankID}", server.removeBank)
		api.Get("/questions", server.questions)
		api.Get("/questions/{uid}", server.question)
		api.Post("/answers", server.answer)
		api.Get("/progress", server.progress)
		api.Put("/progress", server.saveProgress)
		api.Put("/questions/{uid}/state", server.setQuestionState)
		api.Get("/settings", server.settings)
		api.Put("/settings", server.saveSettings)
		api.Get("/assets/{bankID}/*", server.asset)
		api.NotFound(func(writer http.ResponseWriter, _ *http.Request) {
			writeError(writer, http.StatusNotFound, "API 资源不存在")
		})
		api.MethodNotAllowed(func(writer http.ResponseWriter, _ *http.Request) {
			writeError(writer, http.StatusMethodNotAllowed, "请求方法不受支持")
		})
	})
	router.NotFound(server.frontend)
	return router
}

func (s *Server) health(writer http.ResponseWriter, request *http.Request) {
	if err := s.store.Ping(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database is unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"status": "ok", "version": s.version, "time": time.Now().UTC()})
}

func (s *Server) meta(writer http.ResponseWriter, request *http.Request) {
	value, err := s.store.Meta(request.Context(), s.version)
	if handleError(writer, err) {
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func (s *Server) banks(writer http.ResponseWriter, request *http.Request) {
	value, err := s.store.Banks(request.Context(), true)
	if handleError(writer, err) {
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"banks": value})
}

func (s *Server) importBank(writer http.ResponseWriter, request *http.Request) {
	s.importMutex.Lock()
	defer s.importMutex.Unlock()
	request.Body = http.MaxBytesReader(writer, request.Body, maxUploadSize)
	if err := request.ParseMultipartForm(8 << 20); err != nil {
		writeError(writer, http.StatusBadRequest, "题库包无效或超过 512 MiB")
		return
	}
	defer request.MultipartForm.RemoveAll()
	file, header, err := request.FormFile("bank")
	if err != nil {
		writeError(writer, http.StatusBadRequest, "请选择 .qbank 文件")
		return
	}
	defer file.Close()
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".qbank") {
		writeError(writer, http.StatusBadRequest, "只支持 .qbank 文件")
		return
	}
	filename, cleanup, err := s.persistUpload(file)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	defer cleanup()
	pkg, err := qbank.Load(filename)
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := s.store.ImportPackage(request.Context(), pkg)
	if handleError(writer, err) {
		return
	}
	writeJSON(writer, http.StatusCreated, result)
}

func (s *Server) persistUpload(source multipart.File) (string, func(), error) {
	directory := filepath.Join(s.dataDir, "tmp")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", func() {}, err
	}
	file, err := os.CreateTemp(directory, "import-*.qbank")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.Remove(file.Name()) }
	written, err := io.Copy(file, io.LimitReader(source, maxUploadSize+1))
	closeErr := file.Close()
	if err != nil || closeErr != nil || written > maxUploadSize {
		cleanup()
		if err != nil {
			return "", func() {}, err
		}
		if closeErr != nil {
			return "", func() {}, closeErr
		}
		return "", func() {}, fmt.Errorf("题库包超过 512 MiB")
	}
	return file.Name(), cleanup, nil
}

func (s *Server) setBankEnabled(writer http.ResponseWriter, request *http.Request) {
	var payload struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeJSON(writer, request, &payload) {
		return
	}
	err := s.store.SetBankEnabled(request.Context(), pathParam(request, "bankID"), payload.Enabled)
	if handleError(writer, err) {
		return
	}
	writeJSON(writer, http.StatusOK, payload)
}

func (s *Server) removeBank(writer http.ResponseWriter, request *http.Request) {
	purge := request.URL.Query().Get("purge_learning") == "true"
	err := s.store.RemoveBank(request.Context(), pathParam(request, "bankID"), purge)
	if handleError(writer, err) {
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) questions(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	filter := database.QueueFilter{
		Mode: query.Get("mode"), BankIDs: query["bank"], Chapter: query.Get("chapter"),
		Tag: query.Get("tag"), Exam: query.Get("exam"), Limit: limit,
		DueDate: time.Now().Format("2006-01-02"),
	}
	if filter.Mode == "review" && filter.Limit == 0 {
		settings, err := s.store.Settings(request.Context())
		if handleError(writer, err) {
			return
		}
		filter.Limit = settings.DailyTarget
	}
	questions, err := s.store.QuestionQueue(request.Context(), filter)
	if handleError(writer, err) {
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"questions": questions, "count": len(questions)})
}

func (s *Server) question(writer http.ResponseWriter, request *http.Request) {
	value, err := s.store.Question(request.Context(), pathParam(request, "uid"))
	if handleError(writer, err) {
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func (s *Server) answer(writer http.ResponseWriter, request *http.Request) {
	var payload struct {
		UID        string              `json:"uid"`
		Answers    map[string][]string `json:"answers"`
		DurationMS int                 `json:"duration_ms"`
	}
	if !decodeJSON(writer, request, &payload) {
		return
	}
	if payload.UID == "" || len(payload.Answers) == 0 {
		writeError(writer, http.StatusBadRequest, "题目和答案不能为空")
		return
	}
	result, err := s.store.SubmitAnswer(request.Context(), payload.UID, payload.Answers, payload.DurationMS)
	if handleError(writer, err) {
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (s *Server) progress(writer http.ResponseWriter, request *http.Request) {
	scope := request.URL.Query().Get("scope")
	if scope == "" {
		writeError(writer, http.StatusBadRequest, "scope 不能为空")
		return
	}
	value, err := s.store.Progress(request.Context(), scope)
	if handleError(writer, err) {
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"progress": value})
}

func (s *Server) saveProgress(writer http.ResponseWriter, request *http.Request) {
	var payload database.Progress
	if !decodeJSON(writer, request, &payload) {
		return
	}
	if payload.ScopeKey == "" || payload.CurrentUID == "" {
		writeError(writer, http.StatusBadRequest, "进度数据不完整")
		return
	}
	if err := s.store.SaveProgress(request.Context(), payload); handleError(writer, err) {
		return
	}
	writeJSON(writer, http.StatusOK, map[string]bool{"saved": true})
}

func (s *Server) setQuestionState(writer http.ResponseWriter, request *http.Request) {
	var payload struct {
		Starred bool `json:"starred"`
	}
	if !decodeJSON(writer, request, &payload) {
		return
	}
	if err := s.store.SetStarred(request.Context(), pathParam(request, "uid"), payload.Starred); handleError(writer, err) {
		return
	}
	writeJSON(writer, http.StatusOK, payload)
}

func (s *Server) settings(writer http.ResponseWriter, request *http.Request) {
	value, err := s.store.Settings(request.Context())
	if handleError(writer, err) {
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func (s *Server) saveSettings(writer http.ResponseWriter, request *http.Request) {
	var payload database.Settings
	if !decodeJSON(writer, request, &payload) {
		return
	}
	if err := s.store.SaveSettings(request.Context(), payload); handleError(writer, err) {
		return
	}
	writeJSON(writer, http.StatusOK, payload)
}

func (s *Server) asset(writer http.ResponseWriter, request *http.Request) {
	assetPath := strings.TrimPrefix(chi.URLParam(request, "*"), "/")
	data, mimeType, hash, err := s.store.Asset(request.Context(), pathParam(request, "bankID"), assetPath)
	if handleError(writer, err) {
		return
	}
	writer.Header().Set("Content-Type", mimeType)
	writer.Header().Set("ETag", `"`+hash+`"`)
	writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(data)
}

func (s *Server) frontend(writer http.ResponseWriter, request *http.Request) {
	if s.webDevURL != "" {
		target, err := url.Parse(s.webDevURL)
		if err != nil {
			writeError(writer, http.StatusInternalServerError, "invalid web development URL")
			return
		}
		httputil.NewSingleHostReverseProxy(target).ServeHTTP(writer, request)
		return
	}
	dist, err := fs.Sub(webui.Dist, "dist")
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "embedded frontend is unavailable")
		return
	}
	requested := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
	if requested == "." || requested == "" {
		requested = "index.html"
	}
	if _, err := fs.Stat(dist, requested); err != nil {
		requested = "index.html"
	}
	data, err := fs.ReadFile(dist, requested)
	if err != nil {
		writeError(writer, http.StatusNotFound, "frontend resource not found")
		return
	}
	contentType := mime.TypeByExtension(path.Ext(requested))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	writer.Header().Set("Content-Type", contentType)
	if requested == "index.html" {
		writer.Header().Set("Cache-Control", "no-cache")
	} else {
		writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeContent(writer, request, requested, time.Time{}, bytes.NewReader(data))
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		start := time.Now()
		next.ServeHTTP(writer, request)
		slog.Info("http request", "method", request.Method, "path", request.URL.Path, "duration", time.Since(start), "request_id", middleware.GetReqID(request.Context()))
	})
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, destination any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, 1<<20)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(writer, http.StatusBadRequest, "请求 JSON 无效："+err.Error())
		return false
	}
	return true
}

func pathParam(request *http.Request, name string) string {
	value := chi.URLParam(request, name)
	if decoded, err := url.PathUnescape(value); err == nil {
		return decoded
	}
	return value
}

func handleError(writer http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, database.ErrNotFound) {
		writeError(writer, http.StatusNotFound, "资源不存在")
	} else {
		slog.Error("request failed", "error", err)
		writeError(writer, http.StatusInternalServerError, err.Error())
	}
	return true
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func ShutdownServer(server *http.Server, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return server.Shutdown(ctx)
}
