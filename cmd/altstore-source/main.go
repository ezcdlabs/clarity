// Command altstore-source prints the AltStore source JSON for this
// repository's releases. It is a CI helper (not part of the shipped
// git-clarity binary, like cmd/next-version): the mobile-assets workflow
// runs it after attaching the IPA and publishes its output to GitHub Pages,
// which is the URL users add to AltStore. The derivation itself lives in
// internal/altsource and is unit-tested there.
//
// GITHUB_TOKEN is used when present (CI), so the listing never races the
// unauthenticated rate limit; without it the public limit is plenty for a
// manual run.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ezcdlabs/clarity/internal/altsource"
)

func main() {
	releases, err := listReleases()
	if err != nil {
		fmt.Fprintln(os.Stderr, "altstore-source:", err)
		os.Exit(1)
	}
	src, err := altsource.Build(releases)
	if err != nil {
		fmt.Fprintln(os.Stderr, "altstore-source:", err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(src); err != nil {
		fmt.Fprintln(os.Stderr, "altstore-source:", err)
		os.Exit(1)
	}
}

func listReleases() ([]altsource.Release, error) {
	// One page of 100 is years of releases for the feed's purpose; AltStore
	// only installs versions[0] and shows the rest as history.
	req, err := http.NewRequest("GET",
		"https://api.github.com/repos/ezcdlabs/clarity/releases?per_page=100", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "clarity-altstore-source")
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list releases: HTTP %d", resp.StatusCode)
	}

	var raw []struct {
		TagName     string    `json:"tag_name"`
		PublishedAt time.Time `json:"published_at"`
		Draft       bool      `json:"draft"`
		Prerelease  bool      `json:"prerelease"`
		Assets      []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]altsource.Release, 0, len(raw))
	for _, r := range raw {
		if r.Draft || r.Prerelease {
			continue
		}
		rel := altsource.Release{Tag: r.TagName, PublishedAt: r.PublishedAt}
		for _, a := range r.Assets {
			rel.Assets = append(rel.Assets, altsource.Asset{Name: a.Name, URL: a.URL, Size: a.Size})
		}
		out = append(out, rel)
	}
	return out, nil
}
