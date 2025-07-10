package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	buildDir      = "./build"
	tempCertdbDir = "./build/temp_certdb"
	caDir         = "./assets/ca"
	goCertFile    = "./backend/cert/gleip_certificate.go"
)

func main() {
	if len(os.Args) < 2 {
		if filesExist(filepath.Join(caDir, "gleip.cer"), filepath.Join(caDir, "gleip.key")) {
			// Early check for required tools only when building
			checkAndInstallDependencies()
			runCommand("build")
		} else {
			showHelp()
		}
		return
	}

	command := os.Args[1]

	switch command {
	case "help", "--help":
		showHelp()
	case "clean":
		runClean()
	case "deps":
		installDependencies()
	case "certs":
		// Check dependencies needed for certificate generation
		checkAndInstallDependencies()
		runCommand("certs")
	case "save-certs":
		// Generate and save certificate databases for reuse
		checkAndInstallDependencies()
		runCommand("certs")
		saveCertificateDatabases()
	case "dev":
		// Check dependencies needed for development
		checkAndInstallDependencies()
		runCommand("dev")
	case "build":
		// Check dependencies needed for building
		checkAndInstallDependencies()
		runCommand("build")
	case "sign-dmg":
		// Sign a CI-built DMG with local certificate
		if len(os.Args) < 3 {
			fmt.Println("Usage: go run cmd/build-tool/main.go sign-dmg <path-to-dmg>")
			os.Exit(1)
		}
		signCIDmg(os.Args[2])
	case "package-dmg":
		// Package existing .app as DMG
		checkAndInstallDependencies()
		packageMacOSWithDMG(extractVersion())
	case "publish":
		// Publish release from CI artifacts
		publishRelease()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		showHelp()
	}
}

func checkAndInstallDependencies() {
	// Check if wails is available
	if !commandExists("wails") {
		fmt.Println("⚠️  Wails not found in PATH. Attempting to install...")

		// Try to install wails
		cmd := exec.Command("go", "install", "github.com/wailsapp/wails/v2/cmd/wails@latest")
		if err := cmd.Run(); err != nil {
			fmt.Printf("❌ Failed to install Wails: %v\n", err)
			fmt.Println("Please install Wails manually: go install github.com/wailsapp/wails/v2/cmd/wails@latest")
			os.Exit(1)
		}

		// Update PATH to include GOPATH/bin
		updateGoPath()

		// Check again
		if !commandExists("wails") {
			fmt.Println("❌ Wails still not found after installation.")
			fmt.Printf("Please add %s to your PATH\n", getGoBinPath())
			os.Exit(1)
		}

		fmt.Println("✅ Wails installed successfully")
	}
}

func updateGoPath() {
	goBinPath := getGoBinPath()
	currentPath := os.Getenv("PATH")

	// Check if go bin path is already in PATH
	if !strings.Contains(currentPath, goBinPath) {
		newPath := currentPath + string(os.PathListSeparator) + goBinPath
		os.Setenv("PATH", newPath)
		fmt.Printf("✅ Added %s to PATH\n", goBinPath)
	}
}

func getGoBinPath() string {
	// Get GOPATH
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		// Fallback to go env GOPATH
		cmd := exec.Command("go", "env", "GOPATH")
		output, err := cmd.Output()
		if err != nil {
			return filepath.Join(os.Getenv("HOME"), "go", "bin")
		}
		gopath = strings.TrimSpace(string(output))
	}

	// Handle tilde expansion
	if strings.HasPrefix(gopath, "~/") {
		homeDir, _ := os.UserHomeDir()
		gopath = filepath.Join(homeDir, gopath[2:])
	}

	return filepath.Join(gopath, "bin")
}

func showHelp() {
	fmt.Println("Usage: go run cmd/build-tool/main.go [command]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  build       Build the Gleip application for current platform (default)")
	fmt.Println("  dev         Run in development mode")
	fmt.Println("  certs       Generate certificate files only")
	fmt.Println("  save-certs  Generate and save certificate databases for CI reuse")
	fmt.Println("  package-dmg Package existing .app as signed DMG (macOS only)")
	fmt.Println("  sign-dmg    Sign a CI-built DMG with local certificate")
	fmt.Println("  publish     Download CI artifacts, sign DMGs, and publish release")
	fmt.Println("  clean       Remove build artifacts")
	fmt.Println("  deps        Install platform dependencies")
	fmt.Println("  help        Show this help message")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  go run cmd/build-tool/main.go build")
	fmt.Println("  go run cmd/build-tool/main.go save-certs  # Run on Linux/Mac, then commit")
	fmt.Println("  go run cmd/build-tool/main.go package-dmg  # Package .app as DMG")
	fmt.Println("  go run cmd/build-tool/main.go sign-dmg ./downloaded-build.dmg")
	fmt.Println("  go run cmd/build-tool/main.go publish    # Download, sign, and release")
	fmt.Println("  go run cmd/build-tool/main.go dev")
}

func runClean() {
	fmt.Println("=== Gleip Clean Process ===")
	fmt.Println("Removing build artifacts...")

	dirsToRemove := []string{
		"./frontend/node_modules",
		"./frontend/dist",
		"./.wails",
		tempCertdbDir,
		"./build/bin",
	}

	for _, dir := range dirsToRemove {
		if _, err := os.Stat(dir); err == nil {
			fmt.Printf("Removing %s...\n", dir)
			os.RemoveAll(dir)
		}
	}

	if _, err := os.Stat(goCertFile); err == nil {
		fmt.Printf("Removing %s...\n", goCertFile)
		os.Remove(goCertFile)
	}

	fmt.Println("✅ Build artifacts removed.")
}

func installDependencies() {
	fmt.Println("=== Installing Platform Dependencies ===")

	switch runtime.GOOS {
	case "windows":
		fmt.Println("Windows detected. Please ensure you have:")
		fmt.Println("  - Go installed")
		fmt.Println("  - Node.js installed")
		fmt.Println("  - Wails: go install github.com/wailsapp/wails/v2/cmd/wails@latest")
		fmt.Println("  - NSS tools for Windows")
		fmt.Println("  - Visual Studio Build Tools")

	case "darwin":
		fmt.Println("macOS detected. Installing dependencies...")

		if !commandExists("brew") {
			fmt.Println("Error: Homebrew not found. Please install Homebrew first:")
			fmt.Println(`/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`)
			os.Exit(1)
		}

		runCmd("brew", "install", "nss", "node", "go")

	case "linux":
		fmt.Println("Linux detected. Installing dependencies...")

		if fileExists("/etc/debian_version") {
			fmt.Println("Debian/Ubuntu detected...")
			runCmd("sudo", "apt", "update")
			runCmd("sudo", "apt", "install", "-y", "libwebkit2gtk-4.1-dev", "libgtk-3-dev", "pkg-config", "libnss3-tools", "nodejs", "npm", "golang-go")

			// Create webkit compatibility symlink
			linkPath := "/usr/lib/x86_64-linux-gnu/pkgconfig/webkit2gtk-4.0.pc"
			targetPath := "/usr/lib/x86_64-linux-gnu/pkgconfig/webkit2gtk-4.1.pc"
			if !fileExists(linkPath) && fileExists(targetPath) {
				fmt.Println("Creating WebKit compatibility symlink...")
				runCmd("sudo", "ln", "-sf", targetPath, linkPath)
			}
		} else if fileExists("/etc/redhat-release") {
			fmt.Println("Red Hat family detected...")
			runCmd("sudo", "dnf", "install", "-y", "webkit2gtk4.1-devel", "gtk3-devel", "pkg-config", "nss-tools", "nodejs", "npm", "golang")
		} else {
			fmt.Println("Unknown Linux distribution. Please install manually:")
			fmt.Println("  - webkit2gtk development libraries")
			fmt.Println("  - gtk3 development libraries")
			fmt.Println("  - pkg-config, NSS tools, Node.js, Go")
		}
	}

	if !commandExists("wails") {
		fmt.Println("Installing Wails...")
		runCmd("go", "install", "github.com/wailsapp/wails/v2/cmd/wails@latest")
		updateGoPath()
	}

	fmt.Println("✅ Dependencies installation complete")
}

func runCommand(command string) {
	fmt.Printf("=== Gleip Process (Mode: %s) ===\n", command)
	fmt.Printf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)

	// Create directories
	os.MkdirAll(tempCertdbDir, 0755)
	os.MkdirAll(caDir, 0755)

	// Check CA files
	caCertPath := filepath.Join(caDir, "gleip.cer")
	caKeyPath := filepath.Join(caDir, "gleip.key")

	if !filesExist(caCertPath, caKeyPath) {
		fmt.Printf("Error: CA certificate (%s) or key (%s) not found.\n", caCertPath, caKeyPath)
		fmt.Println("Please generate these files first. See README.md for instructions.")
		os.Exit(1)
	}

	fmt.Println("✅ Root CA certificate and key found")

	// Generate certificate databases
	generateCertificateDatabases(caCertPath)

	// Generate Go file
	generateGoFile(caCertPath, caKeyPath)

	if command == "certs" {
		fmt.Println("=== Certificate Generation Complete ===")
		return
	}

	// Install dependencies
	fmt.Println("Installing JavaScript dependencies...")
	runCmdInDir("frontend", "npm", "install")

	fmt.Println("Installing Go dependencies...")
	runCmd("go", "mod", "tidy")

	// Extract version for ldflags
	version := extractVersion()

	// Get PostHog configuration from environment variables
	posthogAPIKey := os.Getenv("POSTHOG_API_KEY")
	posthogEndpoint := os.Getenv("POSTHOG_ENDPOINT")

	// If no API key is provided, telemetry will be disabled at runtime
	if posthogAPIKey == "" || posthogEndpoint == "" {
		fmt.Println("⚠️  No POSTHOG_API_KEY or POSTHOG_ENDPOINT provided - telemetry will be disabled")
		posthogAPIKey = "disabled" // Placeholder to indicate disabled state
	}

	// Build ldflags with version and PostHog configuration
	ldflags := fmt.Sprintf(`-X "Gleip/backend.AppVersion=%s" -X "Gleip/backend.PostHogAPIKey=%s" -X "Gleip/backend.PostHogEndpoint=%s"`,
		version, posthogAPIKey, posthogEndpoint)

	// Build or dev
	switch command {
	case "build":
		fmt.Printf("Building application for current platform (version: %s)...\n", version)

		// Build command arguments
		buildArgs := []string{"build", "-clean", "-ldflags", ldflags}

		// Add webkit2_41 build tag for Linux to fix webkit2gtk-4.0 compatibility
		if runtime.GOOS == "linux" {
			buildArgs = append(buildArgs, "-tags", "webkit2_41")
		}

		runCmd("wails", buildArgs...)

		// Package application
		packageApplication()

	case "dev":
		fmt.Println("Starting development server...")

		// Set development mode environment variable
		os.Setenv("GLEIP_DEV_MODE", "true")

		// Dev command arguments
		devArgs := []string{"dev", "-ldflags", ldflags}

		// Add webkit2_41 build tag for Linux to fix webkit2gtk-4.0 compatibility
		if runtime.GOOS == "linux" {
			devArgs = append(devArgs, "-tags", "webkit2_41")
		}

		runCmd("wails", devArgs...)
	}

	fmt.Printf("=== Process Complete (Mode: %s) ===\n", command)
}

func packageApplication() {
	switch runtime.GOOS {
	case "darwin":
		packageMacOS()
	case "windows":
		packageWindows()
	case "linux":
		packageLinux()
	}
}

func packageMacOS() {
	appPath := "./build/bin/Gleip.app"
	if !fileExists(appPath) {
		return
	}

	fmt.Printf("✅ Application created: %s\n", appPath)
}

func packageMacOSWithDMG(version string) {
	appPath := "./build/bin/Gleip.app"
	if !fileExists(appPath) {
		return
	}

	signApp(appPath)
	createDMG(appPath, version)
}

func packageWindows() {
	exePath := "./build/bin/Gleip.exe"
	if fileExists(exePath) {
		fmt.Printf("✅ Executable created: %s\n", exePath)
	}
}

func packageLinux() {
	binPath := "./build/bin/Gleip"
	if fileExists(binPath) {
		fmt.Printf("✅ Executable created: %s\n", binPath)
	}
}

func signApp(appPath string) {
	if !commandExists("codesign") {
		return
	}

	if os.Getenv("GLEIP_CI_BUILD") != "" && os.Getenv("ENABLE_CODE_SIGNING") != "true" {
		fmt.Println("Skipping code signing in CI environment...")
		return
	}

	if os.Getenv("ENABLE_CODE_SIGNING") == "true" {
		// Release builds with proper certificates
		fmt.Println("Signing application for release...")
		runCmd("codesign", "--force", "--deep", "--sign", "Gleip.io", appPath)
	} else {
		// Local development builds
		fmt.Println("Signing application with development certificate...")
		runCmd("codesign", "--force", "--deep", "--sign", "Gleip.io", appPath)
	}

	// Verify the signature
	fmt.Println("Verifying code signature...")
	runCmd("codesign", "--verify", "--verbose", appPath)
}

func createDMG(appPath, version string) {
	// Extract architecture from the app path or parent directory
	arch := runtime.GOARCH // fallback
	arch = "arm64"

	dmgName := fmt.Sprintf("gleip-%s-macos-%s.dmg", version, arch)
	dmgPath := filepath.Join(filepath.Dir(appPath), dmgName)
	tempDir := filepath.Join(filepath.Dir(appPath), "dmg_temp")

	os.MkdirAll(tempDir, 0755)
	runCmd("cp", "-R", appPath, tempDir+"/")

	if commandExists("create-dmg") {
		runCmd("create-dmg", "--volname", "Gleip", "--window-size", "800", "400", "--app-drop-link", "600", "185", dmgPath, tempDir)
	} else if commandExists("hdiutil") {
		// Create Applications symlink for hdiutil
		runCmd("ln", "-s", "/Applications", tempDir+"/Applications")
		runCmd("hdiutil", "create", "-volname", "Gleip", "-srcfolder", tempDir, "-ov", "-format", "UDZO", dmgPath)
	}

	os.RemoveAll(tempDir)
	fmt.Printf("✅ Distribution DMG created: %s\n", dmgPath)
}

func generateCertificateDatabases(caCertPath string) {
	fmt.Println("Generating certificate databases...")

	// Check if pre-generated certificate databases exist
	cert9Path := filepath.Join(tempCertdbDir, "cert9.db")
	key4Path := filepath.Join(tempCertdbDir, "key4.db")
	pkcs11Path := filepath.Join(tempCertdbDir, "pkcs11.txt")

	// If pre-generated databases exist, use them instead of generating new ones
	if filesExist(cert9Path, key4Path, pkcs11Path) {
		fmt.Println("✅ Found existing certificate databases, using them instead of generating new ones")
		return
	}

	// Check if we have pre-generated databases in assets
	assetsDbDir := "./assets/certdb"
	assetsCert9 := filepath.Join(assetsDbDir, "cert9.db")
	assetsKey4 := filepath.Join(assetsDbDir, "key4.db")
	assetsPkcs11 := filepath.Join(assetsDbDir, "pkcs11.txt")

	if filesExist(assetsCert9, assetsKey4, assetsPkcs11) {
		fmt.Println("✅ Found pre-generated certificate databases in assets, copying them...")
		os.MkdirAll(tempCertdbDir, 0755)

		// Copy pre-generated files
		copyFile(assetsCert9, cert9Path)
		copyFile(assetsKey4, key4Path)
		copyFile(assetsPkcs11, pkcs11Path)

		fmt.Println("✅ Certificate databases copied successfully")
		return
	}

	// Fall back to generating databases with certutil (Linux/Mac)
	if !commandExists("certutil") {
		if os.Getenv("GLEIP_CI_BUILD") != "" {
			fmt.Println("Error: certutil not found in CI environment and no pre-generated databases available.")
			fmt.Println("To fix this issue:")
			fmt.Println("1. Run this build on Linux/Mac to generate certificate databases")
			fmt.Println("2. Commit the generated databases to ./assets/certdb/")
			fmt.Println("3. Or use a Linux CI job to generate and upload the databases as artifacts")
		} else {
			fmt.Println("Error: certutil not found. Run 'go run cmd/build-tool/main.go deps' to install dependencies.")
		}
		os.Exit(1)
	}

	fmt.Println("Generating new certificate databases with certutil...")

	// Remove existing files
	os.Remove(cert9Path)
	os.Remove(key4Path)
	os.Remove(pkcs11Path)

	// Create database
	runCmd("certutil", "-N", "--empty-password", "-d", "sql:"+tempCertdbDir)

	// Create pkcs11.txt if missing
	if !fileExists(pkcs11Path) {
		content := "library=\nparameters=\n"
		os.WriteFile(pkcs11Path, []byte(content), 0644)
	}

	// Add CA certificate
	runCmd("certutil", "-A", "-n", "Gleip_Root_CA", "-t", "C,,", "-i", caCertPath, "-d", "sql:"+tempCertdbDir)

	// Verify
	output := runCmdOutput("certutil", "-L", "-d", "sql:"+tempCertdbDir)
	if !strings.Contains(output, "Gleip_Root_CA") {
		fmt.Println("❌ Failed to add CA certificate to database")
		os.Exit(1)
	}

	fmt.Println("✅ Certificate databases created successfully")

	// Suggest saving the databases for future use
	if os.Getenv("GLEIP_CI_BUILD") == "" {
		fmt.Println("💡 Tip: Consider copying these databases to ./assets/certdb/ for reuse:")
		fmt.Printf("  mkdir -p %s\n", assetsDbDir)
		fmt.Printf("  cp %s %s\n", cert9Path, assetsCert9)
		fmt.Printf("  cp %s %s\n", key4Path, assetsKey4)
		fmt.Printf("  cp %s %s\n", pkcs11Path, assetsPkcs11)
	}
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

func saveCertificateDatabases() {
	fmt.Println("=== Saving Certificate Databases for Reuse ===")

	assetsDbDir := "./assets/certdb"
	tempCert9 := filepath.Join(tempCertdbDir, "cert9.db")
	tempKey4 := filepath.Join(tempCertdbDir, "key4.db")
	tempPkcs11 := filepath.Join(tempCertdbDir, "pkcs11.txt")

	assetsCert9 := filepath.Join(assetsDbDir, "cert9.db")
	assetsKey4 := filepath.Join(assetsDbDir, "key4.db")
	assetsPkcs11 := filepath.Join(assetsDbDir, "pkcs11.txt")

	// Check if source files exist
	if !filesExist(tempCert9, tempKey4, tempPkcs11) {
		fmt.Println("❌ Certificate databases not found. Run 'certs' command first.")
		os.Exit(1)
	}

	// Create assets directory
	if err := os.MkdirAll(assetsDbDir, 0755); err != nil {
		fmt.Printf("❌ Failed to create assets directory: %v\n", err)
		os.Exit(1)
	}

	// Copy certificate databases
	if err := copyFile(tempCert9, assetsCert9); err != nil {
		fmt.Printf("❌ Failed to copy cert9.db: %v\n", err)
		os.Exit(1)
	}

	if err := copyFile(tempKey4, assetsKey4); err != nil {
		fmt.Printf("❌ Failed to copy key4.db: %v\n", err)
		os.Exit(1)
	}

	if err := copyFile(tempPkcs11, assetsPkcs11); err != nil {
		fmt.Printf("❌ Failed to copy pkcs11.txt: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Certificate databases saved to %s\n", assetsDbDir)
	fmt.Println("💡 You can now commit these files to your repository:")
	fmt.Printf("  git add %s\n", assetsDbDir)
	fmt.Println("  git commit -m \"Add pre-generated certificate databases for CI\"")
	fmt.Println("")
	fmt.Println("This will allow Windows CI builds to work without requiring NSS tools.")
}

func generateGoFile(caCertPath, caKeyPath string) {
	fmt.Println("Creating Go file with embedded certificates...")

	// Read files and encode to base64
	cert9Data := readFileToBase64(filepath.Join(tempCertdbDir, "cert9.db"))
	key4Data := readFileToBase64(filepath.Join(tempCertdbDir, "key4.db"))
	pkcs11Data := readFileToBase64(filepath.Join(tempCertdbDir, "pkcs11.txt"))
	caCertData := readFileToBase64(caCertPath)
	caKeyData := readFileToBase64(caKeyPath)

	// Generate Go code
	goCode := fmt.Sprintf(`// Code generated by build tool - DO NOT EDIT.
package cert

import (
	"encoding/base64"
)

var (
	Cert9Db []byte
	Key4Db []byte
	Pkcs11Txt []byte
	EmbeddedCACert []byte
	EmbeddedCAKey []byte
)

func init() {
	var err error
	
	Cert9Db, err = base64.StdEncoding.DecodeString("%s")
	if err != nil {
		panic("Failed to decode cert9.db: " + err.Error())
	}
	
	Key4Db, err = base64.StdEncoding.DecodeString("%s")
	if err != nil {
		panic("Failed to decode key4.db: " + err.Error())
	}
	
	Pkcs11Txt, err = base64.StdEncoding.DecodeString("%s")
	if err != nil {
		panic("Failed to decode pkcs11.txt: " + err.Error())
	}
	
	EmbeddedCACert, err = base64.StdEncoding.DecodeString("%s")
	if err != nil {
		panic("Failed to decode CA certificate: " + err.Error())
	}
	
	EmbeddedCAKey, err = base64.StdEncoding.DecodeString("%s")
	if err != nil {
		panic("Failed to decode CA private key: " + err.Error())
	}
}
`, cert9Data, key4Data, pkcs11Data, caCertData, caKeyData)

	// Ensure directory exists and write file
	os.MkdirAll(filepath.Dir(goCertFile), 0755)
	if err := os.WriteFile(goCertFile, []byte(goCode), 0644); err != nil {
		fmt.Printf("❌ Failed to create Go file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Generated Go file with embedded certificates")
}

func extractVersion() string {
	content, err := os.ReadFile("wails.json")
	if err != nil {
		return "unknown"
	}

	var config map[string]interface{}
	if err := json.Unmarshal(content, &config); err != nil {
		return "unknown"
	}

	if info, ok := config["info"].(map[string]interface{}); ok {
		if version, ok := info["productVersion"].(string); ok {
			return version
		}
	}
	return "unknown"
}

func signCIDmg(dmgPath string) {
	fmt.Println("=== Signing DMG with Local Certificate ===")
	fmt.Printf("Processing DMG: %s\n", dmgPath)

	// Check if DMG exists
	if !fileExists(dmgPath) {
		fmt.Printf("❌ DMG file not found: %s\n", dmgPath)
		os.Exit(1)
	}

	// Check if codesign is available
	if !commandExists("codesign") {
		fmt.Println("❌ codesign command not found. This is required for macOS code signing.")
		os.Exit(1)
	}

	// Create temporary working directory
	tempDir := "./build/sign_temp"
	mountPoint := filepath.Join(tempDir, "mount")
	extractDir := filepath.Join(tempDir, "extract")

	os.RemoveAll(tempDir)
	os.MkdirAll(mountPoint, 0755)
	os.MkdirAll(extractDir, 0755)

	defer os.RemoveAll(tempDir)

	fmt.Println("Mounting DMG...")
	// Mount the DMG
	cmd := exec.Command("hdiutil", "attach", dmgPath, "-mountpoint", mountPoint, "-nobrowse", "-quiet")
	if err := cmd.Run(); err != nil {
		fmt.Printf("❌ Failed to mount DMG: %v\n", err)
		os.Exit(1)
	}

	// Ensure we unmount on exit
	defer func() {
		fmt.Println("Unmounting DMG...")
		exec.Command("hdiutil", "detach", mountPoint, "-quiet").Run()
	}()

	// Find the .app bundle
	appPath := ""
	entries, err := os.ReadDir(mountPoint)
	if err != nil {
		fmt.Printf("❌ Failed to read mounted DMG contents: %v\n", err)
		os.Exit(1)
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".app") {
			appPath = filepath.Join(mountPoint, entry.Name())
			break
		}
	}

	if appPath == "" {
		fmt.Println("❌ No .app bundle found in DMG")
		os.Exit(1)
	}

	fmt.Printf("Found app bundle: %s\n", filepath.Base(appPath))

	// Copy .app to extract directory
	extractedAppPath := filepath.Join(extractDir, filepath.Base(appPath))
	fmt.Println("Extracting app bundle...")
	runCmd("cp", "-R", appPath, extractDir)

	// Unmount before signing (some systems require this)
	fmt.Println("Unmounting DMG...")
	runCmd("hdiutil", "detach", mountPoint, "-quiet")

	// Sign the extracted .app
	fmt.Println("Signing application with Gleip.io certificate...")
	runCmd("codesign", "--force", "--deep", "--sign", "Gleip.io", extractedAppPath)

	// Verify the signature
	fmt.Println("Verifying code signature...")
	runCmd("codesign", "--verify", "--verbose", extractedAppPath)

	// Create signed DMG
	signedDmgName := strings.TrimSuffix(filepath.Base(dmgPath), ".dmg") + "-signed.dmg"
	signedDmgPath := filepath.Join(filepath.Dir(dmgPath), signedDmgName)

	fmt.Printf("Creating signed DMG: %s\n", signedDmgName)

	if commandExists("create-dmg") {
		runCmd("create-dmg", "--volname", "Gleip", "--window-size", "800", "400", signedDmgPath, extractDir)
	} else if commandExists("hdiutil") {
		runCmd("hdiutil", "create", "-volname", "Gleip", "-srcfolder", extractDir, "-ov", "-format", "UDZO", signedDmgPath)
	} else {
		fmt.Println("❌ Neither create-dmg nor hdiutil found for DMG creation")
		os.Exit(1)
	}

	fmt.Printf("✅ Signed DMG created: %s\n", signedDmgPath)
	fmt.Println("✅ DMG signing complete!")
}

func publishRelease() {
	fmt.Println("=== Publishing Release from CI Artifacts ===")

	// Check required tools
	if !commandExists("gh") {
		fmt.Println("❌ GitHub CLI (gh) not found. Please install it first:")
		fmt.Println("  brew install gh")
		fmt.Println("  or visit: https://cli.github.com/")
		os.Exit(1)
	}

	if !commandExists("codesign") {
		fmt.Println("❌ codesign command not found. This is required for macOS code signing.")
		os.Exit(1)
	}

	// Get current repository info
	fmt.Println("🔍 Detecting current repository...")
	cmd := exec.Command("gh", "repo", "view", "--json", "name,owner")
	output, err := cmd.Output()
	if err != nil {
		fmt.Println("❌ Cannot detect current repository.")
		fmt.Println("Please ensure:")
		fmt.Println("  1. You're in a git repository")
		fmt.Println("  2. You have access permissions")
		fmt.Println("  3. You're authenticated with: gh auth login")
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Parse repository info
	var repoInfo struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	}
	if err := json.Unmarshal(output, &repoInfo); err != nil {
		fmt.Printf("❌ Failed to parse repository info: %v\n", err)
		os.Exit(1)
	}

	currentRepo := repoInfo.Owner.Login + "/" + repoInfo.Name
	fmt.Printf("✅ Current repository: %s\n", currentRepo)

	// Get version from backend/settings.go
	version := extractVersion()
	if version == "unknown" {
		fmt.Println("❌ Could not extract version from backend/settings.go")
		os.Exit(1)
	}
	fmt.Printf("📦 Version: %s\n", version)

	// Check if RELEASE.txt exists
	releaseNotesPath := "./RELEASE.txt"
	if !fileExists(releaseNotesPath) {
		fmt.Printf("❌ Release notes file not found: %s\n", releaseNotesPath)
		fmt.Println("Please create RELEASE.txt with your release notes.")
		os.Exit(1)
	}

	// Read release notes
	releaseNotes, err := os.ReadFile(releaseNotesPath)
	if err != nil {
		fmt.Printf("❌ Failed to read release notes: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("📝 Release notes loaded from %s\n", releaseNotesPath)

	// Create temp directory for download
	downloadDir := "./release_temp"
	os.RemoveAll(downloadDir)
	os.MkdirAll(downloadDir, 0755)
	defer os.RemoveAll(downloadDir)

	// Get current commit SHA
	fmt.Println("🔍 Getting current commit SHA...")
	commitCmd := exec.Command("git", "rev-parse", "HEAD")
	commitOutput, err := commitCmd.Output()
	if err != nil {
		fmt.Printf("❌ Failed to get current commit SHA: %v\n", err)
		os.Exit(1)
	}
	commitSHA := strings.TrimSpace(string(commitOutput))
	fmt.Printf("Current commit: %s\n", commitSHA)

	// Find build workflow run for this specific commit
	fmt.Printf("🔍 Finding build workflow for commit %s...\n", commitSHA)
	cmd = exec.Command("gh", "run", "list",
		"--repo", currentRepo,
		"--workflow", "build.yml",
		"--status", "completed",
		"--limit", "10",
		"--json", "databaseId,headSha")
	output, err = cmd.Output()
	if err != nil {
		fmt.Printf("❌ Failed to get workflow runs: %v\n", err)
		os.Exit(1)
	}

	// Parse workflow runs to find the one for current commit
	var workflowRuns []struct {
		DatabaseID int    `json:"databaseId"`
		HeadSHA    string `json:"headSha"`
	}
	if err := json.Unmarshal(output, &workflowRuns); err != nil {
		fmt.Printf("❌ Failed to parse workflow runs: %v\n", err)
		os.Exit(1)
	}

	runID := ""
	for _, run := range workflowRuns {
		if run.HeadSHA == commitSHA {
			runID = fmt.Sprintf("%d", run.DatabaseID)
			break
		}
	}

	if runID == "" {
		fmt.Printf("❌ No successful build workflow found for commit %s\n", commitSHA)
		fmt.Println("Available recent workflows:")
		for i, run := range workflowRuns {
			if i >= 5 {
				break
			}
			fmt.Printf("  - Run %d: commit %s\n", run.DatabaseID, run.HeadSHA)
		}
		os.Exit(1)
	}
	fmt.Printf("✅ Found build workflow run: %s for commit: %s\n", runID, commitSHA)

	// Download all artifacts
	fmt.Println("⬇️  Downloading CI artifacts...")
	cmd = exec.Command("gh", "run", "download", runID, "--repo", currentRepo, "--dir", downloadDir)
	if err := cmd.Run(); err != nil {
		fmt.Printf("❌ Failed to download artifacts: %v\n", err)
		os.Exit(1)
	}

	// Find .tar.gz files and extract .app files to create signed DMGs
	fmt.Println("🔏 Processing macOS .app files...")
	err = filepath.Walk(downloadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(info.Name(), ".tar.gz") && strings.Contains(info.Name(), "macos") {
			fmt.Printf("📱 Extracting and creating signed DMG from: %s\n", info.Name())

			// Extract the tar.gz file
			extractDir := filepath.Join(filepath.Dir(path), "extract_temp")
			os.MkdirAll(extractDir, 0755)

			// Get absolute path for tar.gz file
			absPath, err := filepath.Abs(path)
			if err != nil {
				fmt.Printf("❌ Failed to get absolute path for %s: %v\n", path, err)
				return err
			}

			// Extract tar.gz
			runCmdInDir(extractDir, "tar", "-xzf", absPath)

			// Find the .app in the extracted directory and process it
			filepath.Walk(extractDir, func(appPath string, appInfo os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if strings.HasSuffix(appInfo.Name(), ".app") && appInfo.IsDir() {
					signApp(appPath)
					createDMG(appPath, version)
				}
				return nil
			})

			// Move any created DMGs to downloadDir before cleanup
			filepath.Walk(extractDir, func(dmgPath string, dmgInfo os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if strings.HasSuffix(dmgInfo.Name(), ".dmg") {
					destPath := filepath.Join(downloadDir, dmgInfo.Name())
					fmt.Printf("📦 Moving DMG to: %s\n", dmgInfo.Name())
					if err := copyFile(dmgPath, destPath); err != nil {
						fmt.Printf("⚠️  Failed to move DMG %s: %v\n", dmgInfo.Name(), err)
					}
				}
				return nil
			})

			// Remove the original tar.gz since we now have a DMG
			os.Remove(path)

			// Clean up after processing this tar.gz
			os.RemoveAll(extractDir)
		}
		return nil
	})
	if err != nil {
		fmt.Printf("❌ Error processing .app files: %v\n", err)
		os.Exit(1)
	}

	// Collect all artifacts for release
	fmt.Println("📦 Collecting release artifacts...")
	var artifactPaths []string
	err = filepath.Walk(downloadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Include executables, DMGs, and archives with standard naming
		// Note: Exclude macOS tar.gz files since they've been converted to DMGs
		if strings.HasSuffix(info.Name(), ".exe") ||
			strings.HasSuffix(info.Name(), ".dmg") ||
			(strings.HasSuffix(info.Name(), ".tar.gz") && !strings.Contains(info.Name(), "macos")) ||
			(info.Name() == "Gleip" && !info.IsDir()) ||
			(strings.HasPrefix(info.Name(), "gleip-") && (strings.Contains(info.Name(), "-linux-") || strings.Contains(info.Name(), "-windows-")) && !info.IsDir()) ||
			(strings.HasPrefix(info.Name(), "Gleip-linux-") && !info.IsDir()) ||
			(strings.HasPrefix(info.Name(), "Gleip-windows-") && !info.IsDir()) ||
			(strings.HasPrefix(info.Name(), "Gleip-darwin-") && !info.IsDir()) {
			artifactPaths = append(artifactPaths, path)
			fmt.Printf("  📄 %s\n", info.Name())
		}
		return nil
	})
	if err != nil {
		fmt.Printf("❌ Error collecting artifacts: %v\n", err)
		os.Exit(1)
	}

	if len(artifactPaths) == 0 {
		fmt.Println("❌ No artifacts found to release")
		os.Exit(1)
	}

	// Check if release already exists
	fmt.Printf("🔍 Checking if release %s already exists...\n", version)
	cmd = exec.Command("gh", "release", "view", version, "--repo", currentRepo)
	if err := cmd.Run(); err == nil {
		fmt.Printf("⚠️  Release %s already exists. Deleting it first...\n", version)
		cmd = exec.Command("gh", "release", "delete", version, "--repo", currentRepo, "--yes")
		if err := cmd.Run(); err != nil {
			fmt.Printf("❌ Failed to delete existing release: %v\n", err)
			os.Exit(1)
		}
	}

	// Create release on current repository
	fmt.Printf("🚀 Creating release %s on %s...\n", version, currentRepo)

	// First create the release without assets
	cmd = exec.Command("gh", "release", "create", version,
		"--repo", currentRepo,
		"--title", "Gleip "+version,
		"--notes", string(releaseNotes))

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("❌ Failed to create release: %v\n", err)
		fmt.Println("\nTroubleshooting:")
		fmt.Printf("1. Ensure you have release permissions for %s\n", currentRepo)
		fmt.Println("2. Check you have release permissions: gh auth refresh")
		fmt.Printf("3. Try manually: gh release create %s --repo %s\n", version, currentRepo)
		os.Exit(1)
	}

	// Then upload each artifact separately
	fmt.Println("📤 Uploading artifacts...")
	for _, artifactPath := range artifactPaths {
		fileName := filepath.Base(artifactPath)
		fmt.Printf("  Uploading %s...\n", fileName)

		cmd = exec.Command("gh", "release", "upload", version, artifactPath,
			"--repo", currentRepo)

		if err := cmd.Run(); err != nil {
			fmt.Printf("❌ Failed to upload %s: %v\n", fileName, err)
			// Continue with other files rather than exit
		} else {
			fmt.Printf("  ✅ %s uploaded\n", fileName)
		}
	}

	fmt.Printf("✅ Release v%s published successfully!\n", version)
	fmt.Printf("🌐 View at: https://github.com/%s/releases/tag/v%s\n", currentRepo, version)
}

// Utility functions
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func filesExist(paths ...string) bool {
	for _, path := range paths {
		if !fileExists(path) {
			return false
		}
	}
	return true
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func runCmd(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("Command failed: %s %v\n", name, args)
		os.Exit(1)
	}
}

func runCmdInDir(dir, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("Command failed in %s: %s %v\n", dir, name, args)
		os.Exit(1)
	}
}

func runCmdOutput(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(output)
}

func readFileToBase64(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("Failed to read file %s: %v\n", path, err)
		os.Exit(1)
	}
	return base64.StdEncoding.EncodeToString(data)
}
