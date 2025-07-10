import { writable, derived, get } from 'svelte/store';
import { SaveGleipFlow, GetGleipFlows, DeleteGleipFlow, CreateGleipFlow, AddStepToGleipFlow, SetSelectedGleipFlowID, UpdateGleipFlow, AddChefAction, RemoveChefAction, UpdateChefAction, UpdateChefStep, PasteRequestToGleipFlowAtPosition, UpdateStepExpansion } from '../../../../wailsjs/go/backend/App';
import { backend } from '../../../../wailsjs/go/models';
import type { GleipFlow, ExecutionResult, GleipFlowStep, FuzzResult } from '../types';
import { generateRawHttpRequest } from '../utils/httpUtils';

// Core stores
export const gleipFlows = writable<GleipFlow[]>([]);
export const activeGleipFlowIndex = writable<number | null>(null);
export const activeStepIndex = writable<number | null>(null);
export const isExecuting = writable<boolean>(false);
export const isCreatingGleipFlow = writable<boolean>(false);

// Separate store for fuzz results to avoid triggering full gleip flow re-renders
export const fuzzResults = writable<{ [stepId: string]: FuzzResult[] }>({});

// Store state
export const newGleipFlowName = writable<string>('');

export const updatedRequestIds = writable<Set<string>>(new Set());

// Function to update fuzz results without triggering full gleip flow re-render
export const updateFuzzResults = (stepId: string, results: FuzzResult[]) => {
  fuzzResults.update(current => ({
    ...current,
    [stepId]: results
  }));
};

// Derived stores
export const activeGleipFlow = derived(
  [gleipFlows, activeGleipFlowIndex], 
  ([$gleipFlows, $activeGleipFlowIndex]) => {
    if ($gleipFlows.length === 0 || $activeGleipFlowIndex === null || $activeGleipFlowIndex >= $gleipFlows.length) return null;
    return $gleipFlows[$activeGleipFlowIndex];
  }
);

export const activeStep = derived(
  [activeGleipFlow, activeStepIndex], 
  ([$activeGleipFlow, $activeStepIndex]) => {
    if (!$activeGleipFlow || $activeStepIndex === null || $activeStepIndex >= $activeGleipFlow.steps.length) return null;
    return $activeGleipFlow.steps[$activeStepIndex];
  }
);

// Store actions
export const loadGleipFlows = async () => {
  try {
    const loadedGleipFlows = await GetGleipFlows();
    console.log("Loaded GleipFlows from backend:", loadedGleipFlows);
    
    // Handle case where backend returns null or undefined
    if (!loadedGleipFlows || !Array.isArray(loadedGleipFlows)) {
      console.log("No GleipFlows loaded or invalid data format");
      gleipFlows.set([]);
      return;
    }
    
    // Convert from backend model to our local type
    const convertedGleipFlows: GleipFlow[] = loadedGleipFlows.map(gleipFlow => {
      // Extract variables from the first step if it's a variables step
      let variables: Record<string, string> = gleipFlow.variables || {};
      let gleipSteps: any[] = gleipFlow.steps ? [...gleipFlow.steps] : [];
      
      console.log(`Loading flow ${gleipFlow.id} with ${gleipFlow.executionResults?.length || 0} execution results`);
      
      return {
        id: gleipFlow.id,
        name: gleipFlow.name,
        variables: variables,
        sortingIndex: gleipFlow.sortingIndex || 0,
        executionResults: gleipFlow.executionResults || [], // Include execution results from backend
        isVariableStepExpanded: gleipFlow.isVariableStepExpanded || false, // Load variables step expansion state
        steps: gleipSteps.map(step => {
          let convertedStep: GleipFlowStep = {
            stepType: step.stepType,
            selected: step.selected
          };
          
          if (step.requestStep) {
            // Preserve the original step ID or generate a new one if missing
            const stepId = step.requestStep.stepAttributes.id;
            if (stepId === undefined) {
              console.error('Request Step ID is undefined');
            }
            
            convertedStep.requestStep = {
              stepAttributes: {
                id: stepId,
                name: step.requestStep.stepAttributes.name,
                isExpanded: step.requestStep.stepAttributes.isExpanded // Load expansion state from backend
              },
              request: step.requestStep.request,
              response: step.requestStep.response || { dump: '' },
              variableExtracts: step.requestStep.variableExtracts || [],
              recalculateContentLength: step.requestStep.recalculateContentLength || true,
              gunzipResponse: step.requestStep.gunzipResponse || true,
              isFuzzMode: step.requestStep.isFuzzMode || false,
              fuzzSettings: step.requestStep.fuzzSettings,
              cameFrom: step.requestStep.cameFrom || "user" // Default to "user" if not specified
            };
            
            // Ensure the request has a valid dump if missing
            ensureValidRawRequest(convertedStep);
          }
          
          if (step.scriptStep) {
            // Preserve the original step ID or generate a new one if missing
            const stepId = step.scriptStep.stepAttributes.id || undefined;
            if (stepId === undefined) {
              console.error('Script Step ID is undefined');
            }

            convertedStep.scriptStep = {
              stepAttributes: {
                id: stepId,
                name: step.scriptStep.stepAttributes.name,
                isExpanded: step.scriptStep.stepAttributes.isExpanded // Load expansion state from backend
              },
              content: step.scriptStep.content
            };
          }
          
          if (step.chefStep) {
            // Preserve the original step ID or generate a new one if missing
            const stepId = step.chefStep.stepAttributes.id || undefined;
            if (stepId === undefined) {
              console.error('Chef Step ID is undefined');
            }
            
            convertedStep.chefStep = {
              stepAttributes: {
                id: stepId,
                name: step.chefStep.stepAttributes.name,
                isExpanded: step.chefStep.stepAttributes.isExpanded // Load expansion state from backend
              },
              inputVariable: step.chefStep.inputVariable,
              actions: step.chefStep.actions || [],
              outputVariable: step.chefStep.outputVariable
            };
          }
          
          return convertedStep;
        })
      };
    });
    
    // Sort gleips by sortingIndex
    convertedGleipFlows.sort((a, b) => a.sortingIndex - b.sortingIndex);
    
    // Recalculate sorting indices to ensure they are sequential from 1 to n
    recalculateSortingIndices(convertedGleipFlows);
    
    // Save the recalculated indices back to the backend
    for (const gleipFlow of convertedGleipFlows) {
      await saveGleipFlow(gleipFlow);
    }
    
    gleipFlows.set(convertedGleipFlows);
    
    const currentActiveIndex = get(activeGleipFlowIndex);
    if (convertedGleipFlows.length > 0 && currentActiveIndex !== null && currentActiveIndex >= convertedGleipFlows.length) {
      activeGleipFlowIndex.set(0);
    }
    
    // Counter logic is now handled by the backend
    
  } catch (error) {
    console.error('Failed to load gleips:', error);
  }
};

export const createGleipFlow = async (name?: string) => {
  try {
    const savedGleipFlow = await CreateGleipFlow(name || '');
    
    // Update the store with the newly created flow from backend
    gleipFlows.update($gleipFlows => [...$gleipFlows, savedGleipFlow as GleipFlow]);
    
    isCreatingGleipFlow.set(false);
    newGleipFlowName.set('');
    
    // Set the newly created gleip as active
    const updatedGleips = get(gleipFlows);
    const newGleipIndex = updatedGleips.findIndex(g => g.id === savedGleipFlow.id);
    if (newGleipIndex !== -1) {
      activeGleipFlowIndex.set(newGleipIndex);
    } else {
      activeGleipFlowIndex.set(updatedGleips.length - 1);
    }
    
    // Update the backend with the selected GleipFlow ID
    try {
      await SetSelectedGleipFlowID(savedGleipFlow.id);
      console.log(`Set newly created GleipFlow as selected: ${savedGleipFlow.id}`);
    } catch (error) {
      console.error('Failed to set selected GleipFlow ID for new flow:', error);
    }
    
    return savedGleipFlow.id;
  } catch (error) {
    console.error('Failed to create gleip:', error);
    return null;
  }
};

export const deleteGleipFlow = async (id: string) => {
  try {
    await DeleteGleipFlow(id);
    
    // Get the current gleips
    const currentGleips = get(gleipFlows);
    const currentActiveIndex = get(activeGleipFlowIndex);
    
    // Find and remove the deleted gleip
    const filteredGleips = currentGleips.filter(g => g.id !== id);
    
    // Recalculate sorting indices to ensure they are sequential
    recalculateSortingIndices(filteredGleips);
    
    // Save the updated indices to the backend
    for (const gleipFlow of filteredGleips) {
      await saveGleipFlow(gleipFlow);
    }
    
    // Update the store with the filtered and reindexed gleips
    gleipFlows.set(filteredGleips);
    
    // Adjust the active index and update the selected GleipFlow ID
    let newActiveIndex = currentActiveIndex !== null ? currentActiveIndex : 0;
    if (newActiveIndex >= filteredGleips.length) {
      newActiveIndex = Math.max(0, filteredGleips.length - 1);
    }
    
    activeGleipFlowIndex.set(filteredGleips.length > 0 ? newActiveIndex : null);
    
    // Update the backend with the new selected GleipFlow ID
    if (filteredGleips.length > 0 && newActiveIndex < filteredGleips.length) {
      try {
        await SetSelectedGleipFlowID(filteredGleips[newActiveIndex].id);
        console.log(`Updated selected GleipFlow after deletion: ${filteredGleips[newActiveIndex].id}`);
      } catch (error) {
        console.error('Failed to update selected GleipFlow ID after deletion:', error);
      }
    }
  } catch (error) {
    console.error('Failed to delete GleipFlow:', error);
  }
};

// Helper function to recalculate sorting indices to ensure they are sequential from 1 to n
const recalculateSortingIndices = (gleipFlows: GleipFlow[]) => {
  // First sort by existing sortingIndex to maintain relative order
  gleipFlows.sort((a, b) => a.sortingIndex - b.sortingIndex);
  
  // Then reassign indices from 1 to n
  gleipFlows.forEach((gleip, index) => {
    gleip.sortingIndex = index + 1; // 1-based indexing
  });
};

export const saveGleipFlow = async (gleipFlow: GleipFlow) => {
  try {
    // Backend now handles saving automatically, but keep this for compatibility
    console.log('saveGleipFlow called - backend should handle persistence automatically');
    return true;
  } catch (error) {
    console.error('Failed to save GleipFlow:', error);
    return false;
  }
};

export const addStep = async (type: 'request' | 'script' | 'chef') => {
  const $activeGleipFlowIndex = get(activeGleipFlowIndex);
  const $gleipFlows = get(gleipFlows);
  
  if ($activeGleipFlowIndex === null || $activeGleipFlowIndex >= $gleipFlows.length) return;
  
  const gleipFlow = $gleipFlows[$activeGleipFlowIndex];
  
  try {
    // Call backend to add the step
    const newStep = await AddStepToGleipFlow(gleipFlow.id, type);
    console.log('Backend returned new step:', newStep);
    
    // Reload entire flows from backend to ensure consistency
    await loadGleipFlows();
    
    // Set the new step as active
    const updatedGleips = get(gleipFlows);
    const updatedFlow = updatedGleips[$activeGleipFlowIndex];
    if (updatedFlow) {
      activeStepIndex.set(updatedFlow.steps.length - 1);
    }
    
    return newStep;
  } catch (error) {
    console.error('Failed to add step:', error);
    return null;
  }
};

export const deleteStep = async (index: number) => {
  const $activeGleipFlowIndex = get(activeGleipFlowIndex);
  const $gleipFlows = get(gleipFlows);
  const $activeStepIndex = get(activeStepIndex);
  
  if ($activeGleipFlowIndex === null || $activeGleipFlowIndex >= $gleipFlows.length) return;
  
  // Create a deep copy of the gleipFlow to avoid reference issues
  const gleipFlow = { ...$gleipFlows[$activeGleipFlowIndex] };
  
  // Create a new array of steps without the step to delete
  gleipFlow.steps = [
    ...gleipFlow.steps.slice(0, index),
    ...gleipFlow.steps.slice(index + 1)
  ];
  
  // Update the gleips store
  const updatedGleipFlows = [
    ...$gleipFlows.slice(0, $activeGleipFlowIndex),
    gleipFlow,
    ...$gleipFlows.slice($activeGleipFlowIndex + 1)
  ];
  gleipFlows.set(updatedGleipFlows);
  
  // Save the updated gleipFlow to the backend using updateGleipFlow
  await updateGleipFlow(gleipFlow);
  
  // Update activeStepIndex if needed
  if ($activeStepIndex === index) {
    activeStepIndex.set(null);
  } else if ($activeStepIndex !== null && $activeStepIndex > index) {
    activeStepIndex.set($activeStepIndex - 1);
  }
  
  return true;
};

export const updateStepSelection = async (index: number, selected: boolean) => {
  const $activeGleipFlowIndex = get(activeGleipFlowIndex);
  const $gleipFlows = get(gleipFlows);
  
  if ($activeGleipFlowIndex === null || $activeGleipFlowIndex >= $gleipFlows.length) return;
  if (index < 0 || index >= $gleipFlows[$activeGleipFlowIndex].steps.length) return;
  
  // Create a deep copy of the gleipFlow to avoid reference issues
  const gleipFlow = { ...$gleipFlows[$activeGleipFlowIndex] };
  
  // Update the step selection
  gleipFlow.steps = gleipFlow.steps.map((step, i) => 
    i === index ? { ...step, selected } : step
  );
  
  // Update the gleips store
  const updatedGleipFlows = [
    ...$gleipFlows.slice(0, $activeGleipFlowIndex),
    gleipFlow,
    ...$gleipFlows.slice($activeGleipFlowIndex + 1)
  ];
  gleipFlows.set(updatedGleipFlows);
  
  // Save the updated gleipFlow to the backend using updateGleipFlow
  await updateGleipFlow(gleipFlow);
  
  return true;
};

export const updateStepExpansion = async (index: number, isExpanded: boolean) => {
  const $activeGleipFlowIndex = get(activeGleipFlowIndex);
  const $gleipFlows = get(gleipFlows);
  
  if ($activeGleipFlowIndex === null || $activeGleipFlowIndex >= $gleipFlows.length) return;
  
  // Create a deep copy of the gleipFlow to avoid reference issues
  const gleipFlow = { ...$gleipFlows[$activeGleipFlowIndex] };
  
  // Handle variables step (index -1)
  if (index === -1) {
    gleipFlow.isVariableStepExpanded = isExpanded;
    
    // Update the store
    const updatedGleipFlows = [
      ...$gleipFlows.slice(0, $activeGleipFlowIndex),
      gleipFlow,
      ...$gleipFlows.slice($activeGleipFlowIndex + 1)
    ];
    gleipFlows.set(updatedGleipFlows);
    
    // Update backend
    await UpdateStepExpansion(gleipFlow.id, index, isExpanded);
    return;
  }
  
  if (index < 0 || index >= gleipFlow.steps.length) return;
  
  // Update the step expansion
  gleipFlow.steps = gleipFlow.steps.map((step, i) => {
    if (i !== index) return step;
    
    const updatedStep = { ...step };
    if (step.stepType === 'request' && step.requestStep) {
      updatedStep.requestStep = {
        ...step.requestStep,
        stepAttributes: {
          ...step.requestStep.stepAttributes,
          isExpanded: isExpanded
        }
      };
    } else if (step.stepType === 'script' && step.scriptStep) {
      updatedStep.scriptStep = {
        ...step.scriptStep,
        stepAttributes: {
          ...step.scriptStep.stepAttributes,
          isExpanded: isExpanded
        }
      };
    } else if (step.stepType === 'chef' && step.chefStep) {
      updatedStep.chefStep = {
        ...step.chefStep,
        stepAttributes: {
          ...step.chefStep.stepAttributes,
          isExpanded: isExpanded
        }
      };
    }
    return updatedStep;
  });
  
  // Update the store
  const updatedGleipFlows = [
    ...$gleipFlows.slice(0, $activeGleipFlowIndex),
    gleipFlow,
    ...$gleipFlows.slice($activeGleipFlowIndex + 1)
  ];
  gleipFlows.set(updatedGleipFlows);
  
  // Update backend
  await UpdateStepExpansion(gleipFlow.id, index, isExpanded);
};

// Chef Step Management Functions
export const updateChefStep = async (stepIndex: number, updates: any) => {
  const $activeGleipFlowIndex = get(activeGleipFlowIndex);
  const $gleipFlows = get(gleipFlows);
  
  if ($activeGleipFlowIndex === null || $activeGleipFlowIndex >= $gleipFlows.length) return false;
  
  const gleipFlow = $gleipFlows[$activeGleipFlowIndex];
  
  if (stepIndex < 0 || stepIndex >= gleipFlow.steps.length) return false;
  
  const step = gleipFlow.steps[stepIndex];
  if (!step || step.stepType !== 'chef' || !step.chefStep) return false;
  
  try {
    // Call backend to update the chef step
    await UpdateChefStep(
      gleipFlow.id, 
      stepIndex, 
      updates.inputVariable || step.chefStep.inputVariable || '',
      updates.outputVariable || step.chefStep.outputVariable || '',
      updates.name || step.chefStep.stepAttributes.name || ''
    );
    
    // Reload flows from backend to get updated state
    await loadGleipFlows();
    
    return true;
  } catch (error) {
    console.error('Failed to update chef step:', error);
    return false;
  }
};

export const addChefAction = async (stepIndex: number) => {
  const $activeGleipFlowIndex = get(activeGleipFlowIndex);
  const $gleipFlows = get(gleipFlows);
  
  if ($activeGleipFlowIndex === null || $activeGleipFlowIndex >= $gleipFlows.length) {
    console.log('Invalid gleipFlowIndex');
    return false;
  }
  
  const gleipFlow = $gleipFlows[$activeGleipFlowIndex];
  
  try {
    // Call backend to add the chef action
    await AddChefAction(gleipFlow.id, stepIndex);
    
    // Reload flows from backend to get updated state
    await loadGleipFlows();
    
    return true;
  } catch (error) {
    console.error('Failed to add chef action:', error);
    return false;
  }
};

export const removeChefAction = async (stepIndex: number, actionIndex: number) => {
  const $activeGleipFlowIndex = get(activeGleipFlowIndex);
  const $gleipFlows = get(gleipFlows);
  
  if ($activeGleipFlowIndex === null || $activeGleipFlowIndex >= $gleipFlows.length) return false;
  
  const gleipFlow = $gleipFlows[$activeGleipFlowIndex];
  
  try {
    // Call backend to remove the chef action
    await RemoveChefAction(gleipFlow.id, stepIndex, actionIndex);
    
    // Reload flows from backend to get updated state
    await loadGleipFlows();
    
    return true;
  } catch (error) {
    console.error('Failed to remove chef action:', error);
    return false;
  }
};

export const updateChefAction = async (stepIndex: number, actionIndex: number, updates: any) => {
  const $activeGleipFlowIndex = get(activeGleipFlowIndex);
  const $gleipFlows = get(gleipFlows);
  
  if ($activeGleipFlowIndex === null || $activeGleipFlowIndex >= $gleipFlows.length) return false;
  
  const gleipFlow = $gleipFlows[$activeGleipFlowIndex];
  
  if (stepIndex < 0 || stepIndex >= gleipFlow.steps.length) return false;
  
  const step = gleipFlow.steps[stepIndex];
  if (!step || step.stepType !== 'chef' || !step.chefStep) return false;
  
  if (actionIndex < 0 || actionIndex >= step.chefStep.actions.length) return false;
  
  try {
    // Merge updates with existing action
    const updatedAction = { ...step.chefStep.actions[actionIndex], ...updates };
    
    // Call backend to update the chef action
    await UpdateChefAction(gleipFlow.id, stepIndex, actionIndex, updatedAction);
    
    // Reload flows from backend to get updated state
    await loadGleipFlows();
    
    return true;
  } catch (error) {
    console.error('Failed to update chef action:', error);
    return false;
  }
};

// Paste request from clipboard at specific position
export const pasteRequestAtPosition = async (position: number) => {
  const $activeGleipFlowIndex = get(activeGleipFlowIndex);
  const $gleipFlows = get(gleipFlows);
  
  if ($activeGleipFlowIndex === null || $activeGleipFlowIndex >= $gleipFlows.length) {
    console.log('Invalid gleipFlowIndex');
    return { success: false, message: 'No active gleipFlow to paste into' };
  }
  
  const gleipFlow = $gleipFlows[$activeGleipFlowIndex];
  
  try {
    // Call backend to paste the request
    const newStep = await PasteRequestToGleipFlowAtPosition(gleipFlow.id, position);
    
    // Reload flows from backend to get updated state
    await loadGleipFlows();
    
    // Set the new step as active
    const updatedGleips = get(gleipFlows);
    const updatedFlow = updatedGleips[$activeGleipFlowIndex];
    if (updatedFlow) {
      activeStepIndex.set(position === -1 ? updatedFlow.steps.length - 1 : position);
    }
    
    return { success: true, message: 'Request pasted successfully' };
  } catch (error) {
    console.error('Failed to paste request:', error);
    return { success: false, message: `Failed to paste request: ${error}` };
  }
};

// Helper Functions
export function ensureValidRawRequest(step: GleipFlowStep): void {
  if (step.stepType === 'request' && step.requestStep) {
    // Check if we need to generate a raw request dump
    if (!step.requestStep.request.dump || step.requestStep.request.dump.trim() === '') {
      // Generate a basic raw request if we don't have one
      step.requestStep.request.dump = generateRawHttpRequest(
        'GET',
        '/',
        { 'Host': step.requestStep.request.host || 'example.com' },
        ''
      );
    }
  }
}

export const updateGleipFlow = async (gleipFlow: GleipFlow) => {
  try {
    // Call backend UpdateGleipFlow which automatically saves
    await UpdateGleipFlow(backend.GleipFlow.createFrom(gleipFlow));
    
    // Don't reload - this causes infinite loops
    // The backend is the source of truth, but we don't need to reload everything
    // await loadGleipFlows();
    
    return true;
  } catch (error) {
    console.error('Failed to update GleipFlow:', error);
    return false;
  }
}; 