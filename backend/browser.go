package backend

import (
	"Gleip/backend/cert"
	"Gleip/backend/paths"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"syscall"
)

// DownloadProgress represents the current progress of a Firefox download
type DownloadProgress struct {
	Progress float64 `json:"progress"`
	Status   string  `json:"status"`
	Error    string  `json:"error,omitempty"`
}

// Variables for Firefox download/installation status
var currentDownloadProgress DownloadProgress
var downloadInProgress bool

// CheckFirefoxInstallation checks if Firefox is installed in the app directory
func (a *App) CheckFirefoxInstallation() bool {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	// Check platform-specific Firefox location
	var browserPath string
	switch stdruntime.GOOS {
	case "darwin":
		browserPath = filepath.Join(homeDir, "Library", "Application Support", "Gleip", "browsers", "firefox", "Firefox.app")
	case "linux":
		browserPath = filepath.Join(homeDir, ".config", "Gleip", "browsers", "firefox", "firefox")
	case "windows":
		browserPath = filepath.Join(homeDir, "AppData", "Local", "Gleip", "browsers", "firefox", "firefox.exe")
	default:
		return false
	}

	_, err = os.Stat(browserPath)
	return err == nil
}

// GetFirefoxDownloadProgress returns the current Firefox download progress
func (a *App) GetFirefoxDownloadProgress() DownloadProgress {
	return currentDownloadProgress
}

// IsFirefoxDownloading checks if Firefox is currently being downloaded
func (a *App) IsFirefoxDownloading() bool {
	return downloadInProgress
}

// DownloadFirefox downloads Firefox to the .gleip/browsers directory
func (a *App) DownloadFirefox() error {
	// Prevent multiple simultaneous downloads
	if downloadInProgress {
		return fmt.Errorf("download already in progress")
	}

	// Track only the fact of download starting - no sensitive data needed
	TrackFirefoxAction("download_started", true)

	downloadInProgress = true
	currentDownloadProgress = DownloadProgress{
		Progress: 0.0,
		Status:   "Starting download...",
	}

	// Start download in a goroutine
	go func() {
		err := a.downloadFirefoxInternal()
		if err != nil {
			currentDownloadProgress.Status = "Failed"
			currentDownloadProgress.Error = err.Error()
			// Track error with metadata approach
			TrackError("firefox_download", "download_error")
		} else {
			currentDownloadProgress.Progress = 1.0
			currentDownloadProgress.Status = "Completed"
			TrackFirefoxAction("download_completed", true)
		}
		downloadInProgress = false
	}()

	return nil
}

// downloadFirefoxInternal downloads and installs Firefox with proxy configuration
func (a *App) downloadFirefoxInternal() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %v", err)
	}

	// Create directories using platform-specific paths
	var browserDir string
	switch stdruntime.GOOS {
	case "darwin":
		browserDir = filepath.Join(homeDir, "Library", "Application Support", "Gleip", "browsers", "firefox")
	case "linux":
		browserDir = filepath.Join(homeDir, ".config", "Gleip", "browsers", "firefox")
	case "windows":
		browserDir = filepath.Join(homeDir, "AppData", "Local", "Gleip", "browsers", "firefox")
	default:
		return fmt.Errorf("unsupported platform: %s", stdruntime.GOOS)
	}

	installerCacheDir := filepath.Join(browserDir, "installer")
	if err := os.MkdirAll(browserDir, 0755); err != nil {
		return fmt.Errorf("failed to create browser directory: %v", err)
	}
	if err := os.MkdirAll(installerCacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create installer cache directory: %v", err)
	}

	// Determine installer filename based on platform
	var installerFilename string
	switch stdruntime.GOOS {
	case "darwin":
		installerFilename = "firefox.dmg"
	case "linux":
		installerFilename = "firefox-linux.tar.xz" // Updated to XZ format
	case "windows":
		installerFilename = "firefox-installer.exe"
	default:
		return fmt.Errorf("unsupported platform: %s", stdruntime.GOOS)
	}

	// Check if installer is already cached
	cachedInstallerPath := filepath.Join(installerCacheDir, installerFilename)
	installerExists := false
	if fileInfo, err := os.Stat(cachedInstallerPath); err == nil && fileInfo.Size() > 0 {
		installerExists = true
		fmt.Printf("INFO: Found cached Firefox installer at %s (size: %d bytes)\n",
			cachedInstallerPath, fileInfo.Size())
		currentDownloadProgress.Status = "Using cached Firefox installer..."
		currentDownloadProgress.Progress = 0.75 // Skip ahead in progress
	}

	var downloadPath string
	if !installerExists {
		// Create temp directory for download
		tempDir, err := os.MkdirTemp("", "gleip-firefox-download")
		if err != nil {
			return fmt.Errorf("failed to create temp directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Update progress
		currentDownloadProgress.Status = "Determining download URL..."
		currentDownloadProgress.Progress = 0.05

		// Get download URL from centralized paths configuration
		downloadURL, err := paths.GetFirefoxDownloadURL()
		if err != nil {
			return err
		}

		// Create HTTP client with redirect following
		client := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return nil // Allow redirects
			},
		}

		// Create request
		req, err := http.NewRequest("GET", downloadURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %v", err)
		}

		// Set user agent
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/115.0")

		// Update progress
		currentDownloadProgress.Status = "Connecting to Mozilla server..."
		currentDownloadProgress.Progress = 0.1

		// Send request
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to download Firefox: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to download Firefox, status code: %d", resp.StatusCode)
		}

		// Get content length for progress tracking
		contentLength := resp.ContentLength

		// Create temp file for download
		downloadPath = filepath.Join(tempDir, installerFilename)

		currentDownloadProgress.Status = "Downloading Firefox..."
		currentDownloadProgress.Progress = 0.15

		file, err := os.Create(downloadPath)
		if err != nil {
			return fmt.Errorf("failed to create download file: %v", err)
		}

		// Download with progress updates
		downloadBuf := make([]byte, 1024*1024) // 1MB buffer
		var downloaded int64

		for {
			n, err := resp.Body.Read(downloadBuf)
			if n > 0 {
				// Write to file
				if _, err := file.Write(downloadBuf[:n]); err != nil {
					file.Close()
					return fmt.Errorf("failed to write to file: %v", err)
				}

				// Update progress
				downloaded += int64(n)
				if contentLength > 0 {
					progress := float64(downloaded) / float64(contentLength)
					// Download is 60% of the process (from 0.15 to 0.75)
					currentDownloadProgress.Progress = 0.15 + (progress * 0.6)
				}
			}

			if err != nil {
				if err == io.EOF {
					break
				}
				file.Close()
				return fmt.Errorf("download error: %v", err)
			}
		}

		file.Close()

		// Cache the installer file
		currentDownloadProgress.Status = "Caching installer for future use..."
		if err := os.MkdirAll(filepath.Dir(cachedInstallerPath), 0755); err != nil {
			fmt.Printf("WARNING: Failed to create installer cache directory: %v\n", err)
		} else {
			// Copy the downloaded file to the cache location
			if err := copyFile(downloadPath, cachedInstallerPath); err != nil {
				fmt.Printf("WARNING: Failed to cache installer: %v\n", err)
			} else {
				fmt.Printf("INFO: Cached Firefox installer to %s\n", cachedInstallerPath)
			}
		}

		currentDownloadProgress.Status = "Download complete, installing..."
		currentDownloadProgress.Progress = 0.75
	} else {
		// Use cached installer
		downloadPath = cachedInstallerPath
	}

	// Extract Firefox based on platform
	switch stdruntime.GOOS {
	case "darwin":
		// Mount DMG
		currentDownloadProgress.Status = "Mounting disk image..."
		cmd := exec.Command("hdiutil", "attach", downloadPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to mount Firefox DMG: %v. Output: %s", err, string(output))
		}

		currentDownloadProgress.Progress = 0.8
		currentDownloadProgress.Status = "Copying Firefox to application folder..."

		// Copy Firefox to destination
		cmd = exec.Command("cp", "-R", "/Volumes/Firefox/Firefox.app", browserDir)
		output, err = cmd.CombinedOutput()
		if err != nil {
			// Try to detach DMG even if copy failed, log separately if detach fails
			detachCmd := exec.Command("hdiutil", "detach", "/Volumes/Firefox")
			if detachOutput, detachErr := detachCmd.CombinedOutput(); detachErr != nil {
				// Log this auxiliary error, but prioritize returning the original copy error
				fmt.Printf("Warning: failed to detach DMG after copy error: %v. Detach command output: %s\n", detachErr, string(detachOutput))
			}
			return fmt.Errorf("failed to copy Firefox: %v. Copy command output: %s", err, string(output))
		}

		currentDownloadProgress.Progress = 0.9
		currentDownloadProgress.Status = "Finishing installation..."

		// Detach DMG
		cmd = exec.Command("hdiutil", "detach", "/Volumes/Firefox")
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to detach Firefox DMG: %v. Output: %s", err, string(output))
		}

	case "linux":
		// Create extraction directory - use consistent path structure
		currentDownloadProgress.Status = "Extracting Firefox..."

		// Use the same base directory structure as the browser directory
		var tempExtractDir string
		switch stdruntime.GOOS {
		case "linux":
			tempExtractDir = filepath.Join(homeDir, ".config", "Gleip", "temp-extract")
		default:
			tempExtractDir = filepath.Join(homeDir, ".gleip", "temp-extract")
		}

		if err := os.MkdirAll(tempExtractDir, 0755); err != nil {
			return fmt.Errorf("failed to create extraction directory: %v", err)
		}
		defer func() {
			if err := os.RemoveAll(tempExtractDir); err != nil {
				fmt.Printf("Warning: Failed to clean up temp directory: %v\n", err)
			}
		}()

		// Extract tar.xz with proper compression detection
		fmt.Printf("INFO: Extracting Firefox from %s to %s\n", downloadPath, tempExtractDir)

		// Verify the download file exists and has content
		if fileInfo, err := os.Stat(downloadPath); err != nil {
			return fmt.Errorf("download file not found: %v", err)
		} else {
			fmt.Printf("INFO: Download file size: %d bytes\n", fileInfo.Size())
			if fileInfo.Size() == 0 {
				return fmt.Errorf("download file is empty")
			}
		}

		// Detect file type for proper extraction
		cmd := exec.Command("file", downloadPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Warning: Failed to detect file type: %v\n", err)
		} else {
			fmt.Printf("INFO: Detected file type: %s\n", string(output))
		}

		// Use XZ decompression for the expected .tar.xz format
		var extractCmd *exec.Cmd
		if strings.Contains(string(output), "xz compressed") || strings.HasSuffix(downloadPath, ".tar.xz") {
			fmt.Printf("INFO: Using XZ decompression\n")
			extractCmd = exec.Command("tar", "-xJf", downloadPath, "-C", tempExtractDir)
		} else if strings.Contains(string(output), "bzip2 compressed") || strings.HasSuffix(downloadPath, ".tar.bz2") {
			fmt.Printf("INFO: Using bzip2 decompression\n")
			extractCmd = exec.Command("tar", "-xjf", downloadPath, "-C", tempExtractDir)
		} else if strings.Contains(string(output), "gzip compressed") || strings.HasSuffix(downloadPath, ".tar.gz") {
			fmt.Printf("INFO: Using gzip decompression\n")
			extractCmd = exec.Command("tar", "-xzf", downloadPath, "-C", tempExtractDir)
		} else {
			// Default to XZ for Firefox downloads
			fmt.Printf("INFO: Using default XZ decompression\n")
			extractCmd = exec.Command("tar", "-xJf", downloadPath, "-C", tempExtractDir)
		}

		output, err = extractCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to extract Firefox: %v. Output: %s", err, string(output))
		}
		fmt.Printf("INFO: Extraction completed successfully\n")

		// Verify extraction was successful
		firefoxExtractedPath := filepath.Join(tempExtractDir, "firefox")
		if _, err := os.Stat(firefoxExtractedPath); os.IsNotExist(err) {
			// List what was actually extracted
			fmt.Printf("DEBUG: Listing contents of extraction directory %s:\n", tempExtractDir)
			if files, err := os.ReadDir(tempExtractDir); err == nil {
				for _, file := range files {
					fmt.Printf("  - %s\n", file.Name())
				}
			}
			return fmt.Errorf("firefox directory not found after extraction at %s", firefoxExtractedPath)
		}
		fmt.Printf("INFO: Firefox extracted successfully to %s\n", firefoxExtractedPath)

		currentDownloadProgress.Progress = 0.85
		currentDownloadProgress.Status = "Installing Firefox..."

		// Ensure the destination directory exists
		if err := os.MkdirAll(filepath.Dir(browserDir), 0755); err != nil {
			return fmt.Errorf("failed to create browser parent directory: %v", err)
		}

		// Remove any existing Firefox installation
		if _, err := os.Stat(browserDir); err == nil {
			fmt.Printf("INFO: Removing existing Firefox installation at %s\n", browserDir)
			if err := os.RemoveAll(browserDir); err != nil {
				return fmt.Errorf("failed to remove existing Firefox installation: %v", err)
			}
		}

		// Copy Firefox to destination using Go's file operations for better error handling
		fmt.Printf("INFO: Copying Firefox from %s to %s\n", firefoxExtractedPath, browserDir)
		if err := copyDirectory(firefoxExtractedPath, browserDir); err != nil {
			return fmt.Errorf("failed to copy Firefox: %v", err)
		}

		// Verify the Firefox binary exists and is executable
		firefoxBinary := filepath.Join(browserDir, "firefox")
		if _, err := os.Stat(firefoxBinary); os.IsNotExist(err) {
			return fmt.Errorf("firefox binary not found at %s after installation", firefoxBinary)
		}

		// Make sure the Firefox binary is executable
		if err := os.Chmod(firefoxBinary, 0755); err != nil {
			fmt.Printf("Warning: Failed to make Firefox binary executable: %v\n", err)
		}

		fmt.Printf("INFO: Firefox successfully installed to %s\n", browserDir)

	case "windows":
		// Windows installation is more complex
		currentDownloadProgress.Status = "Setting up Firefox installer..."

		// For Windows, we just keep the installer - no need to copy it again
		// if we already cached it
		if !installerExists {
			// Copy the installer to the regular firefox directory
			installerDest := filepath.Join(browserDir, "firefox-installer.exe")
			if err := copyFile(downloadPath, installerDest); err != nil {
				return fmt.Errorf("failed to copy installer: %v", err)
			}
		}

		// Create a batch file to run the installer silently
		batchContent := `@echo off
firefox-installer.exe /S /D=%~dp0
`
		if err := os.WriteFile(filepath.Join(browserDir, "install.bat"), []byte(batchContent), 0755); err != nil {
			return fmt.Errorf("failed to create batch file: %v", err)
		}

		// Note: We don't actually run the installer here as it requires elevation
		// We'll inform the user they need to run it manually
	}

	// Setup the Firefox profile
	profileDir := paths.GlobalPaths.FirefoxProfileDir
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return fmt.Errorf("failed to create profile directory: %v", err)
	}

	// Create user.js file with proxy settings and complete privacy configuration
	// Get absolute path for proxy.pac file
	absProfileDir, err := filepath.Abs(profileDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for profile directory: %v", err)
	}
	proxyPacPath := filepath.Join(absProfileDir, "proxy.pac")

	userJSContent := fmt.Sprintf(`// Gleip Firefox Configuration - Complete Privacy & Proxy Setup
// Based on Mozilla's guide to stop automatic connections

// === STARTUP & HOMEPAGE ===
user_pref("browser.startup.homepage", "about:blank");
user_pref("browser.startup.page", 0);
user_pref("browser.startup.homepage_override.mstone", "ignore");
user_pref("browser.shell.checkDefaultBrowser", false);

// === PROXY SETTINGS ===
// Use automatic proxy configuration to block Mozilla domains and route traffic through Gleip
user_pref("network.proxy.type", 2);
user_pref("network.proxy.autoconfig_url", "file://%s");
user_pref("network.proxy.autoconfig_retry_interval_min", 5);
user_pref("network.proxy.autoconfig_retry_interval_max", 300);
user_pref("network.proxy.no_proxies_on", "localhost, 127.0.0.1");
user_pref("network.proxy.socks_remote_dns", true);

// === DISABLE ALL AUTO-UPDATES ===
user_pref("app.update.enabled", false);
user_pref("app.update.auto", false);
user_pref("app.update.mode", 0);
user_pref("app.update.service.enabled", false);
user_pref("extensions.update.enabled", false);
user_pref("extensions.update.autoUpdateDefault", false);
user_pref("extensions.systemAddon.update.enabled", false);

// === DISABLE BLOCKLIST UPDATES ===
user_pref("extensions.blocklist.enabled", false);
user_pref("extensions.blocklist.url", "");

// === DISABLE SAFE BROWSING & MALWARE PROTECTION ===
user_pref("browser.safebrowsing.enabled", false);
user_pref("browser.safebrowsing.phishing.enabled", false);
user_pref("browser.safebrowsing.malware.enabled", false);
user_pref("browser.safebrowsing.downloads.enabled", false);
user_pref("browser.safebrowsing.downloads.remote.enabled", false);
user_pref("browser.safebrowsing.downloads.remote.url", "");
user_pref("browser.safebrowsing.downloads.remote.block_potentially_unwanted", false);
user_pref("browser.safebrowsing.downloads.remote.block_uncommon", false);
user_pref("browser.safebrowsing.provider.google.updateURL", "");
user_pref("browser.safebrowsing.provider.google.gethashURL", "");
user_pref("browser.safebrowsing.provider.google4.updateURL", "");
user_pref("browser.safebrowsing.provider.google4.gethashURL", "");
user_pref("browser.safebrowsing.provider.mozilla.updateURL", "");
user_pref("browser.safebrowsing.provider.mozilla.gethashURL", "");

// === DISABLE TRACKING PROTECTION UPDATES ===
user_pref("privacy.trackingprotection.enabled", false);
user_pref("privacy.trackingprotection.pbmode.enabled", false);
user_pref("urlclassifier.trackingTable", "");
user_pref("urlclassifier.trackingWhitelistTable", "");

// === DISABLE CERTIFICATE VERIFICATION & OCSP ===
user_pref("security.OCSP.enabled", 0);
user_pref("security.OCSP.require", false);
user_pref("security.ssl.enable_ocsp_stapling", false);
user_pref("security.ssl.enable_ocsp_must_staple", false);

// === DISABLE ALL PREFETCHING ===
user_pref("network.prefetch-next", false);
user_pref("network.dns.disablePrefetch", true);
user_pref("network.dns.disablePrefetchFromHTTPS", true);
user_pref("network.predictor.enabled", false);
user_pref("network.predictor.enable-hover-on-ssl", false);
user_pref("network.predictor.enable-prefetch", false);
user_pref("network.http.speculative-parallel-limit", 0);

// === DISABLE SEARCH SUGGESTIONS & URL BAR ===
user_pref("browser.search.suggest.enabled", false);
user_pref("browser.urlbar.suggest.searches", false);
user_pref("browser.urlbar.suggest.topsites", false);
user_pref("browser.urlbar.suggest.history", false);
user_pref("browser.urlbar.suggest.bookmark", false);
user_pref("browser.urlbar.suggest.openpage", false);
user_pref("browser.urlbar.suggest.remotetab", false);
user_pref("browser.urlbar.userMadeSearchSuggestionsChoice", true);
user_pref("keyword.enabled", false);
user_pref("browser.fixup.alternate.enabled", false);

// === DISABLE ALL TELEMETRY & DIAGNOSTICS ===
user_pref("toolkit.telemetry.enabled", false);
user_pref("toolkit.telemetry.unified", false);
user_pref("toolkit.telemetry.server", "data:,");
user_pref("toolkit.telemetry.archive.enabled", false);
user_pref("toolkit.telemetry.newProfilePing.enabled", false);
user_pref("toolkit.telemetry.shutdownPingSender.enabled", false);
user_pref("toolkit.telemetry.updatePing.enabled", false);
user_pref("toolkit.telemetry.bhrPing.enabled", false);
user_pref("toolkit.telemetry.firstShutdownPing.enabled", false);
user_pref("toolkit.telemetry.hybridContent.enabled", false);
user_pref("datareporting.healthreport.uploadEnabled", false);
user_pref("datareporting.policy.dataSubmissionEnabled", false);
user_pref("app.shield.optoutstudies.enabled", false);
user_pref("app.normandy.enabled", false);
user_pref("app.normandy.api_url", "");

// === DISABLE CRASH REPORTING ===
user_pref("breakpad.reportURL", "");
user_pref("browser.tabs.crashReporting.sendReport", false);
user_pref("browser.crashReports.unsubmittedCheck.enabled", false);
user_pref("browser.crashReports.unsubmittedCheck.autoSubmit2", false);

// === DISABLE NEW TAB PAGE & ACTIVITY STREAM ===
user_pref("browser.newtabpage.enabled", false);
user_pref("browser.newtabpage.enhanced", false);
user_pref("browser.newtabpage.activity-stream.enabled", false);
user_pref("browser.newtabpage.activity-stream.feeds.topsites", false);
user_pref("browser.newtabpage.activity-stream.feeds.section.highlights", false);
user_pref("browser.newtabpage.activity-stream.feeds.snippets", false);
user_pref("browser.newtabpage.activity-stream.feeds.telemetry", false);
user_pref("browser.newtabpage.activity-stream.telemetry", false);
user_pref("browser.newtabpage.activity-stream.feeds.discoverystreamfeed", false);
user_pref("browser.newtabpage.activity-stream.showSponsored", false);
user_pref("browser.newtabpage.activity-stream.showSponsoredTopSites", false);

// === DISABLE EXTENSIONS METADATA & RECOMMENDATIONS ===
user_pref("extensions.getAddons.cache.enabled", false);
user_pref("extensions.getAddons.showPane", false);
user_pref("extensions.htmlaboutaddons.recommendations.enabled", false);
user_pref("extensions.webservice.discoverURL", "");
user_pref("extensions.getAddons.discovery.api_url", "");

// === DISABLE LOCATION SERVICES ===
user_pref("geo.enabled", false);
user_pref("geo.provider.network.url", "");
user_pref("geo.provider.ms-windows-location", false);
user_pref("geo.provider.use_corelocation", false);
user_pref("geo.provider.use_gpsd", false);
user_pref("browser.region.network.url", "");
user_pref("browser.region.update.enabled", false);

// === DISABLE MEDIA CODECS & DRM ===
user_pref("media.gmp-gmpopenh264.enabled", false);
user_pref("media.gmp-manager.url", "");
user_pref("media.gmp-manager.updateEnabled", false);
user_pref("media.eme.enabled", false);
user_pref("media.gmp-widevinecdm.enabled", false);

// === DISABLE WEBRTC ===
user_pref("media.peerconnection.enabled", false);
user_pref("media.peerconnection.ice.default_address_only", true);
user_pref("media.peerconnection.ice.no_host", true);

// === DISABLE NETWORK DETECTION ===
user_pref("network.captive-portal-service.enabled", false);
user_pref("network.connectivity-service.enabled", false);
user_pref("captivedetect.canonicalURL", "");

// === DISABLE FIREFOX SYNC ===
user_pref("services.sync.enabled", false);
user_pref("identity.fxaccounts.enabled", false);

// === DISABLE POCKET ===
user_pref("extensions.pocket.enabled", false);
user_pref("extensions.pocket.api", "");
user_pref("extensions.pocket.site", "");

// === DISABLE FIREFOX SCREENSHOTS ===
user_pref("extensions.screenshots.disabled", true);

// === DISABLE FORM AUTOFILL ===
user_pref("extensions.formautofill.addresses.enabled", false);
user_pref("extensions.formautofill.creditCards.enabled", false);

// === DISABLE WEB NOTIFICATIONS ===
user_pref("dom.webnotifications.enabled", false);
user_pref("dom.push.enabled", false);

// === DISABLE VARIOUS OTHER CONNECTIONS ===
user_pref("network.allow-experiments", false);
user_pref("network.http.sendOriginHeader", 0);
user_pref("dom.security.https_only_mode", false);
user_pref("security.ssl.errorReporting.enabled", false);
user_pref("browser.ping-centre.telemetry", false);
user_pref("browser.newtabpage.activity-stream.asrouter.userprefs.cfr.addons", false);
user_pref("browser.newtabpage.activity-stream.asrouter.userprefs.cfr.features", false);
`, proxyPacPath)
	if err := os.WriteFile(filepath.Join(profileDir, "user.js"), []byte(userJSContent), 0644); err != nil {
		return fmt.Errorf("failed to create user.js file: %v", err)
	}

	// Create proxy.pac file to block remaining Mozilla connections that can't be disabled via preferences
	proxyPacContent := `// Gleip Proxy PAC file to block Mozilla automatic connections
// Based on https://www.abcdesktop.io/common/disable-firefox-connections/

function FindProxyForURL(url, host) {
    // Gleip proxy server for normal traffic
    var gleipProxy = "PROXY 127.0.0.1:9090";
    
    // Blackhole for blocked Mozilla domains - unreachable proxy
    var blackhole = "PROXY 127.0.0.1:12345";
    
    // Block specific Mozilla domains that cannot be disabled via preferences
    if (host == "firefox.settings.services.mozilla.com" ||
        host == "content-signature-2.cdn.mozilla.net" ||
        host == "firefox-settings-attachments.cdn.mozilla.net" ||
        host == "normandy.cdn.mozilla.net" ||
        host == "classify-client.services.mozilla.com"||
		host == "aus5.mozilla.org"||
		host == "aus4.mozilla.org"||
		host == "aus3.mozilla.org"||
		host == "aus2.mozilla.org"||
		host == "aus1.mozilla.org"||
		host == "aus0.mozilla.org") {
        return blackhole;
    }
    
    // Route all other traffic through Gleip proxy
    return gleipProxy;
}
`
	if err := os.WriteFile(filepath.Join(profileDir, "proxy.pac"), []byte(proxyPacContent), 0644); err != nil {
		return fmt.Errorf("failed to create proxy.pac file: %v", err)
	}

	// Also create a prefs.js to ensure profile is recognized
	prefsJSContent := `// Mozilla User Preferences
// This file is managed by Gleip

user_pref("app.update.enabled", false);
user_pref("browser.shell.checkDefaultBrowser", false);
user_pref("browser.startup.homepage_override.mstone", "ignore");
user_pref("browser.startup.page", 0);
`
	if err := os.WriteFile(filepath.Join(profileDir, "prefs.js"), []byte(prefsJSContent), 0644); err != nil {
		return fmt.Errorf("failed to create prefs.js file: %v", err)
	}

	// Install certificate using pre-built certificate database files
	currentDownloadProgress.Status = "Creating certificate databases..."
	currentDownloadProgress.Progress = 0.95

	// Create certificate database files directly in the profile
	certErr := a.copyPrebuiltCertDatabases(profileDir)
	if certErr != nil {
		// Certificate installation failure - report it clearly in the UI and console
		certError := fmt.Sprintf("❌ CERTIFICATE DATABASE ERROR: %v", certErr)
		fmt.Println(certError) // Console log
		currentDownloadProgress.Status = certError
		currentDownloadProgress.Error = certError
		currentDownloadProgress.Progress = 0.98

		// Return the error here to prevent proceeding without cert databases
		return certErr
	}

	// Copy the certificate directly to the profile directory
	caCertPath := a.proxyServer.certManager.GetCACertificatePath()
	if _, err := os.Stat(caCertPath); err == nil {
		// Add certificate to the profile
		if err := a.addCertificateToPrebuiltDatabase(profileDir, caCertPath); err != nil {
			certError := fmt.Sprintf("❌ CERTIFICATE INSTALLATION ERROR: %v", err)
			fmt.Println(certError) // Console log
			currentDownloadProgress.Status = certError
			currentDownloadProgress.Error = certError
			currentDownloadProgress.Progress = 0.98
			return err
		} else {
			// Certificate installed successfully
			fmt.Println("✅ Successfully installed Gleip CA certificate in Firefox profile")
			currentDownloadProgress.Status = "Firefox installed with certificate"
		}
	} else {
		certError := fmt.Sprintf("❌ CA CERTIFICATE NOT FOUND at %s", caCertPath)
		fmt.Println(certError) // Console log
		currentDownloadProgress.Status = certError
		currentDownloadProgress.Error = certError
		currentDownloadProgress.Progress = 0.98
		return fmt.Errorf("%s", certError)
	}

	// Set final status
	if currentDownloadProgress.Error == "" {
		currentDownloadProgress.Status = "Installation complete"
	} else {
		TrackError("download_failed", currentDownloadProgress.Error)
	}
	currentDownloadProgress.Progress = 1.0

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	// Open source file
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %v", err)
	}
	defer sourceFile.Close()

	// Create destination file
	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %v", err)
	}
	defer destFile.Close()

	// Copy contents
	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy file contents: %v", err)
	}

	// Flush to disk
	if err := destFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync destination file: %v", err)
	}

	return nil
}

// copyDirectory copies a directory from src to dst
func copyDirectory(src, dst string) error {
	// Ensure destination directory exists
	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %v", dst, err)
	}

	// Verify source directory exists
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("source directory %s does not exist: %v", src, err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source %s is not a directory", src)
	}

	fmt.Printf("DEBUG: Copying directory from %s to %s\n", src, dst)

	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("failed to read source directory %s: %v", src, err)
	}

	fmt.Printf("DEBUG: Found %d entries to copy\n", len(entries))

	for i, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		fmt.Printf("DEBUG: [%d/%d] Processing %s -> %s\n", i+1, len(entries), srcPath, dstPath)

		if entry.IsDir() {
			fmt.Printf("DEBUG: Creating directory %s\n", dstPath)
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return fmt.Errorf("failed to create destination directory %s: %v", dstPath, err)
			}
			fmt.Printf("DEBUG: Recursively copying directory %s\n", srcPath)
			if err := copyDirectory(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to copy subdirectory %s: %v", srcPath, err)
			}
		} else {
			fmt.Printf("DEBUG: Copying file %s (size: ", srcPath)
			if info, err := entry.Info(); err == nil {
				fmt.Printf("%d bytes)\n", info.Size())
			} else {
				fmt.Printf("unknown size)\n")
			}

			if err := copyFile(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to copy file %s to %s: %v", srcPath, dstPath, err)
			}

			// Preserve execute permissions for binary files
			if info, err := os.Stat(srcPath); err == nil {
				if err := os.Chmod(dstPath, info.Mode()); err != nil {
					// Don't fail on chmod errors, just warn
					fmt.Printf("Warning: Failed to preserve permissions for %s: %v\n", dstPath, err)
				} else {
					fmt.Printf("DEBUG: Preserved permissions %v for %s\n", info.Mode(), dstPath)
				}
			}
		}
	}

	fmt.Printf("DEBUG: Successfully copied all %d entries from %s to %s\n", len(entries), src, dst)
	return nil
}

// copyPrebuiltCertDatabases copies the embedded certificate database files to the Firefox profile
func (a *App) copyPrebuiltCertDatabases(profileDir string) error {
	// Create the profile directory if it doesn't exist
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		fmt.Printf("ERROR: Failed to create profile directory: %v\n", err)
		return fmt.Errorf("failed to create profile directory: %v", err)
	}

	fmt.Println("INFO: Creating Firefox certificate databases in profile directory")

	// Check directory permissions
	fmt.Printf("INFO: Firefox profile directory: %s\n", profileDir)
	dirInfo, err := os.Stat(profileDir)
	if err != nil {
		fmt.Printf("ERROR: Could not stat profile directory: %v\n", err)
	} else {
		fmt.Printf("INFO: Profile directory permissions: %v\n", dirInfo.Mode())
	}

	// Define certificate files with their byte array data from gleip_certificate.go
	certFiles := map[string][]byte{
		"cert9.db":   cert.Cert9Db,
		"key4.db":    cert.Key4Db,
		"pkcs11.txt": cert.Pkcs11Txt,
	}

	// Write each certificate database file
	var copyErrors []string
	filesCopied := 0

	for filename, data := range certFiles {
		destPath := filepath.Join(profileDir, filename)

		fmt.Printf("INFO: Writing %s (%d bytes)\n", filename, len(data))

		// Write the file
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			errMsg := fmt.Sprintf("Failed to write certificate file %s: %v", filename, err)
			fmt.Println("ERROR: " + errMsg)
			copyErrors = append(copyErrors, errMsg)
			continue
		}

		// Verify the file was written and is readable
		fileInfo, err := os.Stat(destPath)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to verify %s was written: %v", filename, err)
			fmt.Println("ERROR: " + errMsg)
			copyErrors = append(copyErrors, errMsg)
			continue
		}

		// Verify file size
		if fileInfo.Size() == 0 {
			errMsg := fmt.Sprintf("File %s was created but is empty", filename)
			fmt.Println("ERROR: " + errMsg)
			copyErrors = append(copyErrors, errMsg)
			continue
		}

		filesCopied++
		fmt.Printf("SUCCESS: Created %s in profile (size: %d bytes)\n", filename, fileInfo.Size())
	}

	// Copy the CA certificate to the profile
	caPath := a.proxyServer.certManager.GetCACertificatePath()
	if _, err := os.Stat(caPath); err == nil {
		destCertPath := filepath.Join(profileDir, "gleip_ca.crt")
		if err := copyFile(caPath, destCertPath); err != nil {
			fmt.Printf("WARNING: Failed to copy CA certificate to profile: %v\n", err)
		} else {
			fmt.Printf("SUCCESS: Copied CA certificate to profile\n")
		}
	}

	// Additional verification step
	fmt.Println("DEBUG: Verifying all required files")
	requiredFiles := []string{"cert9.db", "key4.db", "pkcs11.txt"}
	allFilesExist := true

	for _, filename := range requiredFiles {
		filePath := filepath.Join(profileDir, filename)
		if _, err := os.Stat(filePath); err != nil {
			fmt.Printf("ERROR: Required file %s missing or inaccessible: %v\n", filename, err)
			allFilesExist = false
		} else {
			fmt.Printf("INFO: Verified %s exists\n", filename)
		}
	}

	// Force filesystem sync
	fmt.Println("INFO: Forcing filesystem sync")
	syncCmd := exec.Command("sync")
	if err := syncCmd.Run(); err != nil {
		fmt.Printf("WARNING: Failed to run sync command: %v\n", err)
	}

	// List all files in the directory
	fmt.Println("INFO: Listing all files in profile directory:")
	files, err := os.ReadDir(profileDir)
	if err != nil {
		fmt.Printf("ERROR: Failed to read profile directory: %v\n", err)
	} else {
		for _, file := range files {
			info, _ := file.Info()
			if info != nil {
				fmt.Printf("  - %s (size: %d bytes, mode: %v)\n", file.Name(), info.Size(), info.Mode())
			} else {
				fmt.Printf("  - %s\n", file.Name())
			}
		}
	}

	if !allFilesExist {
		return fmt.Errorf("not all required certificate database files were created successfully")
	}

	// Show summary
	fmt.Printf("INFO: Certificate database creation summary: %d/%d files created successfully\n", filesCopied, len(certFiles))

	// If any files failed, return error
	if len(copyErrors) > 0 {
		errMsg := fmt.Sprintf("Failed to create some certificate files: %v", copyErrors)
		fmt.Println("ERROR: " + errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}

// addCertificateToPrebuiltDatabase is no longer needed because we're using the embedded
// certificate databases from build.sh that already have the certificate installed.
// This function now just verifies the certificate is in the database.
func (a *App) addCertificateToPrebuiltDatabase(profileDir string, caPath string) error {
	// Ensure gleip_ca.crt exists in the profile directory for Firefox to reference
	if _, err := os.Stat(caPath); err == nil {
		destCertPath := filepath.Join(profileDir, "gleip_ca.crt")
		if err := copyFile(caPath, destCertPath); err != nil {
			fmt.Printf("WARNING: Failed to copy CA certificate to profile: %v\n", err)
		} else {
			fmt.Printf("SUCCESS: Copied CA certificate to profile\n")
		}
	}

	return nil
}

// LaunchFirefoxBrowser launches Firefox with the Gleip proxy configuration
func (a *App) LaunchFirefoxBrowser() error {
	// First check if Firefox is already running
	if a.IsFirefoxRunning() {
		// If Firefox is already running, focus it instead of launching a new instance
		return a.FocusFirefox()
	}

	TrackFirefoxAction("launched", true)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %v", err)
	}

	// Determine platform-specific directory
	var browserDir string
	switch stdruntime.GOOS {
	case "darwin":
		browserDir = filepath.Join(homeDir, "Library", "Application Support", "Gleip", "browsers", "firefox")
	case "linux":
		browserDir = filepath.Join(homeDir, ".config", "Gleip", "browsers", "firefox")
	case "windows":
		browserDir = filepath.Join(homeDir, "AppData", "Local", "Gleip", "browsers", "firefox")
	default:
		return fmt.Errorf("unsupported platform: %s", stdruntime.GOOS)
	}

	// Create profile directory if it doesn't exist
	profileDir := filepath.Join(browserDir, "profile")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return fmt.Errorf("failed to create profile directory: %v", err)
	}

	// Create prefs.js if it doesn't exist
	prefsJSPath := filepath.Join(profileDir, "prefs.js")
	if _, err := os.Stat(prefsJSPath); os.IsNotExist(err) {
		prefsJSContent := `// Mozilla User Preferences
// This file is managed by Gleip

user_pref("app.update.enabled", false);
user_pref("browser.shell.checkDefaultBrowser", false);
user_pref("browser.startup.homepage_override.mstone", "ignore");
user_pref("browser.startup.page", 0);
`
		if err := os.WriteFile(prefsJSPath, []byte(prefsJSContent), 0644); err != nil {
			return fmt.Errorf("failed to create prefs.js file: %v", err)
		}
	}

	// Get Firefox path based on platform
	var firefoxPath string
	var args []string

	// Use absolute path for profile directory to avoid any confusion
	absProfileDir, err := filepath.Abs(profileDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for profile directory: %v", err)
	}

	switch stdruntime.GOOS {
	case "darwin":
		firefoxPath = filepath.Join(browserDir, "Firefox.app", "Contents", "MacOS", "firefox")
		// On macOS, use -P to explicitly specify the profile name
		args = []string{"-profile", absProfileDir, "-no-remote"}
	case "linux":
		firefoxPath = filepath.Join(browserDir, "firefox")
		args = []string{"-profile", absProfileDir, "-no-remote"}
	case "windows":
		firefoxPath = filepath.Join(browserDir, "firefox.exe")
		args = []string{"-profile", absProfileDir, "-no-remote"}
	default:
		return fmt.Errorf("unsupported platform: %s", stdruntime.GOOS)
	}

	// Check if Firefox exists
	if _, err := os.Stat(firefoxPath); os.IsNotExist(err) {
		return fmt.Errorf("firefox not found at %s, please install Firefox first", firefoxPath)
	}

	// Launch Firefox with the specified profile
	cmd := exec.Command(firefoxPath, args...)

	// For debugging, log the command and arguments
	fmt.Printf("Launching Firefox with command: %s %v\n", firefoxPath, args)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch Firefox: %v", err)
	}

	// Store the PID
	a.firefoxPID = cmd.Process.Pid

	// Log the PID for debugging
	fmt.Printf("Firefox launched with PID: %d\n", a.firefoxPID)

	return nil
}

// IsFirefoxRunning checks if the Firefox instance launched by Gleip is still running
func (a *App) IsFirefoxRunning() bool {
	if a.firefoxPID <= 0 {
		return false
	}

	// Different process check methods based on OS
	var process *os.Process
	var err error

	// Try to find the process
	process, err = os.FindProcess(a.firefoxPID)
	if err != nil {
		// On Unix systems, FindProcess never returns an error
		// On Windows, if the process doesn't exist, it returns an error
		a.firefoxPID = 0
		return false
	}

	// On Unix-like systems, we need to send signal 0 to check if the process exists
	if stdruntime.GOOS != "windows" {
		err = process.Signal(syscall.Signal(0))
		if err != nil {
			// Process doesn't exist or we don't have permission
			a.firefoxPID = 0
			return false
		}

		// Additional check for macOS to verify it's actually Firefox and not another process with the same PID
		if stdruntime.GOOS == "darwin" {
			// Use ps to verify if the process is Firefox
			cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", a.firefoxPID), "-o", "comm=")
			output, err := cmd.Output()
			if err != nil || !strings.Contains(strings.ToLower(string(output)), "firefox") {
				// Process is not Firefox or error checking
				a.firefoxPID = 0
				return false
			}
		}
	} else {
		// For Windows, check if process is still running
		// We need more complex logic since FindProcess on Windows doesn't error for non-existent processes
		cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", a.firefoxPID), "/FO", "CSV")
		output, err := cmd.Output()
		if err != nil || !strings.Contains(strings.ToLower(string(output)), "firefox") {
			a.firefoxPID = 0
			return false
		}
	}

	return true
}

// FocusFirefox brings the Firefox window to the foreground
func (a *App) FocusFirefox() error {
	if !a.IsFirefoxRunning() {
		return fmt.Errorf("firefox is not running")
	}

	TrackFirefoxAction("focused", true)

	switch stdruntime.GOOS {
	case "darwin":
		// macOS: Target the specific Firefox process by PID
		// Get the bundle ID first
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get user home directory: %v", err)
		}

		// First try AppleScript specifically targeting our version
		firefoxPath := filepath.Join(homeDir, ".gleip", "browsers", "firefox", "Firefox.app")
		cmd := exec.Command("osascript", "-e", fmt.Sprintf(`tell application "%s" to activate`, firefoxPath))
		if err := cmd.Run(); err != nil {
			// Fallback to activating by PID
			cmd := exec.Command("osascript", "-e", fmt.Sprintf(`
				tell application "System Events"
					set frontmost of the first process whose unix id is %d to true
				end tell
			`, a.firefoxPID))
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to focus Gleip Firefox: %v", err)
			}
		}
	case "windows":
		// Windows: Target the specific Firefox window by PID
		script := fmt.Sprintf(`
		$process = Get-Process -Id %d -ErrorAction SilentlyContinue
		if ($process -ne $null) {
			$hwnd = $process.MainWindowHandle
			if ($hwnd -ne 0) {
				[void][System.Reflection.Assembly]::LoadWithPartialName("System.Windows.Forms")
				[System.Windows.Forms.SetForegroundWindow]::SetForegroundWindow($hwnd)
			}
		}
		`, a.firefoxPID)
		cmd := exec.Command("powershell", "-Command", script)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to focus Gleip Firefox: %v", err)
		}
	case "linux":
		// Linux: Try focusing by window title (Gleip Firefox should have a specific profile name)
		// First try by PID
		cmd := exec.Command("xdotool", "search", "--pid", fmt.Sprintf("%d", a.firefoxPID), "windowactivate")
		if err := cmd.Run(); err != nil {
			// Fallback to wmctrl - but this might not be specific enough
			cmd := exec.Command("wmctrl", "-a", "Firefox")
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to focus Gleip Firefox: %v", err)
			}
		}
	default:
		return fmt.Errorf("focusing window not supported on %s", stdruntime.GOOS)
	}

	return nil
}

// InstallCertificateInFirefox installs the Gleip CA certificate into Firefox's certificate database
func (a *App) InstallCertificateInFirefox() (string, error) {
	// Get path to Gleip Firefox profile
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "❌ Error: Could not locate user home directory", fmt.Errorf("failed to get user home directory: %v", err)
	}

	TrackFirefoxAction("certificate_installation_started", true)

	// Get CA certificate path
	caPath := a.proxyServer.certManager.GetCACertificatePath()
	if _, err := os.Stat(caPath); os.IsNotExist(err) {
		return "❌ Error: CA certificate not found", fmt.Errorf("CA certificate not found at %s", caPath)
	}

	// Determine platform-specific directory
	var browserDir string
	switch stdruntime.GOOS {
	case "darwin":
		browserDir = filepath.Join(homeDir, "Library", "Application Support", "Gleip", "browsers", "firefox")
	case "linux":
		browserDir = filepath.Join(homeDir, ".config", "Gleip", "browsers", "firefox")
	case "windows":
		browserDir = filepath.Join(homeDir, "AppData", "Local", "Gleip", "browsers", "firefox")
	default:
		return "❌ Error: Unsupported platform", fmt.Errorf("unsupported platform: %s", stdruntime.GOOS)
	}

	gleipProfilePath := filepath.Join(browserDir, "profile")

	// Check if profile directory exists
	_, err = os.Stat(gleipProfilePath)
	if os.IsNotExist(err) {
		return "❌ Error: Firefox profile not found", fmt.Errorf("firefox profile not found at %s", gleipProfilePath)
	}

	// Copy certificate database files first
	if err := a.copyPrebuiltCertDatabases(gleipProfilePath); err != nil {
		return fmt.Sprintf("❌ Error: Failed to copy certificate databases: %v", err), err
	}

	// Try to add the certificate to the database
	err = a.addCertificateToPrebuiltDatabase(gleipProfilePath, caPath)
	if err != nil {
		TrackError("certificate_installation_failed", err.Error())
		return fmt.Sprintf("❌ Error: Failed to install certificate: %v", err), err
	}

	TrackFirefoxAction("certificate_installation_succeeded", true)
	return "✅ Successfully installed Gleip CA certificate in the custom Firefox profile", nil
}

// UninstallFirefox uninstalls the Gleip Firefox browser and its profile
func (a *App) UninstallFirefox() error {
	// Check if Firefox is running and terminate it
	if a.IsFirefoxRunning() {
		// Get the process
		process, err := os.FindProcess(a.firefoxPID)
		if err == nil {
			// Kill the process
			if err := process.Kill(); err != nil {
				fmt.Printf("Warning: Failed to terminate Firefox process: %v\n", err)
			} else {
				// Reset the PID
				a.firefoxPID = 0
			}
		}
	}

	TrackFirefoxAction("uninstall_started", true)

	// Get user home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %v", err)
	}

	// Determine platform-specific directory
	var browserDir string
	switch stdruntime.GOOS {
	case "darwin":
		browserDir = filepath.Join(homeDir, "Library", "Application Support", "Gleip", "browsers", "firefox")
	case "linux":
		browserDir = filepath.Join(homeDir, ".config", "Gleip", "browsers", "firefox")
	case "windows":
		browserDir = filepath.Join(homeDir, "AppData", "Local", "Gleip", "browsers", "firefox")
	default:
		return fmt.Errorf("unsupported platform: %s", stdruntime.GOOS)
	}

	// Check if the directories exist
	if _, err := os.Stat(browserDir); os.IsNotExist(err) {
		// Firefox is not installed
		return nil
	}

	// First try to remove the profile directory (less critical)
	profileDir := filepath.Join(browserDir, "profile")
	if _, err := os.Stat(profileDir); err == nil {
		if err := os.RemoveAll(profileDir); err != nil {
			// Log the error but continue with uninstallation
			fmt.Printf("Warning: Failed to remove Firefox profile: %v\n", err)
		}
	}

	// Then remove the entire Firefox directory
	if err := os.RemoveAll(browserDir); err != nil {
		TrackError("uninstall_failed", err.Error())
		return fmt.Errorf("failed to remove Firefox installation: %v", err)
	}

	TrackFirefoxAction("uninstall_succeeded", true)
	return nil
}
