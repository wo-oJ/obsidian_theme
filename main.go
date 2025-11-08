package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"os/exec"
)

const versionManifestURL = "https://launchermeta.mojang.com/mc/game/version_manifest.json"

type VersionRef struct {
	Id  string `json:"id"`
	Url string `json:"url"`
}

type VersionManifest struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
	Versions []VersionRef `json:"versions"`
}

type VersionJSON struct {
	Id      string `json:"id"`
	Assets  string `json:"assets"`
	Downloads struct {
		Client struct {
			Sha1 string `json:"sha1"`
			Size int    `json:"size"`
			Url  string `json:"url"`
		} `json:"client"`
	} `json:"downloads"`
}

func fetchJSON(url string, v interface{}) error {
	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d fetching %s", res.StatusCode, url)
	}
	dec := json.NewDecoder(res.Body)
	return dec.Decode(v)
}

func downloadFile(url, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		// already exists
		return nil
	}
	out, err := os.Create(dest + ".partial")
	if err != nil {
		return err
	}
	defer out.Close()

	client := &http.Client{Timeout: 0}
	res, err := client.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", res.StatusCode)
	}
	_, err = io.Copy(out, res.Body)
	if err != nil {
		return err
	}
	out.Close()
	return os.Rename(dest+".partial", dest)
}

func ensureDir(p string) error {
	return os.MkdirAll(p, 0o755)
}

func findVersionURL(manifest *VersionManifest, id string) (string, error) {
	for _, v := range manifest.Versions {
		if v.Id == id {
			return v.Url, nil
		}
	}
	return "", errors.New("version not found")
}

func main() {
	version := flag.String("version", "", "Minecraft version ID to install (e.g. 1.20.2). If empty, uses latest release.")
	runOffline := flag.Bool("run-offline", false, "After download, attempt to run the client JAR (offline test).")
	minecraftDir := flag.String("mcdir", filepath.Join(os.Getenv("HOME"), ".minecraft"), "Minecraft game directory")
	username := flag.String("username", "Player", "Offline username when running")
	flag.Parse()

	fmt.Println("Fetching version manifest...")
	var manifest VersionManifest
	if err := fetchJSON(versionManifestURL, &manifest); err != nil {
		fmt.Fprintln(os.Stderr, "failed to fetch manifest:", err)
		os.Exit(1)
	}

	var vid string
	if *version == "" {
		vid = manifest.Latest.Release
		fmt.Println("No version specified; using latest release:", vid)
	} else {
		vid = *version
	}

	vurl, err := findVersionURL(&manifest, vid)
	if err != nil {
		fmt.Fprintln(os.Stderr, "version not found in manifest:", vid)
		os.Exit(1)
	}

	fmt.Println("Fetching version JSON for", vid)
	var vjson VersionJSON
	if err := fetchJSON(vurl, &vjson); err != nil {
		fmt.Fprintln(os.Stderr, "failed to fetch version json:", err)
		os.Exit(1)
	}

	versionDir := filepath.Join(*minecraftDir, "versions", vid)
	if err := ensureDir(versionDir); err != nil {
		fmt.Fprintln(os.Stderr, "failed to create version dir:", err)
		os.Exit(1)
	}

	jarPath := filepath.Join(versionDir, vid+".jar")
	fmt.Println("Downloading client jar to", jarPath)
	if err := downloadFile(vjson.Downloads.Client.Url, jarPath); err != nil {
		fmt.Fprintln(os.Stderr, "failed to download client jar:", err)
		os.Exit(1)
	}
	fmt.Println("Downloaded.")

	if *runOffline {
		fmt.Println("Attempting to run JAR offline (minimal test). You must have Java installed and on PATH.")
		cmd := exec.Command("java", "-jar", jarPath)
		// redirect output so you can see it
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		// NOTE: most modern versions require libraries/natives/classpath; this is a simple test.
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "java failed:", err)
			fmt.Fprintln(os.Stderr, "For a real launcher you must build the full classpath (libraries), extract natives, and pass proper args like --username, --version, --gameDir, --assetsDir, --accessToken, etc.")
			os.Exit(1)
		}
	}

	fmt.Println("Done. Version installed to", versionDir)
	fmt.Println("Next steps: assemble libraries, extract natives and implement Microsoft/Xbox OAuth for online play.")
}
