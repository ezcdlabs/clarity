// Package altsource builds the AltStore source — the JSON feed AltStore
// watches to install and update the iOS app. The feed is derived entirely
// from the repository's GitHub releases: one version per release that
// carries the IPA asset, newest first, because AltStore reads versions[0] as
// the latest and compares it with the installed version by equality (it does
// not look at dates or compare semver).
//
// The schema is AltStore Classic's "Make a Source" format. appPermissions is
// present even when empty on purpose: AltStore refuses to install an app
// whose permissions do not match the IPA, and this app has no entitlements
// and no privacy-usage keys to declare.
package altsource

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Release is the slice of a GitHub release the source cares about.
type Release struct {
	Tag         string
	PublishedAt time.Time
	Assets      []Asset
}

// Asset is one release asset.
type Asset struct {
	Name string
	URL  string
	Size int64
}

// ipaAsset is the stable asset name CI publishes, mirrored by the workflow.
const ipaAsset = "clarity-ios.ipa"

// Source is the top-level feed document.
type Source struct {
	Name        string `json:"name"`
	Subtitle    string `json:"subtitle"`
	Description string `json:"description"`
	IconURL     string `json:"iconURL"`
	Website     string `json:"website"`
	Apps        []App  `json:"apps"`
}

// App is the one app this source distributes.
type App struct {
	Name                 string      `json:"name"`
	BundleIdentifier     string      `json:"bundleIdentifier"`
	DeveloperName        string      `json:"developerName"`
	Subtitle             string      `json:"subtitle"`
	LocalizedDescription string      `json:"localizedDescription"`
	IconURL              string      `json:"iconURL"`
	Category             string      `json:"category"`
	AppPermissions       Permissions `json:"appPermissions"`
	Versions             []Version   `json:"versions"`
}

// Permissions is what AltStore validates the IPA against.
type Permissions struct {
	Entitlements []string          `json:"entitlements"`
	Privacy      map[string]string `json:"privacy"`
}

// Version is one installable release.
type Version struct {
	Version              string `json:"version"`
	BuildVersion         string `json:"buildVersion"`
	Date                 string `json:"date"`
	DownloadURL          string `json:"downloadURL"`
	Size                 int64  `json:"size"`
	MinOSVersion         string `json:"minOSVersion"`
	LocalizedDescription string `json:"localizedDescription"`
}

// Build derives the source from releases, which arrive newest-first (the
// order the GitHub API lists them). A release without the IPA asset is not a
// version — offering it would hand AltStore a downloadURL that 404s. No IPA
// anywhere is an error rather than an empty source, and a malformed tag on a
// release that does carry one is the loud kind of error: the generator runs
// in CI, where a failure stops the publish and a guess poisons the feed.
func Build(releases []Release) (Source, error) {
	var versions []Version
	for _, r := range releases {
		var found *Asset
		for i := range r.Assets {
			if r.Assets[i].Name == ipaAsset {
				found = &r.Assets[i]
				break
			}
		}
		if found == nil {
			continue
		}
		build, err := buildVersion(r.Tag)
		if err != nil {
			return Source{}, fmt.Errorf("release %s carries %s but %w", r.Tag, ipaAsset, err)
		}
		versions = append(versions, Version{
			Version:              strings.TrimPrefix(r.Tag, "v"),
			BuildVersion:         build,
			Date:                 r.PublishedAt.UTC().Format(time.RFC3339),
			DownloadURL:          found.URL,
			Size:                 found.Size,
			MinOSVersion:         "16.0",
			LocalizedDescription: "clarity " + strings.TrimPrefix(r.Tag, "v"),
		})
	}
	if len(versions) == 0 {
		return Source{}, fmt.Errorf("no release carries %s — nothing to publish", ipaAsset)
	}

	const icon = "https://raw.githubusercontent.com/ezcdlabs/clarity/main/mobile/apps/ios/Clarity/Assets.xcassets/AppIcon.appiconset/icon-1024.png"
	const description = "Live pipeline status for trunk-based development. " +
		"Commit-centric: HEAD, CI and production deploys per commit, with DORA " +
		"lead times and deploy counts — read straight from a git ref in your " +
		"own repository, no server."
	return Source{
		Name:        "clarity",
		Subtitle:    "Live pipeline status for trunk-based development",
		Description: description,
		IconURL:     icon,
		Website:     "https://github.com/ezcdlabs/clarity",
		Apps: []App{{
			Name:                 "clarity",
			BundleIdentifier:     "dev.ezcd.clarity",
			DeveloperName:        "EzcdLabs",
			Subtitle:             "Is main green right now?",
			LocalizedDescription: description,
			IconURL:              icon,
			Category:             "developer",
			AppPermissions:       Permissions{Entitlements: []string{}, Privacy: map[string]string{}},
			Versions:             versions,
		}},
	}, nil
}

// buildVersion is the same monotonic code the Android versionCode uses —
// major·10⁶ + minor·10³ + patch — because CI stamps it into the IPA's
// CFBundleVersion from the same tag, and AltStore matches the source's
// buildVersion against the IPA's. One formula on both sides, or they drift.
func buildVersion(tag string) (string, error) {
	parts := strings.Split(strings.TrimPrefix(tag, "v"), ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("tag %q is not major.minor.patch", tag)
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return "", fmt.Errorf("tag %q is not major.minor.patch: %w", tag, err)
		}
		nums[i] = n
	}
	return strconv.Itoa(nums[0]*1_000_000 + nums[1]*1_000 + nums[2]), nil
}
