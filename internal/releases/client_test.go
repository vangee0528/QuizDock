package releases

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckSeparatesApplicationAndBankVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[
          {"tag_name":"v0.2.0","html_url":"https://example/app","draft":false,"prerelease":false,"assets":[]},
          {"tag_name":"qbank/software-designer/v0.1.2","html_url":"https://example/bank","draft":false,"prerelease":false,
           "assets":[{"name":"software-designer-0.1.2.qbank","browser_download_url":"https://example/bank.qbank","size":1024}]},
          {"tag_name":"qbank/software-designer/v0.1.1","html_url":"https://example/old","draft":false,"prerelease":false,
           "assets":[{"name":"software-designer-0.1.1.qbank","browser_download_url":"https://example/old.qbank","size":1024}]}
        ]`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "https://example/releases", server.Client())
	catalog, err := client.Check(context.Background(), "0.1.0", map[string]string{"cn.ruankao.software-designer": "0.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	if !catalog.Application.UpdateAvailable || catalog.Application.LatestVersion != "0.2.0" {
		t.Fatalf("unexpected application update: %+v", catalog.Application)
	}
	if len(catalog.Banks) != 1 || !catalog.Banks[0].UpdateAvailable || catalog.Banks[0].LatestVersion != "0.1.2" {
		t.Fatalf("unexpected bank update: %+v", catalog.Banks)
	}
}

func TestDevelopmentVersionDoesNotReportApplicationUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[{"tag_name":"v9.0.0","html_url":"https://example/app","draft":false,"prerelease":false,"assets":[]}]`))
	}))
	defer server.Close()

	catalog, err := NewClient(server.URL, "", server.Client()).Check(context.Background(), "dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Application.UpdateAvailable {
		t.Fatal("development builds must not report an application update")
	}
}
