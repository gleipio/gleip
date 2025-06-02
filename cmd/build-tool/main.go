package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const (
	buildDir      = "./build"
	tempCertdbDir = "./build/temp_certdb"
	caDir         = "./assets/ca"
	goCertFile    = "./backend/cert/gleip_certificate.go"
)

type BuildTarget struct {
	OS   string
	Arch string
	Ext  string
}

var supportedTargets = []BuildTarget{
	{"windows", "amd64", ".exe"},
	{"windows", "arm64", ".exe"},
	{"darwin", "amd64", ""},
	{"darwin", "arm64", ""},
	{"linux", "amd64", ""},
	{"linux", "arm64", ""},
}

func main() {
	if len(os.Args) < 2 {
		if filesExist(filepath.Join(caDir, "gleip.cer"), filepath.Join(caDir, "gleip.key")) {
			// Early check for required tools only when building
			checkAndInstallDependencies()
			runCommand("build", "")
		} else {
			showHelp()
		}
		return
	}

	command := os.Args[1]
	target := ""
	if len(os.Args) > 2 {
		target = os.Args[2]
	}

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
		runCommand("certs", "")
	case "save-certs":
		// Generate and save certificate databases for reuse
		checkAndInstallDependencies()
		runCommand("certs", "")
		saveCertificateDatabases()
	case "dev":
		// Check dependencies needed for development
		checkAndInstallDependencies()
		runCommand("dev", "")
	case "build":
		// Check dependencies needed for building
		checkAndInstallDependencies()
		runCommand("build", target)
	case "build-all":
		// Check dependencies needed for building
		checkAndInstallDependencies()
		runBuildAll()
	case "sign-dmg":
		// Sign a CI-built DMG with local certificate
		if target == "" {
			fmt.Println("Usage: go run cmd/build-tool/main.go sign-dmg <path-to-dmg>")
			os.Exit(1)
		}
		signCIDmg(target)
	case "publish":
		// Publish release from CI artifacts
		publishRelease()
	case "targets":
		showTargets()
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
	fmt.Println("Usage: go run cmd/build-tool/main.go [command] [target]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  build       Build the Gleip application (default)")
	fmt.Println("  build-all   Build for all supported platforms")
	fmt.Println("  dev         Run in development mode")
	fmt.Println("  certs       Generate certificate files only")
	fmt.Println("  save-certs  Generate and save certificate databases for CI reuse")
	fmt.Println("  sign-dmg    Sign a CI-built DMG with local certificate")
	fmt.Println("  publish     Download CI artifacts, sign DMGs, and publish release")
	fmt.Println("  clean       Remove build artifacts")
	fmt.Println("  deps        Install platform dependencies")
	fmt.Println("  targets     Show supported build targets")
	fmt.Println("  help        Show this help message")
	fmt.Println("")
	fmt.Println("Build targets (for build command):")
	fmt.Println("  windows/amd64, windows/arm64, darwin/amd64, darwin/arm64, linux/amd64, linux/arm64")
	fmt.Println("  If no target specified, builds for current platform")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  go run cmd/build-tool/main.go build")
	fmt.Println("  go run cmd/build-tool/main.go build windows/amd64")
	fmt.Println("  go run cmd/build-tool/main.go build-all")
	fmt.Println("  go run cmd/build-tool/main.go save-certs  # Run on Linux/Mac, then commit")
	fmt.Println("  go run cmd/build-tool/main.go sign-dmg ./downloaded-build.dmg")
	fmt.Println("  go run cmd/build-tool/main.go publish    # Download, sign, and release")
	fmt.Println("  go run cmd/build-tool/main.go dev")
}

func showTargets() {
	fmt.Println("Supported build targets:")
	for _, target := range supportedTargets {
		fmt.Printf("  %s/%s\n", target.OS, target.Arch)
	}
}

func runBuildAll() {
	fmt.Println("=== Building for All Platforms ===")

	// Check dependencies first
	checkBuildDependencies()

	// First generate certificates (only needed once)
	fmt.Println("Generating certificates...")
	runCommand("certs", "")

	// Install dependencies for current platform
	fmt.Println("Installing dependencies...")
	runCmdSilent("go", "mod", "tidy")
	if fileExists("frontend") {
		runCmdInDirSilent("frontend", "npm", "install")
	}

	// Note about cross-compilation limitations
	fmt.Println("\n⚠️  Note: Wails has limitations with cross-compilation for GUI applications.")
	fmt.Printf("Building for current platform (%s/%s) and attempting cross-compilation for others.\n", runtime.GOOS, runtime.GOARCH)
	fmt.Println("For best results, build natively on each target platform.\n")

	// Build for each target
	successCount := 0
	failCount := 0

	for _, target := range supportedTargets {
		targetStr := fmt.Sprintf("%s/%s", target.OS, target.Arch)
		fmt.Printf("\n--- Building for %s ---\n", targetStr)

		if target.OS == runtime.GOOS && target.Arch == runtime.GOARCH {
			// Native build - this should work reliably
			fmt.Println("🏠 Native build - should work reliably")
			if buildForTarget(target) {
				fmt.Printf("✅ Build successful for %s\n", targetStr)
				successCount++
			} else {
				fmt.Printf("❌ Build failed for %s\n", targetStr)
				failCount++
			}
		} else {
			// Cross-compilation - may have limitations
			fmt.Println("🌐 Cross-compilation - may have limitations")
			if buildForTarget(target) {
				fmt.Printf("✅ Build successful for %s\n", targetStr)
				successCount++
			} else {
				fmt.Printf("❌ Build failed for %s (this is expected for cross-compilation)\n", targetStr)
				failCount++
			}
		}
	}

	fmt.Printf("\n=== Build Summary ===\n")
	fmt.Printf("✅ Successful builds: %d\n", successCount)
	fmt.Printf("❌ Failed builds: %d\n", failCount)

	if failCount > 0 {
		fmt.Println("\n💡 Tips for failed cross-compilation builds:")
		fmt.Println("  - Use GitHub Actions CI/CD for multi-platform builds")
		fmt.Println("  - Build natively on each target platform")
		fmt.Println("  - Consider using our provided CI/CD pipeline")
	}

	if successCount > 0 {
		fmt.Println("\nBuilt artifacts:")
		listBuiltArtifacts()
	}
}

func checkBuildDependencies() {
	fmt.Println("Checking build dependencies...")

	// Check Go
	if !commandExists("go") {
		fmt.Println("❌ Go not found. Please install Go.")
		os.Exit(1)
	}

	// Check Node.js
	if !commandExists("node") {
		fmt.Println("❌ Node.js not found. Please install Node.js.")
		os.Exit(1)
	}

	// Check npm
	if !commandExists("npm") {
		fmt.Println("❌ npm not found. Please install npm.")
		os.Exit(1)
	}

	// Check wails (should already be installed by checkAndInstallDependencies)
	if !commandExists("wails") {
		fmt.Println("❌ Wails not found. This shouldn't happen after dependency check.")
		os.Exit(1)
	}

	fmt.Println("✅ All build dependencies found")
}

func buildForTarget(target BuildTarget) bool {
	isNative := target.OS == runtime.GOOS && target.Arch == runtime.GOARCH

	// Create target-specific output directory
	var outputDir string
	if isNative {
		outputDir = "./build/bin"
	} else {
		outputDir = fmt.Sprintf("./build/bin/%s-%s", target.OS, target.Arch)
	}
	os.MkdirAll(outputDir, 0755)

	// Extract version for ldflags
	version := extractVersion()

	// Get PostHog configuration from environment variables
	posthogAPIKey := os.Getenv("POSTHOG_API_KEY")
	posthogEndpoint := os.Getenv("POSTHOG_ENDPOINT")

	// Use default endpoint if not specified
	if posthogEndpoint == "" {
		posthogEndpoint = "https://eu.i.posthog.com"
	}

	// If no API key is provided, telemetry will be disabled at runtime
	if posthogAPIKey == "" {
		fmt.Println("⚠️  No POSTHOG_API_KEY provided - telemetry will be disabled")
		posthogAPIKey = "disabled" // Placeholder to indicate disabled state
	}

	// Build ldflags with version and PostHog configuration
	ldflags := fmt.Sprintf("-X 'Gleip/backend.AppVersion=%s' -X 'Gleip/backend.PostHogAPIKey=%s' -X 'Gleip/backend.PostHogEndpoint=%s'",
		version, posthogAPIKey, posthogEndpoint)

	// Prepare wails command
	var args []string
	if isNative {
		// Native build - use simpler approach
		args = []string{"build", "-clean", "-ldflags", ldflags}
	} else {
		// Cross-compilation - try platform flag
		args = []string{"build", "-clean", "-platform", fmt.Sprintf("%s/%s", target.OS, target.Arch), "-ldflags", ldflags}
	}

	// Add output directory
	args = append(args, "-o", outputDir)

	fmt.Printf("Running: wails %s\n", strings.Join(args, " "))
	cmd := exec.Command("wails", args...)

	// Set environment variables for cross-compilation
	env := os.Environ()
	env = append(env, fmt.Sprintf("GOOS=%s", target.OS))
	env = append(env, fmt.Sprintf("GOARCH=%s", target.Arch))
	cmd.Env = env

	// For cross-compilation, capture output to provide better feedback
	if isNative {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Native build failed: %v\n", err)
			return false
		}
	} else {
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Cross-compilation failed (expected): %v\n", err)
			if strings.Contains(string(output), "exec format error") {
				fmt.Println("  → This is a common cross-compilation limitation")
			}
			return false
		}
	}

	// Check if build artifacts were created
	if !verifyBuildArtifacts(outputDir, target) {
		fmt.Printf("Build completed but no artifacts found for %s/%s\n", target.OS, target.Arch)
		// For cross-compilation, also check if artifacts were created in unexpected locations
		if !isNative {
			alternativeLocations := []string{
				"./build/bin",
				fmt.Sprintf("./build/bin/%s", target.OS),
				"./dist",
			}
			for _, altDir := range alternativeLocations {
				if verifyBuildArtifacts(altDir, target) {
					fmt.Printf("  → Found artifacts in %s instead\n", altDir)
					// Move artifacts to expected location
					moveArtifacts(altDir, outputDir, target)
					return true
				}
			}
		}
		return false
	}

	// Post-build packaging for specific platforms
	if target.OS == "darwin" {
		packageMacOSCrossCompiled(outputDir, version, target.Arch)
	}

	return true
}

func moveArtifacts(fromDir, toDir string, target BuildTarget) {
	var artifactName string
	switch target.OS {
	case "windows":
		artifactName = "Gleip.exe"
	case "darwin":
		artifactName = "Gleip.app"
	case "linux":
		artifactName = "Gleip"
	}

	fromPath := filepath.Join(fromDir, artifactName)
	toPath := filepath.Join(toDir, artifactName)

	if fileExists(fromPath) {
		os.MkdirAll(toDir, 0755)
		if err := os.Rename(fromPath, toPath); err != nil {
			fmt.Printf("  → Failed to move artifact: %v\n", err)
		} else {
			fmt.Printf("  → Moved artifact to %s\n", toPath)
		}
	}
}

func verifyBuildArtifacts(outputDir string, target BuildTarget) bool {
	switch target.OS {
	case "windows":
		return fileExists(filepath.Join(outputDir, "Gleip.exe"))
	case "darwin":
		return fileExists(filepath.Join(outputDir, "Gleip.app"))
	case "linux":
		return fileExists(filepath.Join(outputDir, "Gleip"))
	}
	return false
}

func listBuiltArtifacts() {
	fmt.Println("Built artifacts:")

	// Check native build first
	nativePath := "./build/bin"
	if target := findNativeArtifact(nativePath); target != "" {
		fmt.Printf("  Native (%s/%s): %s\n", runtime.GOOS, runtime.GOARCH, target)
	}

	// Check cross-compiled builds
	for _, target := range supportedTargets {
		targetDir := fmt.Sprintf("./build/bin/%s-%s", target.OS, target.Arch)
		if artifacts := findArtifactsInDir(targetDir, target); len(artifacts) > 0 {
			for _, artifact := range artifacts {
				fmt.Printf("  %s/%s: %s\n", target.OS, target.Arch, artifact)
			}
		}
	}
}

func findNativeArtifact(dir string) string {
	patterns := []string{"Gleip.exe", "Gleip.app", "Gleip"}
	for _, pattern := range patterns {
		path := filepath.Join(dir, pattern)
		if fileExists(path) {
			return path
		}
	}
	return ""
}

func findArtifactsInDir(dir string, target BuildTarget) []string {
	var artifacts []string

	if !fileExists(dir) {
		return artifacts
	}

	// Look for platform-specific artifacts
	switch target.OS {
	case "windows":
		if path := filepath.Join(dir, "Gleip.exe"); fileExists(path) {
			artifacts = append(artifacts, path)
		}
	case "darwin":
		if path := filepath.Join(dir, "Gleip.app"); fileExists(path) {
			artifacts = append(artifacts, path)
		}
		// Also check for DMG files
		if files, err := filepath.Glob(filepath.Join(dir, "*.dmg")); err == nil {
			artifacts = append(artifacts, files...)
		}
	case "linux":
		if path := filepath.Join(dir, "Gleip"); fileExists(path) {
			artifacts = append(artifacts, path)
		}
	}

	return artifacts
}

func runClean() {
	fmt.Println("=== Gleip Clean Process ===")
	fmt.Println("Removing build artifacts...")

	dirsToRemove := []string{
		"./frontend/node_modules",
		"./frontend/dist",
		"./.wails",
		tempCertdbDir,
		"./build/bin", // Remove entire bin directory including cross-compiled builds
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

func runCommand(command string, targetPlatform string) {
	fmt.Printf("=== Gleip Process (Mode: %s) ===\n", command)
	if targetPlatform != "" {
		fmt.Printf("Target Platform: %s\n", targetPlatform)
	} else {
		fmt.Printf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	}

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
	ldflags := fmt.Sprintf("-X 'Gleip/backend.AppVersion=%s'", version)

	// Build or dev
	switch command {
	case "build":
		if targetPlatform != "" {
			buildSpecificTarget(targetPlatform)
		} else {
			fmt.Printf("Building application for current platform (version: %s)...\n", version)
			runCmd("wails", "build", "-clean", "-ldflags", ldflags)

			// Package application
			packageApplication(version)
		}

	case "dev":
		fmt.Println("Starting development server...")
		runCmd("wails", "dev", "-ldflags", ldflags)
	}

	fmt.Printf("=== Process Complete (Mode: %s) ===\n", command)
}

func buildSpecificTarget(targetPlatform string) {
	parts := strings.Split(targetPlatform, "/")
	if len(parts) != 2 {
		fmt.Printf("Invalid target format: %s. Use format: os/arch (e.g., windows/amd64)\n", targetPlatform)
		os.Exit(1)
	}

	targetOS := parts[0]
	targetArch := parts[1]

	// Validate target
	validTarget := false
	for _, target := range supportedTargets {
		if target.OS == targetOS && target.Arch == targetArch {
			validTarget = true
			break
		}
	}

	if !validTarget {
		fmt.Printf("Unsupported target: %s\n", targetPlatform)
		fmt.Println("Run 'go run cmd/build-tool/main.go targets' to see supported targets")
		os.Exit(1)
	}

	fmt.Printf("Building for %s...\n", targetPlatform)

	target := BuildTarget{OS: targetOS, Arch: targetArch}
	if buildForTarget(target) {
		fmt.Printf("✅ Build successful for %s\n", targetPlatform)
	} else {
		fmt.Printf("❌ Build failed for %s\n", targetPlatform)
		os.Exit(1)
	}
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

func packageApplication(version string) {
	switch runtime.GOOS {
	case "darwin":
		packageMacOS(version)
	case "windows":
		packageWindows(version)
	case "linux":
		packageLinux(version)
	}
}

func packageMacOS(version string) {
	appPath := "./build/bin/Gleip.app"
	if !fileExists(appPath) {
		return
	}

	// Code signing logic
	if commandExists("codesign") {
		if os.Getenv("GLEIP_CI_BUILD") != "" && os.Getenv("ENABLE_CODE_SIGNING") != "true" {
			fmt.Println("Skipping code signing in CI environment...")
		} else if os.Getenv("ENABLE_CODE_SIGNING") == "true" {
			// Release builds with proper certificates
			fmt.Println("Signing application for release...")
			runCmd("codesign", "--force", "--deep", "--sign", "Gleip.io", appPath)

			// Verify the signature
			fmt.Println("Verifying code signature...")
			runCmd("codesign", "--verify", "--verbose", appPath)
		} else {
			// Local development builds
			fmt.Println("Signing application with development certificate...")
			runCmd("codesign", "--force", "--deep", "--sign", "Gleip.io", appPath)
		}
	}

	// Create DMG with standard naming
	arch := runtime.GOARCH
	dmgName := fmt.Sprintf("gleip-%s-macos-%s.dmg", version, arch)
	dmgPath := filepath.Join("./build/bin", dmgName)
	tempDir := "./build/dmg_temp"

	os.MkdirAll(tempDir, 0755)
	runCmd("cp", "-R", appPath, tempDir+"/")

	if commandExists("create-dmg") {
		runCmd("create-dmg", "--volname", "Gleip Installer", "--window-size", "800", "400", "--app-drop-link", "600", "185", dmgPath, tempDir)
	} else if commandExists("hdiutil") {
		// Create Applications symlink for hdiutil
		runCmd("ln", "-s", "/Applications", tempDir+"/Applications")
		runCmd("hdiutil", "create", "-volname", "Gleip Installer", "-srcfolder", tempDir, "-ov", "-format", "UDZO", dmgPath)
	}

	os.RemoveAll(tempDir)
	fmt.Printf("✅ Distribution DMG created: %s\n", dmgPath)
}

func packageMacOSCrossCompiled(outputDir, version, arch string) {
	appPath := filepath.Join(outputDir, "Gleip.app")
	if !fileExists(appPath) {
		return
	}

	fmt.Printf("Packaging macOS %s build...\n", arch)

	// Create DMG for cross-compiled build
	dmgName := fmt.Sprintf("gleip-%s-macos-%s.dmg", version, arch)
	dmgPath := filepath.Join(outputDir, dmgName)
	tempDir := filepath.Join(outputDir, "dmg_temp")

	os.MkdirAll(tempDir, 0755)
	runCmd("cp", "-R", appPath, tempDir+"/")

	if commandExists("hdiutil") {
		// Create Applications symlink for hdiutil
		runCmd("ln", "-s", "/Applications", tempDir+"/Applications")
		runCmd("hdiutil", "create", "-volname", "Gleip Installer", "-srcfolder", tempDir, "-ov", "-format", "UDZO", dmgPath)
		os.RemoveAll(tempDir)
		fmt.Printf("✅ Cross-compiled DMG created: %s\n", dmgPath)
	}
}

func packageWindows(version string) {
	exePath := "./build/bin/Gleip.exe"
	if fileExists(exePath) {
		fmt.Printf("✅ Executable created: %s\n", exePath)
	}
}

func packageLinux(version string) {
	binPath := "./build/bin/Gleip"
	if fileExists(binPath) {
		fmt.Printf("✅ Executable created: %s\n", binPath)
	}
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

func runCmdSilent(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
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

func runCmdInDirSilent(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd.Run()
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
		runCmd("create-dmg", "--volname", "Gleip Installer", "--window-size", "800", "400", signedDmgPath, extractDir)
	} else if commandExists("hdiutil") {
		runCmd("hdiutil", "create", "-volname", "Gleip Installer", "-srcfolder", extractDir, "-ov", "-format", "UDZO", signedDmgPath)
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

	// Check if target repository exists and is accessible
	fmt.Println("🔍 Checking access to gleipio/gleip repository...")
	cmd := exec.Command("gh", "repo", "view", "gleipio/gleip", "--json", "name")
	if output, err := cmd.Output(); err != nil {
		fmt.Println("❌ Cannot access gleipio/gleip repository.")
		fmt.Println("Please ensure:")
		fmt.Println("  1. The repository exists")
		fmt.Println("  2. You have push/release permissions")
		fmt.Println("  3. You're authenticated with: gh auth login")
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	} else {
		fmt.Printf("✅ Repository access confirmed: %s\n", strings.TrimSpace(string(output)))
	}

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
	downloadDir := "./build/release_temp"
	os.RemoveAll(downloadDir)
	os.MkdirAll(downloadDir, 0755)
	defer os.RemoveAll(downloadDir)

	// Get latest workflow run from vabbb/gleip
	fmt.Println("🔍 Finding latest CI build from vabbb/gleip...")
	cmd = exec.Command("gh", "run", "list", "--repo", "vabbb/gleip", "--limit", "1", "--status", "completed", "--json", "databaseId")
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("❌ Failed to get latest workflow run: %v\n", err)
		os.Exit(1)
	}

	// Parse run ID (simple JSON parsing)
	runID := ""
	outputStr := string(output)
	if strings.Contains(outputStr, "databaseId") {
		re := regexp.MustCompile(`"databaseId":(\d+)`)
		matches := re.FindStringSubmatch(outputStr)
		if len(matches) > 1 {
			runID = matches[1]
		}
	}

	if runID == "" {
		fmt.Println("❌ Could not find latest workflow run ID")
		os.Exit(1)
	}
	fmt.Printf("✅ Found latest CI run: %s\n", runID)

	// Download all artifacts
	fmt.Println("⬇️  Downloading CI artifacts...")
	cmd = exec.Command("gh", "run", "download", runID, "--repo", "vabbb/gleip", "--dir", downloadDir)
	if err := cmd.Run(); err != nil {
		fmt.Printf("❌ Failed to download artifacts: %v\n", err)
		os.Exit(1)
	}

	// Find and sign DMG files
	fmt.Println("🔏 Processing macOS DMG files...")
	err = filepath.Walk(downloadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(info.Name(), ".dmg") && strings.Contains(info.Name(), "darwin") {
			fmt.Printf("📱 Signing DMG: %s\n", info.Name())
			signDMGInPlace(path)
		}
		return nil
	})
	if err != nil {
		fmt.Printf("❌ Error processing DMG files: %v\n", err)
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
		if strings.HasSuffix(info.Name(), ".exe") ||
			strings.HasSuffix(info.Name(), ".dmg") ||
			strings.HasSuffix(info.Name(), ".tar.gz") ||
			(info.Name() == "Gleip" && !info.IsDir()) ||
			(strings.HasPrefix(info.Name(), "gleip-") && (strings.Contains(info.Name(), "-linux-") || strings.Contains(info.Name(), "-windows-") || strings.Contains(info.Name(), "-macos-")) && !info.IsDir()) ||
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
	cmd = exec.Command("gh", "release", "view", version, "--repo", "gleipio/gleip")
	if err := cmd.Run(); err == nil {
		fmt.Printf("⚠️  Release %s already exists. Deleting it first...\n", version)
		cmd = exec.Command("gh", "release", "delete", version, "--repo", "gleipio/gleip", "--yes")
		if err := cmd.Run(); err != nil {
			fmt.Printf("❌ Failed to delete existing release: %v\n", err)
			os.Exit(1)
		}
	}

	// Create release on gleipio/gleip
	fmt.Printf("🚀 Creating release %s on gleipio/gleip...\n", version)

	// First create the release without assets
	cmd = exec.Command("gh", "release", "create", version,
		"--repo", "gleipio/gleip",
		"--title", "Gleip v"+version,
		"--notes", string(releaseNotes))

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("❌ Failed to create release: %v\n", err)
		fmt.Println("\nTroubleshooting:")
		fmt.Println("1. Ensure the repository gleipio/gleip exists")
		fmt.Println("2. Check you have release permissions: gh auth refresh")
		fmt.Println("3. Try manually: gh release create " + version + " --repo gleipio/gleip")
		os.Exit(1)
	}

	// Then upload each artifact separately
	fmt.Println("📤 Uploading artifacts...")
	for _, artifactPath := range artifactPaths {
		fileName := filepath.Base(artifactPath)
		fmt.Printf("  Uploading %s...\n", fileName)

		cmd = exec.Command("gh", "release", "upload", version, artifactPath,
			"--repo", "gleipio/gleip")

		if err := cmd.Run(); err != nil {
			fmt.Printf("❌ Failed to upload %s: %v\n", fileName, err)
			// Continue with other files rather than exit
		} else {
			fmt.Printf("  ✅ %s uploaded\n", fileName)
		}
	}

	fmt.Printf("✅ Release v%s published successfully!\n", version)
	fmt.Printf("🌐 View at: https://github.com/gleipio/gleip/releases/tag/v%s\n", version)
}

func signDMGInPlace(dmgPath string) {
	// Create temporary working directory for this DMG
	tempDir := filepath.Join(filepath.Dir(dmgPath), "sign_temp_"+filepath.Base(dmgPath))
	mountPoint := filepath.Join(tempDir, "mount")
	extractDir := filepath.Join(tempDir, "extract")

	os.RemoveAll(tempDir)
	os.MkdirAll(mountPoint, 0755)
	os.MkdirAll(extractDir, 0755)

	defer os.RemoveAll(tempDir)

	// Mount the DMG
	cmd := exec.Command("hdiutil", "attach", dmgPath, "-mountpoint", mountPoint, "-nobrowse", "-quiet")
	if err := cmd.Run(); err != nil {
		fmt.Printf("❌ Failed to mount DMG: %v\n", err)
		return
	}

	// Ensure we unmount on exit
	defer func() {
		exec.Command("hdiutil", "detach", mountPoint, "-quiet").Run()
	}()

	// Find the .app bundle
	appPath := ""
	entries, err := os.ReadDir(mountPoint)
	if err != nil {
		fmt.Printf("❌ Failed to read mounted DMG contents: %v\n", err)
		return
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".app") {
			appPath = filepath.Join(mountPoint, entry.Name())
			break
		}
	}

	if appPath == "" {
		fmt.Printf("❌ No .app bundle found in DMG: %s\n", filepath.Base(dmgPath))
		return
	}

	// Copy .app to extract directory
	extractedAppPath := filepath.Join(extractDir, filepath.Base(appPath))
	runCmdSilent("cp", "-R", appPath, extractDir)

	// Unmount before signing
	runCmdSilent("hdiutil", "detach", mountPoint, "-quiet")

	// Sign the extracted .app
	if err := runCmdSilent("codesign", "--force", "--deep", "--sign", "Gleip.io", extractedAppPath); err != nil {
		fmt.Printf("❌ Failed to sign app in DMG: %s\n", filepath.Base(dmgPath))
		return
	}

	// Replace original DMG with signed version
	if commandExists("create-dmg") {
		runCmdSilent("create-dmg", "--volname", "Gleip Installer", "--window-size", "800", "400", "--app-drop-link", "600", "185", dmgPath, extractDir)
	} else {
		// Create Applications symlink for hdiutil
		runCmdSilent("ln", "-s", "/Applications", extractDir+"/Applications")
		runCmdSilent("hdiutil", "create", "-volname", "Gleip Installer", "-srcfolder", extractDir, "-ov", "-format", "UDZO", dmgPath)
	}

	fmt.Printf("✅ DMG signed: %s\n", filepath.Base(dmgPath))
}
