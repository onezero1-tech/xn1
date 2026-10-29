package core

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// mihomo releases are published on GitHub as versioned per-platform zips. The
// client keeps a known-good default so a fresh install can provision the core
// without the user hunting for a binary.
const (
	releasesAPI = "https://api.github.com/repos/MetaCubeX/mihomo/releases"
	// defaultVersion is the release tag the client falls back to when no core
	// is present and the network cannot be reached to list newer ones.
	// v1.19.32 is the official latest stable (2026-09-30): the first line that
	// carries every protocol this client renders (wireguard+amnezia-wg-option,
	// masque h2/h3, dialer-proxy chains).
	defaultVersion = "v1.19.32"
)

// ostuple returns the mihomo release asset fragment for this platform.
func ostuple() (osPart, archPart, exeName string) {
	archPart = runtime.GOARCH
	switch archPart {
	case "amd64", "arm64":
	default:
		archPart = "amd64"
	}
	exeName = "mihomo"
	switch runtime.GOOS {
	case "windows":
		osPart = "windows"
		exeName = "mihomo.exe"
	case "linux":
		osPart = "linux"
	case "darwin":
		osPart = "darwin"
	default:
		osPart = runtime.GOOS
	}
	return osPart, archPart, exeName
}

// ExeName returns the core binary filename for this host.
func ExeName() string {
	_, _, n := ostuple()
	return n
}

// CoreInDir returns the expected core binary path inside dir.
func CoreInDir(dir string) string {
	return filepath.Join(dir, ExeName())
}

// release is the subset of the GitHub releases API we consume.
type release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// EnsureBinary guarantees a working core binary in workDir. When one is
// already present it is kept; otherwise the caller may supply localPath, and
// failing that the client downloads the default (or a newer) release.
func EnsureBinary(workDir, localPath string) (string, error) {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", err
	}
	candidate := CoreInDir(workDir)
	if isExecutable(candidate) {
		return candidate, nil
	}
	if localPath != "" {
		if isExecutable(localPath) {
			return localPath, nil
		}
		return "", fmt.Errorf("core: %s is not an executable mihomo binary", localPath)
	}
	// No local core: download a release.
	if err := downloadRelease(workDir); err != nil {
		return "", fmt.Errorf("core: no mihomo binary and download failed: %w", err)
	}
	if isExecutable(candidate) {
		return candidate, nil
	}
	return "", fmt.Errorf("core: download completed but %s is missing", candidate)
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() || fi.Size() == 0 {
		return false
	}
	// The exec bit is what execve actually checks on Linux: a 0644 file makes
	// the kernel refuse with EACCES ("权限不够") even for the owner. mihomo's
	// release zips do not always carry it, and a chmod-less file would leave
	// the client looping into the download path forever.
	return fi.Mode().Perm()&0o111 != 0
}

// downloadRelease fetches the latest release (falling back to defaultVersion)
// and unpacks the archive that matches this host's OS+arch.
func downloadRelease(workDir string) error {
	client := httpClient(30 * time.Second)
	version, assetURL, err := pickAsset(client)
	if err != nil {
		return err
	}
	resp, err := client.Get(assetURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("core: download %s: HTTP %d", assetURL, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 512<<20))
	if err != nil {
		return err
	}
	_ = version
	return unpackArchive(data, workDir)
}

func pickAsset(client *http.Client) (version, url string, err error) {
	if v, u, ok := releaseFor(client, defaultVersion); ok {
		return v, u, nil
	}
	// Fall back to listing latest when the pinned one 404s (e.g. removed).
	req, _ := http.NewRequest(http.MethodGet, releasesAPI+"/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var r release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", "", err
	}
	return matchingAsset(r)
}

func releaseFor(client *http.Client, tag string) (string, string, bool) {
	req, _ := http.NewRequest(http.MethodGet, releasesAPI+"/tags/"+tag, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", false
	}
	var r release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", "", false
	}
	v, u, err := matchingAsset(r)
	return v, u, err == nil
}

// assetRank scores how well an asset name suits the current platform. The
// ranking only ever runs over names containing the <os>-<arch> needle, so the
// windows-only / non-windows-only spread never confuses it.
func assetRank(name string) int {
	switch {
	case strings.HasSuffix(name, ".zip"):
		return 3 // windows: the exe ships as a plain zip
	case strings.HasSuffix(name, ".gz"):
		return 2 // linux/darwin/freebsd: the raw binary rides inside a gzip
	case strings.HasSuffix(name, ".deb"),
		strings.HasSuffix(name, ".rpm"),
		strings.HasSuffix(name, ".pkg.tar.zst"):
		return 1 // distro packages: usable but heavier and arch-qualified
	}
	return 0
}

// matchingAsset finds the asset for <os>-<arch>. Windows ships a zip; every
// other platform ships a gzipped raw binary, so the ".zip" suffix alone used to
// make the lookup fail on linux with "no linux-amd64 asset in release".
func matchingAsset(r release) (string, string, error) {
	osPart, archPart, _ := ostuple()
	needle := osPart + "-" + archPart
	var fallbackURL string
	var fallbackName string
	for _, a := range r.Assets {
		if !strings.Contains(a.Name, needle) {
			continue
		}
		rank := assetRank(a.Name)
		if rank == 0 {
			continue
		}
		switch rank {
		case 3:
			return r.TagName, a.BrowserDownloadURL, nil
		case 2:
			if fallbackURL == "" || assetNewer(a.Name, fallbackName) {
				fallbackURL = a.BrowserDownloadURL
				fallbackName = a.Name
			}
		}
	}
	if fallbackURL == "" {
		return "", "", fmt.Errorf("core: no %s asset in release %s", needle, r.TagName)
	}
	return r.TagName, fallbackURL, nil
}

// assetNewer reports whether asset a carries a newer bare version than b,
// e.g. "mihomo-linux-amd64-v1.19.33.gz" outranks "mihomo-linux-amd64-v1.19.32.gz".
// Asset names carrying Go/toolchain or build-variant suffixes ("v1-go120",
// "-compatible", "-softfloat") are still compared by the bare trailing version.
func assetNewer(a, b string) bool {
	av := bareVersion(a)
	if av == nil {
		return false
	}
	bv := bareVersion(b)
	if bv == nil {
		return false
	}
	for i := range *av {
		if (*av)[i] != (*bv)[i] {
			return (*av)[i] > (*bv)[i]
		}
	}
	return false
}

// bareVersion extracts the trailing "vX.Y.Z" from an asset file name.
func bareVersion(name string) *[]int {
	i := strings.LastIndex(name, "v")
	if i < 0 {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(name[i:], "v"), ".")
	var out []int
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return &out
}

func unpackArchive(data []byte, workDir string) error {
	// The linux/darwin/freebsd release assets are gzipped raw ELFs; the
	// windows ones are zips. Sniff the gzip magic instead of guessing from the
	// URL so both families unpack through the same entry point.
	if len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b {
		return unpackGzip(data, workDir)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		name := filepath.Base(f.Name)
		// Keep the exe plus the geodata mihomo needs for rule-based routing.
		if name != ExeName() && name != geoSiteName && name != geoIPName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(filepath.Join(workDir, name), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
	}
	return nil
}

// unpackGzip inflates a release asset and installs the bare binary into
// workDir under ExeName().
func unpackGzip(data []byte, workDir string) error {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	out, err := os.OpenFile(CoreInDir(workDir), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, zr); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
