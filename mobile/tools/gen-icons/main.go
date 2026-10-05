// Command gen-icons writes the app icons for both platforms.
//
// The output is committed, like generated protobuf: the mark lives in
// mobile/internal/icon as geometry, this writes it out at every size each
// platform wants, and `go run ./mobile/tools/gen-icons` is how you regenerate
// after changing it. Nothing in a build depends on this running.
package main

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/ezcdlabs/clarity/mobile/internal/icon"
)

const (
	androidRes = "mobile/apps/android/app/src/main/res"
	iosAssets  = "mobile/apps/ios/Clarity/Assets.xcassets"
)

// Android densities, as the multiple of mdpi each one is.
var densities = []struct {
	name  string
	scale int
}{
	{"mdpi", 1},
	{"hdpi", 2}, // 1.5×, corrected below
	{"xhdpi", 2},
	{"xxhdpi", 3},
	{"xxxhdpi", 4},
}

func main() {
	root, err := repoRoot()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		log.Fatal(err)
	}

	for _, d := range androidSizes() {
		dir := filepath.Join(androidRes, "mipmap-"+d.name)
		// The legacy icons are the mark itself. Android 7, which minSdk still
		// supports, does not mask them — and it does not need to, because the
		// mark is already a disc.
		write(filepath.Join(dir, "ic_launcher.png"), icon.Logo(d.legacy, icon.OnNothing, 1))
		write(filepath.Join(dir, "ic_launcher_round.png"), icon.Logo(d.legacy, icon.OnNothing, 1))
		// The adaptive foreground is the checks alone, shrunk into the safe
		// zone. Its navy comes from the background layer, which Android paints
		// and moves independently.
		write(filepath.Join(dir, "ic_launcher_foreground.png"),
			icon.Logo(d.adaptive, icon.InkOnly, icon.ForegroundScale))
	}

	// The launch screen's mark keeps its disc, unlike the adaptive foreground.
	// The launch ground follows the theme, and on a light one an ink-only mark
	// would hand the white check a white background to disappear into.
	//
	// Android gives a splash icon a 288dp canvas and guarantees only the inner
	// 192dp. Whether it masks is not something to rely on, so the mark is sized
	// to survive one either way.
	for _, d := range densities {
		write(filepath.Join(androidRes, "mipmap-"+d.name, "splash_mark.png"),
			icon.Logo(splashSize(d.name), icon.OnNothing, icon.SplashScale))
	}

	// The status bar icon. Android draws it as a white mask at 24dp, which is
	// why it is the check alone and why it is a vector: a silhouette has no
	// colours left to tell three overlapping checks apart, and a mask that is
	// recoloured and rescaled by the system should not be a bitmap.
	writeFile(filepath.Join(androidRes, "drawable", "ic_stat_clarity.xml"), notificationXML())

	writeFile(filepath.Join(androidRes, "mipmap-anydpi-v26", "ic_launcher.xml"), adaptiveXML)
	writeFile(filepath.Join(androidRes, "mipmap-anydpi-v26", "ic_launcher_round.xml"), adaptiveXML)
	writeFile(filepath.Join(androidRes, "values", "ic_launcher_background.xml"), backgroundXML())

	// One 1024px icon and let Xcode downscale: the single-size app icon has
	// been the supported form since Xcode 14, and a folder of fourteen sizes is
	// fourteen things to regenerate wrongly. Filled rather than transparent —
	// iOS rejects an app icon with any transparency at all, and the navy
	// reaching the corners is the same navy the disc is made of.
	write(filepath.Join(iosAssets, "AppIcon.appiconset", "icon-1024.png"),
		icon.Logo(1024, icon.OnNavy, 1))
	writeFile(filepath.Join(iosAssets, "AppIcon.appiconset", "Contents.json"), appIconContents)
	writeFile(filepath.Join(iosAssets, "LaunchBackground.colorset", "Contents.json"), launchColorContents())
	writeFile(filepath.Join(iosAssets, "Contents.json"), catalogContents)
}

type androidSize struct {
	name     string
	legacy   int // a 48dp launcher icon
	adaptive int // a 108dp adaptive foreground
}

// splashSize is 288dp at each density, the canvas Android gives a splash icon.
func splashSize(density string) int {
	switch density {
	case "mdpi":
		return 288
	case "hdpi":
		return 432
	case "xhdpi":
		return 576
	case "xxhdpi":
		return 864
	default:
		return 1152
	}
}

func androidSizes() []androidSize {
	out := make([]androidSize, 0, len(densities))
	for _, d := range densities {
		out = append(out, androidSize{name: d.name, legacy: 48 * d.scale, adaptive: 108 * d.scale})
	}
	// hdpi is 1.5×, the one density that is not a whole multiple.
	out[1].legacy, out[1].adaptive = 72, 162
	return out
}

func write(path string, img image.Image) {
	mkdirAll(path)
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	fmt.Println(path)
}

func writeFile(path, content string) {
	mkdirAll(path)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Println(path)
}

func mkdirAll(path string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
}

// repoRoot walks up from the working directory until it finds go.mod, so this
// runs the same from anywhere.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

const adaptiveXML = `<?xml version="1.0" encoding="utf-8"?>
<!-- Generated by mobile/tools/gen-icons. Do not edit. -->
<adaptive-icon xmlns:android="http://schemas.android.com/apk/res/android">
    <background android:drawable="@color/ic_launcher_background" />
    <foreground android:drawable="@mipmap/ic_launcher_foreground" />
</adaptive-icon>
`

func backgroundXML() string {
	n := icon.Navy
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<!-- Generated by mobile/tools/gen-icons. Do not edit. -->
<resources>
    <color name="ic_launcher_background">#%02X%02X%02X</color>
    <color name="brand_navy">#%02X%02X%02X</color>
</resources>
`, n.R, n.G, n.B, n.R, n.G, n.B)
}

// launchColorContents is a two-appearance colour set, so the iOS launch screen
// follows the system the way the Android one does.
func launchColorContents() string {
	return `{
  "colors" : [
    {
      "color" : {
        "color-space" : "srgb",
        "components" : { "alpha" : "1.000", "blue" : "0xED", "green" : "0xED", "red" : "0xED" }
      },
      "idiom" : "universal"
    },
    {
      "appearances" : [
        { "appearance" : "luminosity", "value" : "dark" }
      ],
      "color" : {
        "color-space" : "srgb",
        "components" : { "alpha" : "1.000", "blue" : "0x1A", "green" : "0x1A", "red" : "0x1A" }
      },
      "idiom" : "universal"
    }
  ],
  "info" : {
    "author" : "mobile/tools/gen-icons",
    "version" : 1
  }
}
`
}

const appIconContents = `{
  "images" : [
    {
      "filename" : "icon-1024.png",
      "idiom" : "universal",
      "platform" : "ios",
      "size" : "1024x1024"
    }
  ],
  "info" : {
    "author" : "mobile/tools/gen-icons",
    "version" : 1
  }
}
`

const catalogContents = `{
  "info" : {
    "author" : "mobile/tools/gen-icons",
    "version" : 1
  }
}
`

// notificationXML is the status bar icon: the mark's check, white, on the 24dp
// canvas Android gives a small icon.
//
// The 2dp margin is Android's own guidance and it is not decoration — the
// system draws its own badge furniture around a small icon, and a mark pressed
// to the edge of the canvas collides with it.
//
// No tint attribute: the system recolours a notification icon itself, using
// only the alpha channel, and a theme reference here would be asked to resolve
// somewhere there is no theme.
func notificationXML() string {
	const size, margin = 24.0, 2.0
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<!-- Generated by mobile/tools/gen-icons from mobile/internal/icon. Do not edit. -->
<vector xmlns:android="http://schemas.android.com/apk/res/android"
    android:width="%.0fdp"
    android:height="%.0fdp"
    android:viewportWidth="%.0f"
    android:viewportHeight="%.0f">
    <path
        android:fillColor="@android:color/white"
        android:pathData="%s" />
</vector>
`, size, size, size, size, icon.CheckPath(size, margin))
}
