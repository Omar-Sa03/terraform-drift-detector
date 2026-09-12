package httpserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	httpserver "github.com/medom/terraform-drift-detector/internal/http"
	"github.com/medom/terraform-drift-detector/internal/model"
	"github.com/medom/terraform-drift-detector/internal/report"
	"github.com/medom/terraform-drift-detector/internal/scan"
)

func TestDashboardAndReportAPI(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "last-report.json")
	rep := report.Build("state", nil, nil, []model.Drift{{
		Kind: model.KindDeleted, Address: "aws_instance.web", ID: "i-1",
	}}, nil)
	if err := report.SaveFile(out, rep); err != nil {
		t.Fatal(err)
	}
	srv := httpserver.New(&scan.Engine{}, scan.Options{OutPath: out})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("dashboard %d", res.StatusCode)
	}

	res, err = http.Get(ts.URL + "/api/report")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var got model.Report
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Summary.Deleted != 1 {
		t.Fatalf("%+v", got.Summary)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
}
