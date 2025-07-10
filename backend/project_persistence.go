package backend

import (
	"Gleip/backend/chef"
	"Gleip/backend/network"
	"Gleip/backend/paths"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	rt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// init registers all types that need to be serialized with gob
func init() {
	// Register chef types that are embedded in other structures
	gob.Register(&chef.ChefStep{})
	gob.Register(&chef.ChefAction{})
	gob.Register([]chef.ChefAction{})

	// Register network types that might be used
	gob.Register(&network.HTTPTransaction{})
	gob.Register(&network.HTTPRequest{})
	gob.Register(&network.HTTPResponse{})

	// Register all gleipflow-related types
	gob.Register(&GleipFlow{})
	gob.Register([]*GleipFlow{})
	gob.Register(&GleipFlowStep{})
	gob.Register([]GleipFlowStep{})
	gob.Register(&RequestStep{})
	gob.Register(&ScriptStep{})
	gob.Register(&ExecutionResult{})
	gob.Register([]ExecutionResult{})
	gob.Register(&VariableExtract{})
	gob.Register([]VariableExtract{})
	gob.Register(&FuzzSettings{})
	gob.Register(&FuzzResult{})
	gob.Register([]FuzzResult{})
	gob.Register(&PhantomRequest{})
	gob.Register([]PhantomRequest{})

	// Register API collection types
	gob.Register(&APICollection{})
	gob.Register([]APICollection{})
	gob.Register(&APIVariable{})
	gob.Register([]APIVariable{})
	gob.Register(&APIRequest{})
	gob.Register([]APIRequest{})
	gob.Register(&APIHeader{})
	gob.Register([]APIHeader{})
	gob.Register(&APIExample{})
	gob.Register([]APIExample{})
	gob.Register(&APIParameter{})
	gob.Register([]APIParameter{})
	gob.Register(&APISecurityScheme{})
	gob.Register([]APISecurityScheme{})

	// Register common map and slice types
	gob.Register(map[string]interface{}{})
	gob.Register(map[string]string{})
	gob.Register([]string{})
}

// ProjectPersistence handles all project file operations
type ProjectPersistence struct {
	app *App
}

// NewProjectPersistence creates a new ProjectPersistence instance
func NewProjectPersistence(app *App) *ProjectPersistence {
	return &ProjectPersistence{app: app}
}

// saveProjectToPath saves a project to a specific file path using binary encoding
func (pp *ProjectPersistence) saveProjectToPath(project *Project, fullPath string) error {
	if project == nil {
		return fmt.Errorf("no project to save")
	}
	if fullPath == "" {
		return fmt.Errorf("file path cannot be empty for saving project")
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dir, err)
	}

	// Set up a temporary file to avoid corrupting the project file
	tempFile, err := os.CreateTemp(dir, "*.gleip.bak")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %v", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath) // Clean up on either success or failure

	// Use gob encoder for binary serialization
	encoder := gob.NewEncoder(tempFile)
	if err := encoder.Encode(project); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to encode project: %v", err)
	}

	// Close the temporary file
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %v", err)
	}

	// Move the temporary file to the target location
	if err := os.Rename(tempPath, fullPath); err != nil {
		return fmt.Errorf("failed to save project file to %s: %v", fullPath, err)
	}

	return nil
}

// loadProjectFromPath loads a project from disk by path using binary decoding
func (pp *ProjectPersistence) loadProjectFromPath(filePath string) (*Project, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path cannot be empty for loading project")
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open project file from %s: %v", filePath, err)
	}
	defer file.Close()

	var project Project
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&project); err != nil {
		return nil, fmt.Errorf("failed to decode project file %s: %v", filePath, err)
	}

	// Set the file path
	project.FilePathOnDisk = filePath

	return &project, nil
}

// updateProjectFromCurrentStateData populates a given Project struct with current app state.
func (pp *ProjectPersistence) updateProjectFromCurrentStateData(projectData *Project) error {
	if projectData == nil {
		return fmt.Errorf("projectData cannot be nil")
	}
	projectData.UpdatedAt = time.Now().Format(time.RFC3339)

	// Capture request history
	if pp.app.proxyServer != nil && pp.app.proxyServer.transactionStore != nil {
		allTransactions := pp.app.proxyServer.transactionStore.GetAll()
		history := make([]*network.HTTPTransaction, len(allTransactions))
		for i := range allTransactions {
			txCopy := allTransactions[i]
			history[i] = &txCopy
		}
		projectData.RequestHistory = history
	}

	// Capture gleipFlows
	pp.app.gleipFlowsMutex.RLock()
	gleipFlows := make([]*GleipFlow, 0, len(pp.app.gleipFlowsCache))
	for _, gleipFlow := range pp.app.gleipFlowsCache {
		gleipFlows = append(gleipFlows, gleipFlow)
	}
	projectData.GleipFlows = gleipFlows
	pp.app.gleipFlowsMutex.RUnlock()

	return nil
}

// updateProjectFromCurrentStateDataIncremental updates only the dirty components of the project
func (pp *ProjectPersistence) updateProjectFromCurrentStateDataIncremental(projectData *Project, dirtyComponents []string) error {
	if projectData == nil {
		return fmt.Errorf("projectData cannot be nil")
	}

	// Always update the timestamp when saving
	projectData.UpdatedAt = time.Now().Format(time.RFC3339)

	for _, component := range dirtyComponents {
		switch component {
		case "request_history":
			// Capture request history
			if pp.app.proxyServer != nil && pp.app.proxyServer.transactionStore != nil {
				allTransactions := pp.app.proxyServer.transactionStore.GetAll()
				history := make([]*network.HTTPTransaction, len(allTransactions))
				for i := range allTransactions {
					txCopy := allTransactions[i]
					history[i] = &txCopy
				}
				projectData.RequestHistory = history
			}

		case "gleip_flows":
			// Capture gleipFlows
			pp.app.gleipFlowsMutex.RLock()
			gleipFlows := make([]*GleipFlow, 0, len(pp.app.gleipFlowsCache))
			for _, gleipFlow := range pp.app.gleipFlowsCache {
				gleipFlows = append(gleipFlows, gleipFlow)
				fmt.Printf("DEBUG: Saving flow %s with %d execution results\n", gleipFlow.ID, len(gleipFlow.ExecutionResults))
			}
			projectData.GleipFlows = gleipFlows
			pp.app.gleipFlowsMutex.RUnlock()

		case "api_collections":
			// API collections are already stored in the project, no need to copy from cache
			// This is handled differently since they're stored directly in the project

		case "project_meta":
			// Project metadata like name, settings are already in the project struct
			// This flag is used when these fields are modified directly
		}
	}

	return nil
}

// saveToTempProject saves the current project state to the current auto-save target
func (pp *ProjectPersistence) saveToTempProject() error {
	if pp.app.currentProject == nil {
		return fmt.Errorf("no active project to save")
	}

	// Update project data before saving
	if err := pp.updateProjectFromCurrentStateData(pp.app.currentProject); err != nil {
		return fmt.Errorf("failed to update project data: %v", err)
	}

	// Create a project copy to save
	projectToSave := *pp.app.currentProject

	// Save to the current auto-save target (could be user's file or temp file)
	err := pp.saveProjectToPath(&projectToSave, pp.app.tempProjectPath)
	if err != nil {
		return fmt.Errorf("failed to save project: %v", err)
	}

	return nil
}

// saveToTempProjectIncremental saves only the dirty components to the temporary file
func (pp *ProjectPersistence) saveToTempProjectIncremental() error {
	if pp.app.currentProject == nil {
		return fmt.Errorf("no active project to save")
	}

	// Check if there are any dirty components
	if !pp.app.hasDirtyComponents() {
		fmt.Printf("Debug: No dirty components, skipping incremental save\n")
		return nil
	}

	dirtyComponents := pp.app.getDirtyComponents()
	fmt.Printf("DEBUG: Incremental save - dirty components: %v\n", dirtyComponents)

	// Update only dirty components
	if err := pp.updateProjectFromCurrentStateDataIncremental(pp.app.currentProject, dirtyComponents); err != nil {
		return fmt.Errorf("failed to update project data: %v", err)
	}

	// Create a project copy to save
	projectToSave := *pp.app.currentProject

	// Determine the correct save path - use the project's actual file path if it exists
	savePath := pp.app.tempProjectPath
	if pp.app.currentProject.FilePathOnDisk != "" {
		savePath = pp.app.currentProject.FilePathOnDisk
	}

	fmt.Printf("DEBUG: Saving incremental changes to: %s\n", savePath)

	// Save to the correct location
	err := pp.saveProjectToPath(&projectToSave, savePath)
	if err != nil {
		return fmt.Errorf("failed to save project: %v", err)
	}

	// Clear dirty flags after successful save
	pp.app.clearDirtyFlags()

	fmt.Printf("DEBUG: Incremental save completed successfully\n")

	return nil
}

// ProjectTransitionResult represents the result of a project transition operation
type ProjectTransitionResult int

const (
	TransitionSuccess ProjectTransitionResult = iota
	TransitionCancelled
	TransitionError
)

// hasProjectMeaningfulData checks if the current application state contains meaningful data worth saving
func (pp *ProjectPersistence) hasProjectMeaningfulData() bool {
	if pp.app.currentProject == nil {
		return false
	}

	// Check current proxy server for HTTP transactions (not just saved history)
	if pp.app.proxyServer != nil && pp.app.proxyServer.transactionStore != nil {
		allTransactions := pp.app.proxyServer.transactionStore.GetAll()
		if len(allTransactions) > 0 {
			return true
		}
	}

	// Check current gleipFlows cache for flows with actual steps
	pp.app.gleipFlowsMutex.RLock()
	for _, gleipFlow := range pp.app.gleipFlowsCache {
		if gleipFlow != nil && len(gleipFlow.Steps) > 0 {
			pp.app.gleipFlowsMutex.RUnlock()
			return true
		}
	}
	pp.app.gleipFlowsMutex.RUnlock()

	// Check current API collections in project
	if len(pp.app.currentProject.APICollections) > 0 {
		return true
	}

	// Fallback: check saved project data (for backward compatibility)
	// Check if there are any HTTP transactions in saved history
	if len(pp.app.currentProject.RequestHistory) > 0 {
		return true
	}

	// Check if there are any saved GleipFlows with actual steps
	if len(pp.app.currentProject.GleipFlows) > 0 {
		for _, gleipFlow := range pp.app.currentProject.GleipFlows {
			if gleipFlow != nil && len(gleipFlow.Steps) > 0 {
				return true
			}
		}
	}

	return false
}

// handleUnsavedProjectTransition handles the workflow when transitioning away from an unsaved project
func (pp *ProjectPersistence) handleUnsavedProjectTransition(actionName string) ProjectTransitionResult {
	// Check if current project is unsaved temporary project
	defaultTempPath := filepath.Join(paths.GlobalPaths.AppDataDir, "temp_project.gleip")
	if pp.app.currentProject == nil || pp.app.tempProjectPath != defaultTempPath {
		// Not an unsaved temp project, proceed
		return TransitionSuccess
	}

	// Check if it has meaningful data
	if !pp.hasProjectMeaningfulData() {
		// Project has no meaningful data, silently proceed without prompting
		fmt.Printf("Current temp project has no meaningful data, proceeding with %s without prompting\n", actionName)
		if err := pp.saveToTempProject(); err != nil {
			fmt.Printf("Warning: Failed to save current state to temp project: %v\n", err)
		}
		return TransitionSuccess
	}

	// Check if context is available for showing dialogs
	if pp.app.ctx == nil {
		// No context available (e.g., during startup), just save current state
		fmt.Printf("No context available for dialog, saving current state before %s\n", actionName)
		if err := pp.saveToTempProject(); err != nil {
			fmt.Printf("Warning: Failed to save current state to temp project: %v\n", err)
		}
		return TransitionSuccess
	}

	// Project has meaningful data, prompt user to save it
	result, err := rt.MessageDialog(pp.app.ctx, rt.MessageDialogOptions{
		Type:          rt.QuestionDialog,
		Title:         "Save Current Project",
		Message:       fmt.Sprintf("You have an unsaved project with meaningful data. Would you like to save it before %s?", actionName),
		Buttons:       []string{"Save", "Don't Save", "Cancel"},
		DefaultButton: "Save",
	})

	if err != nil {
		fmt.Printf("Error showing save dialog: %v\n", err)
		return TransitionError
	}

	switch result {
	case "Save":
		// User wants to save, call saveProjectAs
		if err := pp.promptSaveProjectAs(); err != nil {
			// If save fails or user cancels, don't proceed
			fmt.Printf("Save operation failed or was cancelled: %v\n", err)
			return TransitionCancelled
		}
		return TransitionSuccess

	case "Cancel":
		// User cancelled, don't proceed
		return TransitionCancelled

	case "Don't Save":
		// User chose not to save, proceed
		// Save current state to temp file one last time before discarding
		if err := pp.saveToTempProject(); err != nil {
			fmt.Printf("Warning: Failed to save current state to temp project: %v\n", err)
		}
		return TransitionSuccess

	default:
		return TransitionCancelled
	}
}

// promptSaveProjectAs presents the save dialog and handles the save workflow
func (pp *ProjectPersistence) promptSaveProjectAs() error {
	pp.app.projectMutex.Lock()

	if pp.app.currentProject == nil {
		pp.app.projectMutex.Unlock()
		return fmt.Errorf("no active project to save")
	}

	// Save to the temp project first to ensure all current state is captured
	if err := pp.saveToTempProject(); err != nil {
		pp.app.projectMutex.Unlock()
		return fmt.Errorf("failed to update temporary project: %v", err)
	}

	// Store the current temp path to clean it up later
	currentTempPath := pp.app.tempProjectPath
	defaultTempPath := filepath.Join(paths.GlobalPaths.AppDataDir, "temp_project.gleip")

	// Ensure the projects directory exists before showing the save dialog
	if err := paths.EnsureProjectsDir(); err != nil {
		pp.app.projectMutex.Unlock()
		return fmt.Errorf("failed to create projects directory: %v", err)
	}

	// Always use the GleipProjects directory as the default save location
	dialogDefaultDir := paths.GlobalPaths.ProjectsDir

	// Generate a default filename with today's date (yyyy-mm-dd.gleip)
	currentDate := time.Now().Format("2006-01-02")
	defaultFilename := currentDate + ".gleip"

	// If the project has a non-default name, use it instead
	if pp.app.currentProject.Name != defaultProjectName && pp.app.currentProject.Name != "" {
		defaultFilename = pp.app.currentProject.Name + ".gleip"
	}

	pp.app.projectMutex.Unlock() // Unlock before showing dialog

	chosenPath, err := rt.SaveFileDialog(pp.app.ctx, rt.SaveDialogOptions{
		DefaultDirectory: dialogDefaultDir,
		DefaultFilename:  defaultFilename,
		Title:            "Save Project As",
		Filters:          []rt.FileFilter{{DisplayName: "Gleip Project (*.gleip)", Pattern: "*.gleip;*.GLEIP"}},
	})

	if err != nil {
		rt.MessageDialog(pp.app.ctx, rt.MessageDialogOptions{Type: rt.ErrorDialog, Title: "Save Error", Message: fmt.Sprintf("Could not display save dialog: %v", err)})
		return fmt.Errorf("error during save dialog: %v", err)
	}
	if chosenPath == "" {
		return fmt.Errorf("save cancelled by user")
	}

	if !strings.HasSuffix(strings.ToLower(chosenPath), ".gleip") {
		chosenPath += ".gleip"
	}

	newProjectName := strings.TrimSuffix(filepath.Base(chosenPath), ".gleip")
	newProjectName = strings.TrimSuffix(newProjectName, ".GLEIP") // Handle if original was .GLEIP

	if newProjectName == "" {
		rt.MessageDialog(pp.app.ctx, rt.MessageDialogOptions{Type: rt.ErrorDialog, Title: "Invalid Name", Message: "Project name cannot be empty."})
		return fmt.Errorf("project name cannot be empty")
	}

	// Update current project details first
	pp.app.projectMutex.Lock()
	pp.app.currentProject.Name = newProjectName
	pp.app.currentProject.FilePathOnDisk = chosenPath
	pp.app.currentProject.UpdatedAt = time.Now().Format(time.RFC3339)

	// Update project data before saving to the new location
	if err := pp.updateProjectFromCurrentStateData(pp.app.currentProject); err != nil {
		pp.app.projectMutex.Unlock()
		return fmt.Errorf("failed to update project data: %v", err)
	}

	// Create a project copy to save
	projectToSave := *pp.app.currentProject
	pp.app.projectMutex.Unlock()

	// Save directly to the user's chosen location
	if err := pp.saveProjectToPath(&projectToSave, chosenPath); err != nil {
		return fmt.Errorf("failed to save project to %s: %v", chosenPath, err)
	}

	// Switch auto-save target to the user's chosen file (this is the key fix)
	pp.app.tempProjectPath = chosenPath

	// Clean up the old temp file if it was the default temp file
	if currentTempPath == defaultTempPath && currentTempPath != chosenPath {
		if err := os.Remove(currentTempPath); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: Failed to delete temporary project file %s: %v\n", currentTempPath, err)
		} else if err == nil {
			fmt.Printf("Cleaned up temporary project file: %s\n", currentTempPath)
		}
	}

	rt.EventsEmit(pp.app.ctx, "project:saved", pp.app.currentProject.ID)

	// Update window title
	pp.app.updateWindowTitle()

	fmt.Printf("Project saved to: %s \n", chosenPath)
	return nil
}

// CreateNewProject creates a new project, handling any existing unsaved work
func (pp *ProjectPersistence) CreateNewProject() error {
	// Handle transition from current project (only if we're not in startup)
	if pp.app.ctx != nil {
		result := pp.handleUnsavedProjectTransition("creating a new project")
		switch result {
		case TransitionCancelled:
			return nil // User cancelled, operation aborted
		case TransitionError:
			return fmt.Errorf("failed to handle project transition")
		}
	}

	// Store the previous temp project path for cleanup
	previousTempPath := pp.app.tempProjectPath
	defaultTempPath := filepath.Join(paths.GlobalPaths.AppDataDir, "temp_project.gleip")

	// Always wipe the existing temporary project when creating a new one
	if _, err := os.Stat(defaultTempPath); err == nil {
		if err := os.Remove(defaultTempPath); err != nil {
			fmt.Printf("Warning: Failed to remove existing temp project file: %v\n", err)
		} else {
			fmt.Printf("Wiped existing temporary project file\n")
		}
	}

	// Create the new project
	pp.app.projectMutex.Lock()
	pp.app.currentProject = &Project{
		ID:         uuid.New().String(),
		Name:       defaultProjectName,
		CreatedAt:  time.Now().Format(time.RFC3339),
		UpdatedAt:  time.Now().Format(time.RFC3339),
		Variables:  make(map[string]string),
		ProxyPort:  9090,
		GleipFlows: []*GleipFlow{},

		// Initialize default sorting state: descending "#" column
		SortColumn:             "id",
		SortDirection:          "desc",
		SecondarySortColumn:    "id",
		SecondarySortDirection: "desc",
	}

	// Create a default GleipFlow
	firstGleipFlow := &GleipFlow{
		ID:           uuid.New().String(),
		Name:         "GleipFlow 1",
		Steps:        []GleipFlowStep{},
		Variables:    make(map[string]string),
		SortingIndex: 1,
	}

	// Add to project's GleipFlows
	pp.app.currentProject.GleipFlows = append(pp.app.currentProject.GleipFlows, firstGleipFlow)

	// Set as selected gleip flow
	pp.app.currentProject.SelectedGleipFlowID = firstGleipFlow.ID
	pp.app.projectMutex.Unlock()

	// Update the cache
	pp.app.selectedGleipMutex.Lock()
	pp.app.selectedGleipFlowID = firstGleipFlow.ID
	pp.app.selectedGleipMutex.Unlock()

	// Clear existing data (wipe all application state)
	pp.app.clearApplicationState()

	// Add the firstGleipFlow to the cache
	pp.app.gleipFlowsMutex.Lock()
	pp.app.gleipFlowsCache[firstGleipFlow.ID] = firstGleipFlow
	pp.app.gleipFlowsMutex.Unlock()

	// Reset proxy request counter (clear proxy history)
	if pp.app.proxyServer != nil {
		pp.app.proxyServer.ResetRequestCounter()
	}

	// Reset to default temp path for new unsaved projects
	pp.app.tempProjectPath = defaultTempPath

	// Clean up any previous temporary project files (but only if it's the default temp file)
	// Never delete user's saved project files
	if previousTempPath != pp.app.tempProjectPath && previousTempPath != "" && previousTempPath == defaultTempPath {
		if err := os.Remove(previousTempPath); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: Failed to delete previous temp project file %s: %v\n", previousTempPath, err)
		} else if err == nil {
			fmt.Printf("Deleted previous temp project file: %s\n", previousTempPath)
		}
	}

	// Save the new clean project to the temp file
	if err := pp.saveToTempProject(); err != nil {
		fmt.Printf("Failed to save new project to temp file: %v\n", err)
	}

	// Notify frontend of new project (only if context is available)
	if pp.app.ctx != nil {
		rt.EventsEmit(pp.app.ctx, "project:created", pp.app.currentProject.ID)
	}

	// Update window title
	pp.app.updateWindowTitle()

	fmt.Printf("Created new project with clean state\n")
	return nil
}

// LoadProject loads a project from disk, handling any existing unsaved work
func (pp *ProjectPersistence) LoadProject() error {
	// Handle transition from current project
	result := pp.handleUnsavedProjectTransition("opening a different project")
	switch result {
	case TransitionCancelled:
		return nil // User cancelled, operation aborted
	case TransitionError:
		return fmt.Errorf("failed to handle project transition")
	}

	// Store the current temp project path before changing it
	previousTempPath := pp.app.tempProjectPath
	defaultTempPath := filepath.Join(paths.GlobalPaths.AppDataDir, "temp_project.gleip")

	// Ensure the projects directory exists before showing the load dialog
	if err := paths.EnsureProjectsDir(); err != nil {
		return fmt.Errorf("failed to create projects directory: %v", err)
	}

	selectedFilePath, err := rt.OpenFileDialog(pp.app.ctx, rt.OpenDialogOptions{
		DefaultDirectory: paths.GlobalPaths.ProjectsDir,
		Title:            "Open Gleip Project",
		Filters:          []rt.FileFilter{{DisplayName: "Gleip Projects (*.gleip)", Pattern: "*.gleip;*.GLEIP"}},
	})

	if err != nil {
		rt.MessageDialog(pp.app.ctx, rt.MessageDialogOptions{
			Type:    rt.ErrorDialog,
			Title:   "Error",
			Message: "Failed to open file dialog: " + err.Error(),
		})
		return fmt.Errorf("open file dialog error: %v", err)
	}

	if selectedFilePath == "" {
		return nil // User cancelled
	}

	// Load the selected project
	project, err := pp.loadProjectFromPath(selectedFilePath)
	if err != nil {
		return err
	}

	// Set the loaded project as current
	pp.app.clearApplicationState()
	pp.app.projectMutex.Lock()
	pp.app.currentProject = project
	pp.app.projectMutex.Unlock()

	if errRestore := pp.app.restoreApplicationState(); errRestore != nil {
		return fmt.Errorf("failed to restore application state: %v", errRestore)
	}

	// Set the request counter based on loaded history
	maxSeqNum := 0
	pp.app.projectMutex.RLock()
	if pp.app.currentProject != nil && pp.app.currentProject.RequestHistory != nil {
		for _, tx := range pp.app.currentProject.RequestHistory {
			if tx != nil && tx.SeqNumber > maxSeqNum {
				maxSeqNum = tx.SeqNumber
			}
		}
	}
	pp.app.projectMutex.RUnlock()

	if pp.app.proxyServer != nil {
		pp.app.proxyServer.SetRequestCounter(maxSeqNum)
	}

	rt.EventsEmit(pp.app.ctx, "project:loaded", project.ID)

	// Update the auto-save target to the loaded project's file
	pp.app.tempProjectPath = project.FilePathOnDisk

	// Clean up the previous temporary project file if it was the default temp file
	// Only delete if it's different from the new target and it's the default temp file
	if previousTempPath != pp.app.tempProjectPath && previousTempPath == defaultTempPath {
		if err := os.Remove(previousTempPath); err != nil && !os.IsNotExist(err) {
			// Don't fail the load operation if we can't delete the temp file
			fmt.Printf("Warning: Failed to delete previous temp project file %s: %v\n", previousTempPath, err)
		} else if err == nil {
			fmt.Printf("Deleted previous temp project file: %s\n", previousTempPath)
		}
	}

	// Now that we've loaded a user's project file, save current state directly to that file
	// (not to temp file) to ensure we're immediately auto-saving to the user's chosen location
	pp.app.projectMutex.Lock()
	if err := pp.updateProjectFromCurrentStateData(pp.app.currentProject); err != nil {
		pp.app.projectMutex.Unlock()
		return fmt.Errorf("failed to update project data: %v", err)
	}

	// Create a project copy to save
	projectToSave := *pp.app.currentProject
	pp.app.projectMutex.Unlock()

	// Save to the loaded project's file to ensure it's up to date
	if err := pp.saveProjectToPath(&projectToSave, project.FilePathOnDisk); err != nil {
		return fmt.Errorf("failed to save loaded project: %v", err)
	}

	// Update window title
	pp.app.updateWindowTitle()

	fmt.Printf("Loaded project from: %s \n", project.FilePathOnDisk)
	return nil
}

// SaveProject saves the current project to its existing location or prompts for location if needed
func (pp *ProjectPersistence) SaveProject() error {
	pp.app.projectMutex.Lock()

	if pp.app.currentProject == nil {
		pp.app.projectMutex.Unlock()
		return fmt.Errorf("no active project to save")
	}

	// If the project doesn't have a destination path yet, call promptSaveProjectAs
	if pp.app.currentProject.FilePathOnDisk == "" || pp.app.currentProject.Name == defaultProjectName {
		pp.app.projectMutex.Unlock()
		return pp.promptSaveProjectAs()
	}

	// Project already has a destination, get it
	savePath := pp.app.currentProject.FilePathOnDisk

	// Update project data before saving
	if err := pp.updateProjectFromCurrentStateData(pp.app.currentProject); err != nil {
		pp.app.projectMutex.Unlock()
		return fmt.Errorf("failed to update project data: %v", err)
	}

	// Create a project copy to save
	projectToSave := *pp.app.currentProject
	pp.app.projectMutex.Unlock()

	// Save directly to the user's chosen file
	if err := pp.saveProjectToPath(&projectToSave, savePath); err != nil {
		fmt.Printf("Error saving project: %v\n", err)
		rt.MessageDialog(pp.app.ctx, rt.MessageDialogOptions{
			Type:    rt.ErrorDialog,
			Title:   "Save Error",
			Message: fmt.Sprintf("Failed to save project: %v", err),
		})
		return err
	}

	// Ensure auto-save target is pointing to the user's chosen file
	pp.app.tempProjectPath = savePath

	// Clean up temp file if we're no longer using it for auto-save
	defaultTempPath := filepath.Join(paths.GlobalPaths.AppDataDir, "temp_project.gleip")
	if savePath != defaultTempPath {
		if _, err := os.Stat(defaultTempPath); err == nil {
			if err := os.Remove(defaultTempPath); err != nil {
				fmt.Printf("Warning: Failed to clean up temp file %s: %v\n", defaultTempPath, err)
			} else {
				fmt.Printf("Cleaned up temporary project file since project is now saved to: %s\n", savePath)
			}
		}
	}

	fmt.Printf("Project saved to: %s\n", savePath)
	return nil
}

// SaveProjectAs provides public access to the save-as functionality
func (pp *ProjectPersistence) SaveProjectAs() error {
	return pp.promptSaveProjectAs()
}

// InitializeProjectOnStartup handles project initialization during app startup
// It checks for existing temporary projects and loads them, or creates a new project if none exists
func (pp *ProjectPersistence) InitializeProjectOnStartup() error {
	// Set up the default temp project path
	defaultTempPath := filepath.Join(paths.GlobalPaths.AppDataDir, "temp_project.gleip")
	pp.app.tempProjectPath = defaultTempPath

	// Check if there's an existing temp project to load
	if _, err := os.Stat(defaultTempPath); err == nil {
		// Try to load the temp project
		project, err := pp.loadProjectFromPath(defaultTempPath)
		if err != nil {
			fmt.Printf("Failed to load temp project, creating new one: %v\n", err)
			return pp.CreateNewProject()
		}

		// Check if the loaded project has meaningful data
		pp.app.projectMutex.Lock()
		pp.app.currentProject = project
		pp.app.projectMutex.Unlock()

		if !pp.hasProjectMeaningfulData() {
			// Temp project exists but has no meaningful data, create a fresh one
			fmt.Printf("Existing temp project has no meaningful data, creating fresh project\n")
			return pp.CreateNewProject()
		}

		// Project has meaningful data, restore it
		fmt.Printf("Loading existing temporary project with meaningful data\n")

		// Set the loaded project as current (already done above)
		pp.app.clearApplicationState()

		if errRestore := pp.app.restoreApplicationState(); errRestore != nil {
			fmt.Printf("Failed to restore application state, creating new project: %v\n", errRestore)
			return pp.CreateNewProject()
		}

		// Set the request counter based on loaded history
		maxSeqNum := 0
		pp.app.projectMutex.RLock()
		if pp.app.currentProject != nil && pp.app.currentProject.RequestHistory != nil {
			for _, tx := range pp.app.currentProject.RequestHistory {
				if tx != nil && tx.SeqNumber > maxSeqNum {
					maxSeqNum = tx.SeqNumber
				}
			}
		}
		pp.app.projectMutex.RUnlock()

		if pp.app.proxyServer != nil {
			pp.app.proxyServer.SetRequestCounter(maxSeqNum)
		}

		rt.EventsEmit(pp.app.ctx, "project:loaded", project.ID)

		// Update window title
		pp.app.updateWindowTitle()

		return nil
	}

	// No temp project found, create a new project
	fmt.Printf("No existing temporary project found, creating new project\n")
	return pp.CreateNewProject()
}

// ClearTemporaryProject removes the temporary project file and resets to clean state
func (pp *ProjectPersistence) ClearTemporaryProject() error {
	defaultTempPath := filepath.Join(paths.GlobalPaths.AppDataDir, "temp_project.gleip")

	// Remove the temporary project file if it exists
	if _, err := os.Stat(defaultTempPath); err == nil {
		if err := os.Remove(defaultTempPath); err != nil {
			return fmt.Errorf("failed to remove temporary project file: %v", err)
		}
		fmt.Printf("Cleared temporary project file\n")
	}

	return nil
}
