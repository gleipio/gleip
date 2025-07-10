<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from 'svelte/store';
  import { ClipboardGetText, EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime';
  import { StartFuzzing, SetSelectedGleipFlowID, GetSelectedGleipFlowID, DuplicateGleipFlow, RenameGleipFlow, AddStepToGleipFlowAtPosition, GetPhantomRequests, AddPhantomRequestToGleipFlow, UpdateGleipFlowVariables } from '../../../wailsjs/go/backend/App';
  import * as monaco from 'monaco-editor';
  import GleipStepCard from './components/GleipFlowStepCard.svelte';
  import AddStepButtons from './components/AddStepButtons.svelte';
  import ContextMenu from '../../shared/components/ContextMenu.svelte';
  import PhantomRequestsSection from './components/PhantomRequestsSection.svelte';
  import { GleipFlowExecutionService } from './services/GleipFlowExecutionService';
  import { 
    gleipFlows, 
    activeGleipFlowIndex, 
    activeGleipFlow,
    activeStepIndex, 
    isExecuting,
    loadGleipFlows,
    createGleipFlow,
    deleteGleipFlow,
    updateGleipFlow,
    addStep,
    deleteStep,
    updateStepSelection,
    updateStepExpansion,
    pasteRequestAtPosition,
    updateFuzzResults
  } from './store/gleipStore';
  import { getRequestFromClipboard, createRequestStepFromClipboard } from './utils/clipboardUtils';
  import type { GleipFlowStep, GleipFlow } from './types';

  // Focus action for accessibility
  const focusElement = (node: HTMLElement) => {
    node.focus();
  };

  // Focus and select all text for renaming
  const focusAndSelectAll = (node: HTMLInputElement) => {
    node.focus();
    node.select();
  };
 
  // State for paste notification
  let showPasteNotification = false;
  let pasteNotificationMessage = '';
  
  // Monaco editor state
  let monacoEditor: monaco.editor.IStandaloneCodeEditor | null = null;
  
  // Context menu state
  let showContextMenu = false;
  let contextMenuX = 0;
  let contextMenuY = 0;
  let contextMenuFlowId = '';
  let contextMenuFlowIndex = -1;
  
  // Rename state
  let isRenaming = false;
  let renamingFlowId = '';
  let renamingFlowName = '';
  
  // Listen for step execution updates from backend
  const handleStepExecuted = (event: any) => {
    console.log("Received step execution event:", event);
    const { gleipFlowId, currentStepIndex, results } = event;
    
    // Reload complete flow data to get updated action previews and execution results
    if ($activeGleipFlow && $activeGleipFlow.id === gleipFlowId) {
      console.log(`Reloading flow data to get updated action previews for step ${currentStepIndex}`);
      loadGleipFlows();
    }
  };

  // Listen for fuzz updates from backend
  const handleFuzzUpdate = (event: any) => {
    console.log("Received fuzz update event:", event);
    const { stepId, fuzzResults, isFuzzing } = event;
    
    if (!stepId) {
      console.warn("No stepId in fuzz update event");
      return;
    }
    
    // Update fuzz results in separate store to avoid triggering full gleip flow re-render
    updateFuzzResults(stepId, fuzzResults || []);
    
    console.log(`Updated fuzz results for step ${stepId} with ${fuzzResults?.length || 0} results (isFuzzing: ${isFuzzing})`);
  };
  
  // Load gleipFlows on component mount
  onMount(() => {
    console.log("GleipFlow component mounted");
    
    // Load gleipFlows immediately
    loadGleipFlows().then(async () => {
      // After loading, set the active gleip based on the saved selection
      const currentGleips = get(gleipFlows);
      if (currentGleips.length > 0) {
        try {
          // Get the selected GleipFlow ID from the backend
          const selectedGleipFlowID = await GetSelectedGleipFlowID();
          
          if (selectedGleipFlowID) {
            // Find the index of the selected GleipFlow
            const selectedIndex = currentGleips.findIndex(g => g.id === selectedGleipFlowID);
            if (selectedIndex !== -1) {
              // Set the saved selection as active
              activeGleipFlowIndex.set(selectedIndex);
              console.log(`Restored selected GleipFlow: ${selectedGleipFlowID} at index ${selectedIndex}`);
            } else {
              // If the selected GleipFlow doesn't exist, fall back to the first one
              console.warn(`Selected GleipFlow ${selectedGleipFlowID} not found, falling back to first`);
              const sortedGleips = [...currentGleips].sort((a, b) => a.sortingIndex - b.sortingIndex);
              const firstGleipIndex = currentGleips.findIndex(g => g.id === sortedGleips[0].id);
              activeGleipFlowIndex.set(firstGleipIndex);
              // Update the backend with the fallback selection
              await SetSelectedGleipFlowID(sortedGleips[0].id);
            }
          } else {
            // No selection saved, use the first one and save it
            const sortedGleips = [...currentGleips].sort((a, b) => a.sortingIndex - b.sortingIndex);
            const firstGleipIndex = currentGleips.findIndex(g => g.id === sortedGleips[0].id);
            activeGleipFlowIndex.set(firstGleipIndex);
            // Save this as the selected GleipFlow
            await SetSelectedGleipFlowID(sortedGleips[0].id);
            console.log(`No saved selection, set first GleipFlow as active: ${sortedGleips[0].id}`);
          }
        } catch (error) {
          console.error('Failed to get selected GleipFlow ID, falling back to first:', error);
          // Fall back to first gleip if there's an error
          const sortedGleips = [...currentGleips].sort((a, b) => a.sortingIndex - b.sortingIndex);
          const firstGleipIndex = currentGleips.findIndex(g => g.id === sortedGleips[0].id);
          activeGleipFlowIndex.set(firstGleipIndex);
        }
      }
    });
    
    // Initialize the gleip execution service
    GleipFlowExecutionService.init();
    
    // Set up keyboard event listener for paste
    window.addEventListener('keydown', handleKeyDown);
    
    // Setup event listeners for real-time updates
    console.log("Setting up gleipFlow event listeners");
    EventsOn('gleipFlow:stepExecuted', handleStepExecuted);
    EventsOn('gleipFlow:fuzzUpdate', handleFuzzUpdate);
    
    // Return cleanup function
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
      console.log("Removing gleipFlow event listeners");
      EventsOff('gleipFlow:stepExecuted');
      EventsOff('gleipFlow:fuzzUpdate');
    };
  });

  // Function to transform gleip data for UI display by adding a variables step
  function getUISteps(gleip: GleipFlow): GleipFlowStep[] {
    if (!gleip) return [];
    
    // Create a variables step from the gleip's variables
    const variablesStep: GleipFlowStep = {
      stepType: 'variables',
      selected: true,
      variablesStep: gleip.variables || {}
    };
    
    // Return the variables step followed by the actual steps
    return [variablesStep, ...gleip.steps];
  }

  // Handle keyboard events
  function handleKeyDown(e: KeyboardEvent) {
    // Check for Ctrl+V or Cmd+V (paste)
    if ((e.ctrlKey || e.metaKey) && e.key === 'v') {
      // Only handle if we're in the request gleip tab and not in an input/textarea
      const activeElement = document.activeElement;
      if (activeElement instanceof HTMLInputElement || activeElement instanceof HTMLTextAreaElement) {
        // Let the browser handle the paste in input fields
        return;
      }
      
      // Handle paste from clipboard
      handlePaste();
    }
    
    // Check for Ctrl+T or Cmd+T (new gleipflow)
    if ((e.ctrlKey || e.metaKey) && e.key === 't') {
      e.preventDefault(); // Prevent browser default behavior (new tab)
      
      // Only handle if we're not in an input/textarea
      const activeElement = document.activeElement;
      if (activeElement instanceof HTMLInputElement || activeElement instanceof HTMLTextAreaElement) {
        return;
      }
      
      // Create a new gleipflow
      createGleipFlow("");
    }
    
    // Check for Ctrl+W or Cmd+W (delete current gleipflow)
    if ((e.ctrlKey || e.metaKey) && e.key === 'w') {
      e.preventDefault(); // Prevent browser default behavior (close tab)
      
      // Only handle if we're not in an input/textarea
      const activeElement = document.activeElement;
      if (activeElement instanceof HTMLInputElement || activeElement instanceof HTMLTextAreaElement) {
        return;
      }
      
      // Delete current gleipflow if there is one
      if ($gleipFlows.length > 0 && $activeGleipFlowIndex !== null && $activeGleipFlowIndex < $gleipFlows.length) {
        const currentFlow = $gleipFlows[$activeGleipFlowIndex];
        deleteGleipFlow(currentFlow.id);
      }
    }
  }

  // Handle paste event
  async function handlePaste() {
    // Make sure we have an active gleip
    if ($activeGleipFlowIndex === null || $activeGleipFlowIndex >= $gleipFlows.length) {
      showNotification('No active gleip to paste into');
      return;
    }
    
    try {
      const { success, request, message } = await getRequestFromClipboard();
      
      if (success && request) {
        // Create a new request step from the clipboard data
        const newRequestStep = createRequestStepFromClipboard(request);
        const newStep = {
          requestStep: newRequestStep,
          stepType: 'request',
          selected: true
        };
        
        // Update the request name to include index
        if (newStep.requestStep && $activeGleipFlowIndex !== null) {
          const requestCount = $gleipFlows[$activeGleipFlowIndex].steps.filter(s => s.stepType === 'request').length + 2;
          newStep.requestStep.stepAttributes.name = `Request ${requestCount}`;
        }
        
        // Create a deep copy of the current gleip flow
        const currentGleipFlow = $gleipFlows[$activeGleipFlowIndex];
        const updatedGleipFlow = {
          ...currentGleipFlow, 
          steps: [...currentGleipFlow.steps, newStep]
        };
        
        // Update the store
        gleipFlows.set([
          ...$gleipFlows.slice(0, $activeGleipFlowIndex),
          updatedGleipFlow,
          ...$gleipFlows.slice($activeGleipFlowIndex + 1)
        ]);
        
        // Save the gleip
        updateGleipFlow(updatedGleipFlow).then(() => {
          // Set the new step as active
          activeStepIndex.set(updatedGleipFlow.steps.length - 1);
          
          // Show success notification
          showNotification('Request pasted successfully');
        });
      } else {
        showNotification(message);
      }
    } catch (e) {
      showNotification('Failed to process clipboard content');
      console.error(e);
    }
  }

  // Show a notification
  function showNotification(message: string) {
    pasteNotificationMessage = message;
    showPasteNotification = true;
    
    // Hide after 3 seconds
    setTimeout(() => {
      showPasteNotification = false;
    }, 3000);
  }

  // Toggle step expansion
  async function handleToggleExpand(event: CustomEvent) {
    const { stepIndex } = event.detail;
    
    // Handle variables step (index 0) - use special index -1 for backend
    if (stepIndex === 0) {
      if ($activeGleipFlowIndex === null) return;
      const currentFlow = $gleipFlows[$activeGleipFlowIndex];
      if (!currentFlow) return;
      
      const currentExpanded = currentFlow.isVariableStepExpanded || false;
      await updateStepExpansion(-1, !currentExpanded);
      return;
    }
    
    // Adjust index to account for the fake variables step
    const actualIndex = stepIndex - 1;
    
    // Get current expansion state from the step data
    if ($activeGleipFlowIndex === null) return;
    const currentFlow = $gleipFlows[$activeGleipFlowIndex];
    if (!currentFlow || actualIndex >= currentFlow.steps.length) return;
    
    const step = currentFlow.steps[actualIndex];
    let currentExpanded = false;
    
    // Get current expanded state based on step type
    if (step.stepType === 'request' && step.requestStep) {
      currentExpanded = step.requestStep.stepAttributes.isExpanded;
    } else if (step.stepType === 'script' && step.scriptStep) {
      currentExpanded = step.scriptStep.stepAttributes.isExpanded;
    } else if (step.stepType === 'chef' && step.chefStep) {
      currentExpanded = step.chefStep.stepAttributes.isExpanded;
    }
    
    // Toggle the expansion state
    await updateStepExpansion(actualIndex, !currentExpanded);
    
    // Set the step as active when expanding
    if (!currentExpanded) {
      activeStepIndex.set(actualIndex);
    }
  }

  // Helper function to get expansion state of a step
  function getStepExpansionState(step: GleipFlowStep, index: number): boolean {
    // Variables step (index 0) uses the flow's isVariableStepExpanded field
    if (index === 0 && step.stepType === 'variables') {
      if ($activeGleipFlowIndex !== null && $gleipFlows[$activeGleipFlowIndex]) {
        return $gleipFlows[$activeGleipFlowIndex].isVariableStepExpanded || false;
      }
      return false;
    }
    
    // For other steps, check the stepAttributes
    if (step.stepType === 'request' && step.requestStep) {
      return step.requestStep.stepAttributes.isExpanded;
    } else if (step.stepType === 'script' && step.scriptStep) {
      return step.scriptStep.stepAttributes.isExpanded;
    } else if (step.stepType === 'chef' && step.chefStep) {
      return step.chefStep.stepAttributes.isExpanded;
    }
    
    return false;
  }

  // Update step selection
  async function handleUpdateSelection(event: CustomEvent) {
    const { stepIndex, selected } = event.detail;
    
    // Skip the variables step (index 0)
    if (stepIndex === 0) return;
    
    // Adjust index to account for the fake variables step
    const actualIndex = stepIndex - 1;
    
    await updateStepSelection(actualIndex, selected);
  }

  // Update step data
  function handleUpdateStep(event: CustomEvent) {
    const { stepIndex, stepType, updates } = event.detail;
    
    if (stepIndex === 0 && stepType === 'variables') {
      // Handle variables step specially - use the new backend method that auto-executes chef steps
      if (updates.variables !== undefined && $activeGleipFlowIndex !== null) {
        const currentGleipFlow = $gleipFlows[$activeGleipFlowIndex];
        
        // Call the new backend method that updates variables and executes enabled chef steps
        // The step execution events will automatically trigger data reload to get updated action previews
        UpdateGleipFlowVariables(currentGleipFlow.id, updates.variables)
          .then(() => {
            // Reload the gleipFlows from backend to refresh UI with updated variables
            loadGleipFlows();
          })
          .catch((error: any) => {
            console.error('Failed to update variables:', error);
            showNotification(`Error updating variables: ${error.message || 'Unknown error'}`);
          });
      }
      return;
    }
    
    // Adjust index for regular steps
    const actualIndex = stepIndex - 1;
    
    if (actualIndex >= 0 && $activeGleipFlowIndex !== null && $gleipFlows[$activeGleipFlowIndex] && actualIndex < $gleipFlows[$activeGleipFlowIndex].steps.length) {
      const currentGleipFlow = $gleipFlows[$activeGleipFlowIndex];
      const updatedSteps = [...currentGleipFlow.steps];
      const step = {...updatedSteps[actualIndex]};
      
      if (stepType === 'request' && step.requestStep) {
        step.requestStep = {
          ...step.requestStep,
          ...updates
        };
      } else if (stepType === 'script' && step.scriptStep) {
        step.scriptStep = {
          ...step.scriptStep,
          ...updates
        };
      } else if (stepType === 'chef' && step.chefStep) {
        step.chefStep = {
          ...step.chefStep,
          ...updates
        };
      }
      
      updatedSteps[actualIndex] = step;
      
      const updatedGleipFlow = { 
        ...currentGleipFlow, 
        steps: updatedSteps 
      };
      
      gleipFlows.set([
        ...$gleipFlows.slice(0, $activeGleipFlowIndex),
        updatedGleipFlow,
        ...$gleipFlows.slice($activeGleipFlowIndex + 1)
      ]);
      
      updateGleipFlow(updatedGleipFlow);
    }
  }

  // Execute a single step
  function handleExecuteStep(event: CustomEvent) {
    console.log("🚨 HANDLE EXECUTE STEP CALLED", event.detail);
    alert("🚨 HANDLE EXECUTE STEP CALLED with stepIndex: " + event.detail.stepIndex);
    
    const { stepIndex } = event.detail;
    
    // Skip execution of the variables step
    if (stepIndex === 0) return;
    
    // Adjust index to account for the fake variables step
    const actualIndex = stepIndex - 1;
    console.log("🚨 CALLING executeSingleStep with actualIndex:", actualIndex);
    GleipFlowExecutionService.executeSingleStep(actualIndex);
  }

  // Get execution result for a step
  function getStepExecutionResult(step: GleipFlowStep) {
    if (!step || !$activeGleipFlow) return undefined;
    
    const stepId = step.stepType === 'request' ? step.requestStep?.stepAttributes.id : 
                 step.stepType === 'script' ? step.scriptStep?.stepAttributes.id : 
                 step.stepType === 'chef' ? step.chefStep?.stepAttributes.id :
                 $activeGleipFlow.id + '-variables';
    
    if (stepId && $activeGleipFlow.executionResults) {
      const result = $activeGleipFlow.executionResults.find(r => r.stepId === stepId);
      return result;
    }
    
    return undefined;
  }

  // Handle editor mount
  function handleEditorMount(event: CustomEvent) {
    const { editor } = event.detail;
    monacoEditor = editor;
  }

  // Execute the gleip
  function executeGleipFlow() {
    if (!$activeGleipFlow) return;
    
    // Clear any existing notification
    if (showPasteNotification) {
      showPasteNotification = false;
    }
    
    GleipFlowExecutionService.executeGleipFlow($activeGleipFlow.id)
      .then(success => {
        if (!success) {
          showNotification('Failed to execute GleipFlow');
        }
      })
      .catch(error => {
        console.error('Error executing GleipFlow:', error);
        showNotification(`Error: ${error.message || 'Failed to execute GleipFlow'}`);
      });
  }

  // Handle delete step event from UI
  async function handleDeleteStep(event: CustomEvent) {
    const { stepIndex } = event.detail;
    
    // Can't delete the variables step
    if (stepIndex === 0) return;
    
    // Adjust index to account for the fake variables step
    const actualIndex = stepIndex - 1;
    await deleteStep(actualIndex);
  }

  // Handle starting the fuzzing process
  async function handleStartFuzzing(event: CustomEvent) {
    const { stepIndex } = event.detail;
    
    // Skip the variables step (index 0)
    if (stepIndex === 0) return;
    
    // Adjust index to account for the fake variables step
    const actualIndex = stepIndex - 1;
    
    if (actualIndex !== undefined && $activeGleipFlowIndex !== null && $gleipFlows[$activeGleipFlowIndex] && actualIndex < $gleipFlows[$activeGleipFlowIndex].steps.length) {
      const currentGleipFlow = $gleipFlows[$activeGleipFlowIndex];
      const step = currentGleipFlow.steps[actualIndex];
      
      if (step.stepType === 'request' && step.requestStep) {
        try {
          isExecuting.set(true);
          
          // Ensure fuzzResults is initialized before starting
          if (step.requestStep.fuzzSettings && (!step.requestStep.fuzzSettings.fuzzResults || !Array.isArray(step.requestStep.fuzzSettings.fuzzResults))) {
            console.log("Initializing empty fuzzResults array before starting");
            const updatedStep = {
              ...step,
              requestStep: {
                ...step.requestStep,
                fuzzSettings: {
                  ...step.requestStep.fuzzSettings,
                  fuzzResults: []
                }
              }
            };
            
            // Update the gleipFlow with the updated step
            const updatedGleipFlow = {
              ...currentGleipFlow,
              steps: [
                ...currentGleipFlow.steps.slice(0, actualIndex),
                updatedStep,
                ...currentGleipFlow.steps.slice(actualIndex + 1)
              ]
            };
            
            // Update the store
            gleipFlows.set([
              ...$gleipFlows.slice(0, $activeGleipFlowIndex),
              updatedGleipFlow,
              ...$gleipFlows.slice($activeGleipFlowIndex + 1)
            ]);
            
            // Save the updated flow
            await updateGleipFlow(updatedGleipFlow);
          }
          
          console.log("Starting fuzzing for step ID:", step.requestStep.stepAttributes.id);
          
          // Call the backend to start fuzzing
          await StartFuzzing(currentGleipFlow.id, step.requestStep.stepAttributes.id);
          
          // Show success notification
          showNotification('Fuzzing completed successfully');
        } catch (error) {
          console.error('Failed to start fuzzing:', error);
          showNotification(`Failed to start fuzzing: ${error}`);
        } finally {
          isExecuting.set(false);
          
          // Don't reload the gleipFlows after fuzzing as this may overwrite the fuzz results
          // that were updated through real-time events
          // await loadGleipFlows();
        }
      }
    }
  }

  // Handle mode change in request step
  function handleRequestModeChange(event: CustomEvent) {
    const { stepIndex, mode } = event.detail;
    
    // Skip the variables step (index 0)
    if (stepIndex === 0) return;
    
    // Adjust index to account for the fake variables step
    const actualIndex = stepIndex - 1;
    
    if (actualIndex !== undefined && $activeGleipFlowIndex !== null && $gleipFlows[$activeGleipFlowIndex] && actualIndex < $gleipFlows[$activeGleipFlowIndex].steps.length) {
      const currentGleipFlow = $gleipFlows[$activeGleipFlowIndex];
      const step = currentGleipFlow.steps[actualIndex];
      
      if (step.stepType === 'request' && step.requestStep) {
        // Handle mode change here if needed
        console.log(`Mode changed to ${mode} for step ${stepIndex}`);
      }
    }
  }

  // Handle GleipFlow tab click
  async function handleGleipFlowTabClick(gleipFlowId: string, gleipFlowIndex: number) {
    try {
      // Update the backend with the selected GleipFlow ID
      await SetSelectedGleipFlowID(gleipFlowId);
      
      // Update the frontend store
      activeGleipFlowIndex.set(gleipFlowIndex);
      
      console.log(`Selected GleipFlow: ${gleipFlowId} at index ${gleipFlowIndex}`);
    } catch (error) {
      console.error('Failed to set selected GleipFlow ID:', error);
      // Still update the frontend store even if backend call fails
      activeGleipFlowIndex.set(gleipFlowIndex);
    }
  }

  // Handle right-click on flow tab to show context menu
  function handleFlowRightClick(event: MouseEvent, gleipFlowId: string, gleipFlowIndex: number) {
    event.preventDefault();
    event.stopPropagation();
    
    contextMenuX = event.clientX;
    contextMenuY = event.clientY;
    contextMenuFlowId = gleipFlowId;
    contextMenuFlowIndex = gleipFlowIndex;
    showContextMenu = true;
  }

  // Close context menu
  function closeContextMenu() {
    showContextMenu = false;
    contextMenuFlowId = '';
    contextMenuFlowIndex = -1;
  }

  // Start renaming a flow
  function startRename() {
    const flow = $gleipFlows.find(f => f.id === contextMenuFlowId);
    if (flow) {
      isRenaming = true;
      renamingFlowId = contextMenuFlowId;
      renamingFlowName = flow.name;
    }
    closeContextMenu();
  }

  // Finish renaming a flow
  async function finishRename() {
    if (renamingFlowName.trim() === '') {
      cancelRename();
      return;
    }

    try {
      await RenameGleipFlow(renamingFlowId, renamingFlowName.trim());
      
      // Update the local store
      const flowIndex = $gleipFlows.findIndex(f => f.id === renamingFlowId);
      if (flowIndex !== -1) {
        const updatedFlow = { ...$gleipFlows[flowIndex], name: renamingFlowName.trim() };
        gleipFlows.set([
          ...$gleipFlows.slice(0, flowIndex),
          updatedFlow,
          ...$gleipFlows.slice(flowIndex + 1)
        ]);
      }
      
      showNotification('Flow renamed successfully');
    } catch (error) {
      console.error('Failed to rename flow:', error);
      showNotification(`Failed to rename flow: ${error}`);
    }
    
    cancelRename();
  }

  // Cancel renaming
  function cancelRename() {
    isRenaming = false;
    renamingFlowId = '';
    renamingFlowName = '';
  }

  // Duplicate a flow
  async function duplicateFlow() {
    try {
      const duplicatedFlow = await DuplicateGleipFlow(contextMenuFlowId);
      
      // Reload flows to get the updated list
      await loadGleipFlows();
      
      // Set the duplicated flow as active
      const newFlowIndex = $gleipFlows.findIndex(f => f.id === duplicatedFlow.id);
      if (newFlowIndex !== -1) {
        activeGleipFlowIndex.set(newFlowIndex);
        await SetSelectedGleipFlowID(duplicatedFlow.id);
      }
      
      showNotification('Flow duplicated successfully');
    } catch (error) {
      console.error('Failed to duplicate flow:', error);
      showNotification(`Failed to duplicate flow: ${error}`);
    }
    
    closeContextMenu();
  }

  // Insert a step at a specific position
  async function insertStepAt(position: number, stepType: 'request' | 'chef' | 'script') {
    if ($activeGleipFlowIndex === null || !$activeGleipFlow) return;
    
    const currentGleipFlow = $gleipFlows[$activeGleipFlowIndex];
    
    try {
      // Use the new backend function to create a step at the specified position
      const newStep = await AddStepToGleipFlowAtPosition(currentGleipFlow.id, stepType, position);
      console.log('Backend returned new step at position:', newStep);
      
      // Reload flows from backend to get the updated state
      await loadGleipFlows();
      
      // Set the new step as active
      activeStepIndex.set(position);
      
      return newStep;
    } catch (error) {
      console.error('Failed to insert step:', error);
      showNotification(`Failed to add step: ${error}`);
      return null;
    }
  }

  // Paste a request at a specific position
  async function pasteRequestAt(position: number) {
    try {
      const result = await pasteRequestAtPosition(position);
      
      if (result.success) {
        showNotification(result.message);
      } else {
        showNotification(result.message);
      }
    } catch (error) {
      console.error('Failed to paste request:', error);
      showNotification(`Failed to paste request: ${error}`);
    }
  }

  // Get the last request in the current flow for phantom request generation
  function getLastRequestInFlow() {
    if (!$activeGleipFlow || $activeGleipFlow.steps.length === 0) {
      return null;
    }
    
    // Find the last request step
    for (let i = $activeGleipFlow.steps.length - 1; i >= 0; i--) {
      const step = $activeGleipFlow.steps[i];
      if (step.stepType === 'request' && step.requestStep) {
        return step.requestStep;
      }
    }
    
    return null;
  }

  // Check if the flow has any request steps
  function hasRequestSteps() {
    if (!$activeGleipFlow || $activeGleipFlow.steps.length === 0) {
      return false;
    }
    
    return $activeGleipFlow.steps.some(step => step.stepType === 'request' && step.requestStep);
  }

  // Handle adding a phantom request to the flow
  async function handleAddPhantomRequest(event: CustomEvent) {
    const { phantomRequest } = event.detail;
    
    if (!$activeGleipFlow) {
      showNotification('No active flow to add request to');
      return;
    }
    
    try {
      // Call backend to add the phantom request
      await AddPhantomRequestToGleipFlow($activeGleipFlow.id, phantomRequest);
      
      // Reload flows to get the updated state
      await loadGleipFlows();
      
      showNotification('Request added successfully');
    } catch (error) {
      console.error('Failed to add phantom request:', error);
      showNotification(`Failed to add request: ${error}`);
    }
  }
</script>

<div class="flex flex-col h-full">
  <div class="h-14 border-b border-gray-700/50 bg-gray-800/30 px-4 flex items-center">
    <div class="flex space-x-2">
      <!-- GleipFlow tabs -->
      <div class="flex">
        {#each [...$gleipFlows].sort((a, b) => a.sortingIndex - b.sortingIndex) as gleip, index}
          <button 
            class={`px-3 py-1 text-sm rounded-t relative ${$gleipFlows.findIndex(g => g.id === gleip.id) === $activeGleipFlowIndex ? 'bg-[var(--color-midnight-accent)]/30 text-white' : 'bg-gray-800 text-gray-50 hover:bg-gray-700/50'}`}
            on:click={() => handleGleipFlowTabClick(gleip.id, $gleipFlows.findIndex(g => g.id === gleip.id))}
            on:contextmenu={(e) => handleFlowRightClick(e, gleip.id, $gleipFlows.findIndex(g => g.id === gleip.id))}
          >
            {#if isRenaming && renamingFlowId === gleip.id}
              <input
                type="text"
                class="bg-transparent border border-gray-500 rounded px-1 text-white text-sm focus:outline-none focus:border-blue-400 w-24"
                bind:value={renamingFlowName}
                on:keydown={(e) => {
                  if (e.key === 'Enter') finishRename();
                  if (e.key === 'Escape') cancelRename();
                }}
                on:blur={finishRename}
                use:focusAndSelectAll
              />
            {:else}
              {gleip.name}
              <span 
                class="ml-2 text-gray-500 hover:text-gray-100 cursor-pointer"
                tabindex="0"
                role="button"
                on:click={(e) => {
                  e.stopPropagation();
                  deleteGleipFlow(gleip.id);
                }}
                on:keydown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    e.stopPropagation();
                    deleteGleipFlow(gleip.id);
                  }
                }}
              >
                ×
              </span>
            {/if}
          </button>
        {/each}
        
        <!-- Add new gleip button -->
        <button 
          class="px-3 py-1 text-sm bg-gray-800 text-gray-50 hover:bg-gray-700 hover:text-gray-50 rounded-t"
          on:click={() => {
            // Let the backend handle naming automatically
            createGleipFlow("");
          }}
        >
          +
        </button>
      </div>
    </div>
  </div>
  
  <!-- Main content area -->
  <div class="flex-1 flex flex-col min-h-0 p-3 pb-8">
    <!-- Action buttons -->
    <div class="flex justify-between mb-2">
      <div class="flex space-x-2">
        <button 
          class="px-3 py-1 bg-[var(--color-midnight-accent)] hover:bg-[var(--color-midnight-accent)]/80 text-[var(--color-button-text)] rounded text-sm"
          on:click={() => addStep('request')}
        >
          Add Request
        </button>
        <button 
          class="px-3 py-1 bg-[var(--color-midnight-accent)] hover:bg-[var(--color-midnight-accent)]/80 text-[var(--color-button-text)] rounded text-sm"
          on:click={() => addStep('chef')}
        >
          Add Chef
        </button>
        <button 
          class="px-3 py-1 bg-[var(--color-midnight-accent)] hover:bg-[var(--color-midnight-accent)]/80 text-[var(--color-button-text)] rounded text-sm"
          on:click={handlePaste}
        >
          Paste Request
        </button>
      </div>
      
      {#if $gleipFlows.length > 0 && $activeGleipFlowIndex !== null && $activeGleipFlowIndex < $gleipFlows.length}
        <button 
          class="px-3 py-1 bg-[var(--color-secondary-accent)] hover:bg-opacity-90 text-[var(--color-button-text)] rounded text-sm"
          on:click={executeGleipFlow}
          disabled={$isExecuting || ($activeGleipFlowIndex !== null && $gleipFlows[$activeGleipFlowIndex].steps.length === 0)}
        >
          {$isExecuting ? 'Executing...' : 'Execute GleipFlow'}
        </button>
      {/if}
    </div>
    
    <!-- Scrollable container for cards -->
    <div class="flex-1 overflow-x-scroll overflow-y-hidden scrollbar-visible relative">
      {#if $gleipFlows.length > 0 && $activeGleipFlowIndex !== null && $activeGleipFlowIndex < $gleipFlows.length}
        <div class="flex flex-row space-x-3 py-1 h-full">
          {#each getUISteps($gleipFlows[$activeGleipFlowIndex]) as step, index}
            <!-- Step card with relative positioning for plus button placement -->
            <div class="flex-shrink-0 relative">
              <GleipStepCard
                {step}
                stepIndex={index}
                isExpanded={getStepExpansionState(step, index)}
                executionResult={getStepExecutionResult(step)}
                isExecuting={$isExecuting}
                gleipFlowID={$gleipFlows[$activeGleipFlowIndex].id}
                on:toggleExpand={handleToggleExpand}
                on:deleteStep={handleDeleteStep}
                on:updateSelection={handleUpdateSelection}
                on:updateStep={handleUpdateStep}
                on:executeStep={handleExecuteStep}
                on:editorMount={handleEditorMount}
                on:requestModeChange={handleRequestModeChange}
                on:startFuzzing={handleStartFuzzing}
              />
              
              <!-- Add button after this step -->
              <AddStepButtons 
                position="right" 
                stepPosition={index} 
                on:addStep={(e) => insertStepAt(e.detail.position, e.detail.stepType)}
                on:paste={(e) => pasteRequestAt(e.detail.position)}
              />
            </div>
          {/each}
          
          <!-- Phantom requests section - only show if flow has at least one request -->
          {#if hasRequestSteps()}
            <PhantomRequestsSection
              gleipFlowId={$gleipFlows[$activeGleipFlowIndex].id}
              lastRequestInFlow={getLastRequestInFlow()}
              cachedPhantomRequests={$gleipFlows[$activeGleipFlowIndex].cachedPhantomRequests || []}
              on:addPhantomRequest={handleAddPhantomRequest}
            />
          {/if}
        </div>
      {:else}
        <div class="flex items-center justify-center h-64 text-gray-500">
          {#if $gleipFlows.length === 0}
            <div class="text-center">
              <p>No gleipFlows available</p>
              <p class="text-sm mt-2">Create a new gleip to get started</p>
            </div>
          {:else}
            <div class="text-center">
              <p>No steps in this gleip</p>
              <p class="text-sm mt-2">Add a request or script step to get started</p>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>
  
  <!-- Paste notification -->
  {#if showPasteNotification}
    <div class="fixed right-4 bottom-4 bg-[var(--color-secondary-accent)]/90 text-white px-4 py-3 rounded shadow-lg z-50 transition-opacity duration-300">
      <div class="flex items-center">
        <span class="mr-1">
          {pasteNotificationMessage.startsWith('Failed') || pasteNotificationMessage.startsWith('Invalid') 
            ? '❌' 
            : '✅'}
        </span>
        <span>{pasteNotificationMessage}</span>
      </div>
    </div>
  {/if}

  <!-- Context Menu -->
  {#if showContextMenu}
    <ContextMenu
      x={contextMenuX}
      y={contextMenuY}
      onClose={closeContextMenu}
      items={[
        { label: 'Rename', onClick: startRename },
        { label: 'Duplicate', onClick: duplicateFlow }
      ]}
    />
  {/if}
</div>

<style>
  /* Custom scrollbar styling for horizontal scroll */
  .scrollbar-visible {
    scrollbar-width: thin; /* Firefox - make it thinner */
    scrollbar-color: #9ca3af #1f2937; /* Firefox: lighter thumb, darker track */
    /* Force scrollbar to always be visible */
    overflow-x: scroll !important;
  }

  /* Force scrollbar to always be visible */
  .scrollbar-visible::-webkit-scrollbar {
    height: 12px; /* Increased height for better visibility */
    -webkit-appearance: none;
    display: block !important; /* Ensure it's always displayed */
  }

  .scrollbar-visible::-webkit-scrollbar-track {
    background: #1f2937; /* darker gray-800 */
    border-radius: 8px;
    border: 1px solid #374151;
  }

  .scrollbar-visible::-webkit-scrollbar-thumb {
    background: #9ca3af; /* lighter gray-400 */
    border-radius: 8px;
    border: 2px solid #1f2937;
    min-width: 30px; /* Ensure minimum thumb size */
  }

  .scrollbar-visible::-webkit-scrollbar-thumb:hover {
    background: #d1d5db; /* even lighter gray-300 on hover */
  }

  .scrollbar-visible::-webkit-scrollbar-corner {
    background: #1f2937;
  }

  /* HTTP syntax highlighting styles */
  :global(.http-request-method) {
    color: #22c55e; /* green-500 */
    font-weight: bold;
  }
  
  :global(.http-request-path) {
    color: #93c5fd; /* blue-300 */
  }
  
  :global(.http-request-version) {
    color: #9ca3af; /* gray-400 */
  }
  
  :global(.http-header-name) {
    color: #c4b5fd; /* purple-300 */
    font-weight: 600;
  }
  
  :global(.http-header-value) {
    color: #d1d5db; /* gray-300 */
  }
  
  :global(.http-body) {
    color: #fde68a; /* amber-200 */
  }
  
  :global(.http-method) {
    color: #3b82f6; /* blue-500 */
    font-weight: bold;
  }
  
  :global(.http-path) {
    color: #10b981; /* emerald-500 */
  }
  
  :global(.http-version) {
    color: #6b7280; /* gray-500 */
  }
  
  :global(.http-header-key) {
    color: #a855f7; /* purple-500 */
    font-weight: 600;
  }
  
  :global(.http-header-value) {
    color: #f59e0b; /* amber-500 */
  }
  
  /* Add a style for the body content */
  :global(.http-body) {
    color: #e5e7eb; /* gray-200 */
  }
</style>