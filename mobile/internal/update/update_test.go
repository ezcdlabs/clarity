package update_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ezcdlabs/clarity/mobile/internal/update"
)

// TestNewerThan pins the semver comparison. An update is offered only for a
// strictly newer release: equal versions are current, and a dev build that
// somehow reads higher than the latest release must not be told to
// "update" backwards.
func TestNewerThan(t *testing.T) {
	cases := []struct {
		name    string
		remote  string
		current string
		want    bool
	}{
		{"newer patch", "v1.3.1", "1.3.0", true},
		{"newer minor", "v1.4.0", "1.3.9", true},
		{"newer major", "v2.0.0", "1.9.9", true},
		{"equal", "v1.3.0", "1.3.0", false},
		{"older", "v1.2.9", "1.3.0", false},
		{"numeric not lexicographic", "v1.10.0", "1.9.0", true},
		{"dev build sees every release as newer", "v1.3.0", "0.0.0-dev", true},
		{"current may carry the v too", "v1.4.0", "v1.3.0", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := update.NewerThan(c.remote, c.current)
			if err != nil {
				t.Fatalf("NewerThan(%q, %q): %v", c.remote, c.current, err)
			}
			if got != c.want {
				t.Errorf("NewerThan(%q, %q) = %v, want %v", c.remote, c.current, got, c.want)
			}
		})
	}
}

// A tag that is not a semver is an error, not a quiet "no update": the only
// way it happens is a release process change, and silently never updating
// again is the worst possible way to learn about one.
func TestNewerThan_MalformedRemoteTagIsAnError(t *testing.T) {
	if _, err := update.NewerThan("nightly", "1.3.0"); err == nil {
		t.Fatal("a malformed remote tag was accepted")
	}
}

// latestRelease serves the subset of the GitHub "latest release" response the
// checker reads, plus the sha256 asset's content.
func latestRelease(t *testing.T, tag string, withSHA bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/ezcdlabs/clarity/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		assets := fmt.Sprintf(`{"name":"clarity-android.apk","browser_download_url":%q,"size":123456}`,
			srv.URL+"/dl/clarity-android.apk")
		if withSHA {
			assets += fmt.Sprintf(`,{"name":"clarity-android.apk.sha256","browser_download_url":%q,"size":64}`,
				srv.URL+"/dl/clarity-android.apk.sha256")
		}
		fmt.Fprintf(w, `{"tag_name":%q,"assets":[%s]}`, tag, assets)
	})
	mux.HandleFunc("/dl/clarity-android.apk.sha256", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "abc123def456  clarity-android.apk")
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCheck_NewerReleaseIsAnUpdate(t *testing.T) {
	srv := latestRelease(t, "v1.4.0", true)
	got, err := update.Check(srv.Client(), srv.URL, "ezcdlabs/clarity", "clarity-android.apk", "1.3.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !got.Available {
		t.Fatal("a newer release was not offered as an update")
	}
	if got.Version != "1.4.0" {
		t.Errorf("Version = %q, want 1.4.0 (bare, as the app displays it)", got.Version)
	}
	if got.DownloadURL != srv.URL+"/dl/clarity-android.apk" {
		t.Errorf("DownloadURL = %q", got.DownloadURL)
	}
	if got.Size != 123456 {
		t.Errorf("Size = %d, want 123456", got.Size)
	}
	if got.SHA256 != "abc123def456" {
		t.Errorf("SHA256 = %q, want the bare hex from the checksum asset", got.SHA256)
	}
}

func TestCheck_CurrentVersionIsNotAnUpdate(t *testing.T) {
	srv := latestRelease(t, "v1.3.0", true)
	got, err := update.Check(srv.Client(), srv.URL, "ezcdlabs/clarity", "clarity-android.apk", "1.3.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got.Available {
		t.Error("the installed version was offered as an update to itself")
	}
}

// A release without the APK asset is one cut before mobile assets existed, or
// one whose mobile-assets job failed. Either way there is nothing to offer —
// not an error, because the app asks on every open and a red banner about a
// server-side gap helps nobody.
func TestCheck_ReleaseWithoutTheAssetIsNoUpdate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/ezcdlabs/clarity/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v9.9.9","assets":[]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	got, err := update.Check(srv.Client(), srv.URL, "ezcdlabs/clarity", "clarity-android.apk", "1.3.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got.Available {
		t.Error("a release with no APK asset was offered as an update")
	}
}

// The checksum asset is a guard, not a dependency: the system installer
// already refuses an APK whose signature differs from the installed app's,
// so a release missing its .sha256 still updates — with SHA256 empty, which
// tells the platform side to skip its pre-install verification.
func TestCheck_MissingChecksumAssetLeavesSHAEmpty(t *testing.T) {
	srv := latestRelease(t, "v1.4.0", false)
	got, err := update.Check(srv.Client(), srv.URL, "ezcdlabs/clarity", "clarity-android.apk", "1.3.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !got.Available || got.SHA256 != "" {
		t.Errorf("want an available update with empty SHA256, got %+v", got)
	}
}
