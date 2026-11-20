// Package update decides whether a newer release of the app exists.
//
// The decision lives here, on the Go side of the FFI boundary, for the same
// reason every other decision does: it can be tested without a device. The
// platform side is left with only what is platform by nature — downloading
// the file and handing it to the system installer.
//
// The feed is the repository's own GitHub releases, read anonymously. The
// unauthenticated rate limit (60 requests/hour/IP) is generous for a check on
// app open, and the request carries no user data — a generic User-Agent,
// which GitHub requires, and nothing else.
package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Result is the verdict of one check.
//
// SHA256 is the bare hex digest of the asset, from the `.sha256` file
// published next to it, and may be empty: the checksum is a guard against a
// corrupted download, not the security boundary — that is the platform
// installer, which refuses any APK whose signing key differs from the
// installed app's. An empty SHA256 means "install without pre-verification",
// not "do not install".
type Result struct {
	Available   bool
	Version     string // bare semver, no v — what the app displays
	DownloadURL string
	Size        int64
	SHA256      string
}

// Check reads the latest release of repo from a GitHub-shaped API at apiBase
// and reports whether it updates current. A release without the named asset
// is reported as no update rather than an error: it is a release cut before
// mobile assets existed (or whose asset job failed), the app asks on every
// open, and an error banner about a server-side gap helps nobody.
func Check(client *http.Client, apiBase, repo, assetName, current string) (Result, error) {
	req, err := http.NewRequest("GET", apiBase+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return Result{}, err
	}
	// GitHub rejects requests without a User-Agent. A generic client name,
	// deliberately: nothing about the person or their repositories crosses.
	req.Header.Set("User-Agent", "git-clarity-app")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("latest release: HTTP %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return Result{}, fmt.Errorf("latest release: %w", err)
	}

	newer, err := NewerThan(release.TagName, current)
	if err != nil {
		return Result{}, err
	}
	res := Result{Version: strings.TrimPrefix(release.TagName, "v")}
	if !newer {
		return res, nil
	}

	var shaURL string
	for _, a := range release.Assets {
		switch a.Name {
		case assetName:
			res.Available = true
			res.DownloadURL = a.URL
			res.Size = a.Size
		case assetName + ".sha256":
			shaURL = a.URL
		}
	}
	if !res.Available {
		return Result{Version: res.Version}, nil
	}
	if shaURL != "" {
		if sha, err := fetchSHA(client, shaURL); err == nil {
			res.SHA256 = sha
		}
		// A checksum that cannot be fetched degrades to no pre-verification,
		// exactly like a release that never published one.
	}
	return res, nil
}

// fetchSHA reads a `sha256sum`-format checksum file ("<hex>  <name>") and
// returns the bare hex.
func fetchSHA(client *http.Client, url string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "git-clarity-app")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksum: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return "", fmt.Errorf("checksum: empty file")
	}
	return fields[0], nil
}

// NewerThan reports whether remote (a release tag, with or without the v) is
// strictly newer than current (the installed version). Strictly: equal is
// current, and a version somehow ahead of the latest release must not be
// told to "update" backwards.
//
// current is parsed leniently — a local dev build reads "0.0.0-dev", and
// every release is newer than that, which is the honest answer for a build
// no release process produced. remote is parsed strictly: the only way the
// latest release tag stops being semver is a release-process change, and
// silently never updating again is the worst way to learn of one.
func NewerThan(remote, current string) (bool, error) {
	r, err := parseSemver(remote, false)
	if err != nil {
		return false, fmt.Errorf("release tag %q: %w", remote, err)
	}
	c, err := parseSemver(current, true)
	if err != nil {
		return false, fmt.Errorf("installed version %q: %w", current, err)
	}
	for i := range 3 {
		if r[i] != c[i] {
			return r[i] > c[i], nil
		}
	}
	return false, nil
}

// parseSemver reads "v1.2.3" or "1.2.3" into its three numbers. Lenient
// parsing takes the leading digits of each part, so "0.0.0-dev" parses as
// 0.0.0 rather than failing on the suffix.
func parseSemver(s string, lenient bool) ([3]int, error) {
	var out [3]int
	parts := strings.SplitN(strings.TrimPrefix(s, "v"), ".", 3)
	if len(parts) != 3 {
		return out, fmt.Errorf("not a major.minor.patch version")
	}
	for i, p := range parts {
		if lenient {
			j := 0
			for j < len(p) && p[j] >= '0' && p[j] <= '9' {
				j++
			}
			p = p[:j]
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, fmt.Errorf("part %d: %w", i+1, err)
		}
		out[i] = n
	}
	return out, nil
}
