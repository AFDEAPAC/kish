package collectors_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/infrastructure/collectors"
	"github.com/AFDEAPAC/kish/internal/infrastructure/command"
	"github.com/AFDEAPAC/kish/internal/testutil"
)

const sampleDpkgOutput = `Desired=Unknown/Install/Remove/Purge/Hold
|| Status=Not/Inst/Conf-files/Unpacked/halF-conf/Half-inst/trig-aWait/Trig-pend
||/ Err?=(none)/Reinst-required (Status,Err: uppercase=bad)
|||/ Name                Version         Architecture Description
+++-===================-===============-============-==========
ii  libnuma1            2.0.18-1build1  amd64        NUMA policy library
ii  libpython3.12       3.12.3-1        amd64        Shared Python runtime library
un  some-uninstalled    <none>          <none>        (no description available)
`

func TestSystemPackageCollector_DpkgParsing(t *testing.T) {
	runner := testutil.NewFakeRunner()
	runner.LookPathResponses["dpkg"] = nil // dpkg found
	runner.RunResponses["dpkg"] = testutil.FakeRunResponse{
		Result: command.CommandResult{Stdout: sampleDpkgOutput},
	}

	c := collectors.NewSystemPackageCollector(runner, t.TempDir())
	result := c.Collect(context.Background())

	if result.Status != environment.StatusSuccess {
		t.Errorf("expected status=success, got %q; warnings: %v", result.Status, result.Warnings)
	}
	if len(result.PackageSets) != 1 {
		t.Fatalf("expected 1 package set, got %d", len(result.PackageSets))
	}
	ps := result.PackageSets[0]
	if ps.Manager != "dpkg" {
		t.Errorf("expected manager=dpkg, got %q", ps.Manager)
	}
	// Only "ii" lines should be parsed; "un" line should be skipped.
	if len(ps.Packages) != 2 {
		t.Errorf("expected 2 packages, got %d: %+v", len(ps.Packages), ps.Packages)
	}
	if ps.Packages[0].Name != "libnuma1" {
		t.Errorf("expected first package name=libnuma1, got %q", ps.Packages[0].Name)
	}
	if ps.Packages[0].Version != "2.0.18-1build1" {
		t.Errorf("expected first package version=2.0.18-1build1, got %q", ps.Packages[0].Version)
	}
	if ps.Packages[0].Architecture != "amd64" {
		t.Errorf("expected first package arch=amd64, got %q", ps.Packages[0].Architecture)
	}
}

func TestSystemPackageCollector_DpkgParsing_RawFileWritten(t *testing.T) {
	dir := t.TempDir()
	runner := testutil.NewFakeRunner()
	runner.LookPathResponses["dpkg"] = nil
	runner.RunResponses["dpkg"] = testutil.FakeRunResponse{
		Result: command.CommandResult{Stdout: sampleDpkgOutput},
	}

	c := collectors.NewSystemPackageCollector(runner, dir)
	result := c.Collect(context.Background())

	if len(result.PackageSets) == 0 {
		t.Fatal("expected at least one package set")
	}
	ps := result.PackageSets[0]
	if ps.RawFile == "" {
		t.Error("expected RawFile to be set")
	}
	fullPath := filepath.Join(dir, ps.RawFile)
	if _, err := os.Stat(fullPath); err != nil {
		t.Errorf("expected raw file to exist at %q: %v", fullPath, err)
	}
}

func TestSystemPackageCollector_NeitherDpkgNorRpm_Skipped(t *testing.T) {
	runner := testutil.NewFakeRunner()
	// LookPath returns errors for both (default behaviour of FakeRunner).

	c := collectors.NewSystemPackageCollector(runner, t.TempDir())
	result := c.Collect(context.Background())

	if result.Status != environment.StatusSkipped {
		t.Errorf("expected status=skipped, got %q", result.Status)
	}
}

func TestSystemPackageCollector_DpkgCommandFails_StatusFailed(t *testing.T) {
	runner := testutil.NewFakeRunner()
	runner.LookPathResponses["dpkg"] = nil
	runner.RunResponses["dpkg"] = testutil.FakeRunResponse{
		Result: command.CommandResult{ExitCode: 1},
		Err:    errors.New("dpkg: command not found"),
	}

	c := collectors.NewSystemPackageCollector(runner, t.TempDir())
	result := c.Collect(context.Background())

	if result.Status != environment.StatusFailed {
		t.Errorf("expected status=failed, got %q", result.Status)
	}
}

func TestSystemPackageCollector_RPMQueryformat_ParsesCorrectly(t *testing.T) {
	// Tab-delimited queryformat output: NAME\tVERSION\tRELEASE\tARCH
	rpmOutput := "sgml-common\t0.6.3\t50.1.al8\tnoarch\n" +
		"bash\t5.1.8\t6.el9\tx86_64\n"

	runner := testutil.NewFakeRunner()
	runner.LookPathResponses["rpm"] = nil
	runner.RunResponses["rpm"] = testutil.FakeRunResponse{
		Result: command.CommandResult{Stdout: rpmOutput},
	}

	c := collectors.NewSystemPackageCollector(runner, t.TempDir())
	result := c.Collect(context.Background())

	if result.Status != environment.StatusSuccess {
		t.Errorf("expected status=success, got %q; warnings: %v", result.Status, result.Warnings)
	}
	if len(result.PackageSets) != 1 {
		t.Fatalf("expected 1 package set, got %d", len(result.PackageSets))
	}
	pkgs := result.PackageSets[0].Packages
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}

	if pkgs[0].Name != "sgml-common" {
		t.Errorf("expected name=sgml-common, got %q", pkgs[0].Name)
	}
	if pkgs[0].Version != "0.6.3" {
		t.Errorf("expected version=0.6.3, got %q", pkgs[0].Version)
	}
	if pkgs[0].Release != "50.1.al8" {
		t.Errorf("expected release=50.1.al8, got %q", pkgs[0].Release)
	}
	if pkgs[0].Architecture != "noarch" {
		t.Errorf("expected architecture=noarch, got %q", pkgs[0].Architecture)
	}
}

func TestSystemPackageCollector_RPMQueryformat_MalformedLineIsPartial(t *testing.T) {
	// Mix of valid and malformed lines.
	rpmOutput := "good-pkg\t1.0\t1.el9\tx86_64\n" +
		"malformed-line-no-tabs\n"

	runner := testutil.NewFakeRunner()
	runner.LookPathResponses["rpm"] = nil
	runner.RunResponses["rpm"] = testutil.FakeRunResponse{
		Result: command.CommandResult{Stdout: rpmOutput},
	}

	c := collectors.NewSystemPackageCollector(runner, t.TempDir())
	result := c.Collect(context.Background())

	// Malformed lines should not fail the collector entirely.
	if result.Status == environment.StatusFailed {
		t.Errorf("malformed lines should not cause failed status, got %q", result.Status)
	}
	if result.Status != environment.StatusPartial {
		t.Errorf("expected status=partial for malformed lines, got %q", result.Status)
	}
	// Both packages should be present (malformed as raw entry).
	if len(result.PackageSets[0].Packages) != 2 {
		t.Errorf("expected 2 packages (including malformed), got %d", len(result.PackageSets[0].Packages))
	}
}

func TestSystemPackageCollector_RPM_RawFileWritten(t *testing.T) {
	dir := t.TempDir()
	runner := testutil.NewFakeRunner()
	runner.LookPathResponses["rpm"] = nil
	runner.RunResponses["rpm"] = testutil.FakeRunResponse{
		Result: command.CommandResult{Stdout: "pkg\t1.0\t1\tx86_64\n"},
	}

	c := collectors.NewSystemPackageCollector(runner, dir)
	result := c.Collect(context.Background())

	if len(result.PackageSets) == 0 {
		t.Fatal("expected package set")
	}
	ps := result.PackageSets[0]
	if ps.RawFile == "" {
		t.Error("expected RawFile to be set")
	}
	fullPath := filepath.Join(dir, ps.RawFile)
	if _, err := os.Stat(fullPath); err != nil {
		t.Errorf("expected raw file to exist at %q: %v", fullPath, err)
	}
}
