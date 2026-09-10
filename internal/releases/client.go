package releases

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultAPIURL     = "https://api.github.com/repos/vangee0528/QuizDock/releases?per_page=100"
	DefaultReleaseURL = "https://github.com/vangee0528/QuizDock/releases"
	MaxBankSize       = int64(512 << 20)
)

var (
	appTagPattern  = regexp.MustCompile(`^v([0-9]+\.[0-9]+\.[0-9]+)$`)
	bankTagPattern = regexp.MustCompile(`^qbank/([^/]+)/v([0-9]+\.[0-9]+\.[0-9]+)$`)
	officialBanks  = map[string]BankDefinition{
		"software-designer": {
			Slug: "software-designer", ID: "cn.ruankao.software-designer", Name: "软件设计师题库",
		},
	}
)

type BankDefinition struct {
	Slug string `json:"slug"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ComponentUpdate struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url"`
}

type BankUpdate struct {
	BankDefinition
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version,omitempty"`
	LatestVersion    string `json:"latest_version,omitempty"`
	UpdateAvailable  bool   `json:"update_available"`
	InstallAvailable bool   `json:"install_available"`
	ReleaseURL       string `json:"release_url"`
	AssetURL         string `json:"asset_url,omitempty"`
	AssetSize        int64  `json:"asset_size,omitempty"`
}

type Catalog struct {
	CheckedAt   time.Time       `json:"checked_at"`
	Repository  string          `json:"repository_url"`
	Application ComponentUpdate `json:"application"`
	Banks       []BankUpdate    `json:"banks"`
}

type Client struct {
	apiURL     string
	releaseURL string
	httpClient *http.Client
	mu         sync.Mutex
	cached     []githubRelease
	cacheUntil time.Time
}

func NewClient(apiURL, releaseURL string, httpClient *http.Client) *Client {
	if apiURL == "" {
		apiURL = DefaultAPIURL
	}
	if releaseURL == "" {
		releaseURL = DefaultReleaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Minute}
	}
	return &Client{apiURL: apiURL, releaseURL: releaseURL, httpClient: httpClient}
}

func (c *Client) Invalidate() {
	c.mu.Lock()
	c.cacheUntil = time.Time{}
	c.mu.Unlock()
}

type githubRelease struct {
	TagName    string        `json:"tag_name"`
	HTMLURL    string        `json:"html_url"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Assets     []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

func (c *Client) Check(ctx context.Context, currentVersion string, installed map[string]string) (Catalog, error) {
	releases, err := c.fetch(ctx)
	if err != nil {
		return Catalog{}, err
	}
	catalog := Catalog{
		CheckedAt:  time.Now().UTC(),
		Repository: c.releaseURL,
		Application: ComponentUpdate{
			CurrentVersion: strings.TrimPrefix(currentVersion, "v"),
			ReleaseURL:     c.releaseURL,
		},
	}
	bankBySlug := make(map[string]BankUpdate, len(officialBanks))
	for slug, definition := range officialBanks {
		version, ok := installed[definition.ID]
		bankBySlug[slug] = BankUpdate{
			BankDefinition:   definition,
			Installed:        ok,
			InstalledVersion: version,
			ReleaseURL:       c.releaseURL,
		}
	}
	for _, release := range releases {
		if release.Draft || release.Prerelease {
			continue
		}
		if match := appTagPattern.FindStringSubmatch(release.TagName); match != nil {
			if catalog.Application.LatestVersion == "" || compareVersions(match[1], catalog.Application.LatestVersion) > 0 {
				catalog.Application.LatestVersion = match[1]
				catalog.Application.ReleaseURL = release.HTMLURL
			}
			continue
		}
		match := bankTagPattern.FindStringSubmatch(release.TagName)
		if match == nil {
			continue
		}
		bank, known := bankBySlug[match[1]]
		if !known || (bank.LatestVersion != "" && compareVersions(match[2], bank.LatestVersion) <= 0) {
			continue
		}
		assetName := fmt.Sprintf("%s-%s.qbank", match[1], match[2])
		for _, asset := range release.Assets {
			if asset.Name != assetName || asset.Size <= 0 || asset.Size > MaxBankSize {
				continue
			}
			bank.LatestVersion = match[2]
			bank.ReleaseURL = release.HTMLURL
			bank.AssetURL = asset.BrowserDownloadURL
			bank.AssetSize = asset.Size
			bank.InstallAvailable = true
			bankBySlug[match[1]] = bank
			break
		}
	}
	if catalog.Application.LatestVersion != "" && validVersion(catalog.Application.CurrentVersion) {
		catalog.Application.UpdateAvailable = compareVersions(catalog.Application.LatestVersion, catalog.Application.CurrentVersion) > 0
	}
	for _, slug := range []string{"software-designer"} {
		bank := bankBySlug[slug]
		bank.UpdateAvailable = bank.Installed && bank.InstallAvailable && compareVersions(bank.LatestVersion, bank.InstalledVersion) > 0
		catalog.Banks = append(catalog.Banks, bank)
	}
	return catalog, nil
}

func (c *Client) Download(ctx context.Context, assetURL string, destination io.Writer) error {
	if assetURL == "" {
		return fmt.Errorf("题库发布包不存在")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "QuizDock")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("下载题库失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("下载题库失败：HTTP %d", response.StatusCode)
	}
	if response.ContentLength > MaxBankSize {
		return fmt.Errorf("题库包超过 512 MiB")
	}
	written, err := io.Copy(destination, io.LimitReader(response.Body, MaxBankSize+1))
	if err != nil {
		return fmt.Errorf("下载题库失败：%w", err)
	}
	if written > MaxBankSize {
		return fmt.Errorf("题库包超过 512 MiB")
	}
	return nil
}

func (c *Client) fetch(ctx context.Context) ([]githubRelease, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().Before(c.cacheUntil) {
		return append([]githubRelease(nil), c.cached...), nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "QuizDock")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("检查更新失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("检查更新失败：GitHub API 返回 HTTP %d", response.StatusCode)
	}
	var releases []githubRelease
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
	if err := decoder.Decode(&releases); err != nil {
		return nil, fmt.Errorf("解析发布信息失败：%w", err)
	}
	c.cached = append([]githubRelease(nil), releases...)
	c.cacheUntil = time.Now().Add(15 * time.Minute)
	return append([]githubRelease(nil), releases...), nil
}

func validVersion(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}

func compareVersions(left, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	for index := 0; index < 3; index++ {
		leftValue, rightValue := 0, 0
		if index < len(leftParts) {
			leftValue, _ = strconv.Atoi(leftParts[index])
		}
		if index < len(rightParts) {
			rightValue, _ = strconv.Atoi(rightParts[index])
		}
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
	}
	return 0
}
