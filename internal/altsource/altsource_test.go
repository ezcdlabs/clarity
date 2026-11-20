package altsource_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/internal/altsource"
)

func release(tag string, published time.Time, assets ...altsource.Asset) altsource.Release {
	return altsource.Release{Tag: tag, PublishedAt: published, Assets: assets}
}

func ipa(url string, size int64) altsource.Asset {
	return altsource.Asset{Name: "clarity-ios.ipa", URL: url, Size: size}
}

// The source lists one version per release that actually carries the IPA,
// newest first — AltStore reads versions[0] as the latest, compares it with
// the installed version by equality, and does not look at dates. A release
// without the asset (cut before iOS assets existed, or whose job failed)
// is simply not a version; offering it would give AltStore a downloadURL
// that 404s.
func TestBuild_OneVersionPerIPARelease_NewestFirst(t *testing.T) {
	src, err := altsource.Build([]altsource.Release{
		release("v1.5.0", time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC), ipa("https://dl/v1.5.0.ipa", 500)),
		release("v1.4.1", time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)), // no IPA
		release("v1.4.0", time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC), ipa("https://dl/v1.4.0.ipa", 400)),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	app := src.Apps[0]
	if len(app.Versions) != 2 {
		t.Fatalf("expected 2 versions (the release without an IPA is not one), got %+v", app.Versions)
	}
	v := app.Versions[0]
	if v.Version != "1.5.0" {
		t.Errorf("versions[0] = %q, want the newest, bare of its v", v.Version)
	}
	if v.DownloadURL != "https://dl/v1.5.0.ipa" || v.Size != 500 {
		t.Errorf("versions[0] carries %q/%d, want the IPA asset's URL and size", v.DownloadURL, v.Size)
	}
	if v.Date != "2026-11-21T10:00:00Z" {
		t.Errorf("date = %q, want ISO 8601", v.Date)
	}
}

// buildVersion must match the IPA's CFBundleVersion, which CI sets to the
// same monotonic code the Android versionCode uses: major·10⁶ + minor·10³ +
// patch. One formula, derived from the tag on both sides, so the source and
// the binary cannot disagree.
func TestBuild_BuildVersionMatchesTheMonotonicCode(t *testing.T) {
	src, err := altsource.Build([]altsource.Release{
		release("v1.4.0", time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC), ipa("https://dl", 1)),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := src.Apps[0].Versions[0].BuildVersion; got != "1004000" {
		t.Errorf("buildVersion = %q, want 1004000", got)
	}
}

// A release tag that is not semver cannot produce a buildVersion that
// matches any IPA, so it is an error — the loud kind, because the generator
// runs in CI where an error fails the job and a guess poisons the source.
func TestBuild_MalformedTagIsAnError(t *testing.T) {
	_, err := altsource.Build([]altsource.Release{
		release("nightly", time.Now(), ipa("https://dl", 1)),
	})
	if err == nil {
		t.Fatal("a malformed tag was accepted")
	}
}

// The envelope AltStore validates installs against: identity fields,
// and appPermissions present even when empty — AltStore refuses to install
// an app whose permissions do not match the IPA, and this app has no
// entitlements and no privacy-usage keys to declare.
func TestBuild_EnvelopeCarriesIdentityAndEmptyPermissions(t *testing.T) {
	src, err := altsource.Build([]altsource.Release{
		release("v1.4.0", time.Now().UTC(), ipa("https://dl", 1)),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	app := src.Apps[0]
	if app.BundleIdentifier != "dev.ezcd.clarity" {
		t.Errorf("bundleIdentifier = %q", app.BundleIdentifier)
	}
	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{
		`"appPermissions"`, `"entitlements":[]`, `"privacy":{}`,
		`"bundleIdentifier":"dev.ezcd.clarity"`, `"minOSVersion":"16.0"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("source JSON is missing %s:\n%s", want, raw)
		}
	}
}

// No IPA anywhere is an error, not an empty source: publishing a source with
// an app that has no versions makes AltStore show an app that cannot be
// installed, and the only way it happens is the generator running before the
// first iOS asset exists.
func TestBuild_NoIPAAnywhereIsAnError(t *testing.T) {
	_, err := altsource.Build([]altsource.Release{
		release("v1.4.0", time.Now(), altsource.Asset{Name: "clarity-android.apk", URL: "x", Size: 1}),
	})
	if err == nil {
		t.Fatal("an empty source was built")
	}
}
