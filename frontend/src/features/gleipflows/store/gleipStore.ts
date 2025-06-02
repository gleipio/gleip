import { writable, derived, get } from 'svelte/store';
import { SaveGleipFlow, GetGleipFlows, DeleteGleipFlow, CreateGleipFlow, AddStepToGleipFlow, SetSelectedGleipFlowID } from '../../../../wailsjs/go/backend/App';
import { backend } from '../../../../wailsjs/go/models';
import type { GleipFlow, ExecutionResult, GleipFlowStep } from '../types';
import { generateRawHttpRequest } from '../utils/httpUtils';

// Store state
export const gleipFlows = writable<GleipFlow[]>([]);
export const activeGleipFlowIndex = writable<number>(0);
export const isCreatingGleipFlow = writable<boolean>(false);
export const newGleipFlowName = writable<string>('');
export const activeStepIndex = writable<number | null>(null);
export const executionResults = writable<ExecutionResult[]>([]);
export const isExecuting = writable<boolean>(false);
export const expandedStepIndices = writable<number[]>([]);

export const updatedRequestIds = writable<Set<string>>(new Set());

// Derived stores
export const activeGleipFlow = derived(
  [gleipFlows, activeGleipFlowIndex], 
  ([$gleipFlows, $activeGleipFlowIndex]) => {
    if ($gleipFlows.length === 0 || $activeGleipFlowIndex >= $gleipFlows.length) return null;
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
      
      return {
        id: gleipFlow.id,
        name: gleipFlow.name,
        variables: variables,
        sortingIndex: gleipFlow.sortingIndex || 0,
        steps: gleipSteps.map(step => {
          let convertedStep: GleipFlowStep = {
            stepType: step.stepType,
            selected: step.selected !== undefined ? step.selected : true
          };
          
          if (step.requestStep) {
            // Preserve the original step ID or generate a new one if missing
            const stepId = step.requestStep.id || crypto.randomUUID();
            
            convertedStep.requestStep = {
              id: stepId,
              name: step.requestStep.name,
              request: step.requestStep.request,
              variableExtracts: step.requestStep.variableExtracts || [],
              recalculateContentLength: step.requestStep.recalculateContentLength || true,
              gunzipResponse: step.requestStep.gunzipResponse || true,
              isConfigExpanded: step.requestStep.isConfigExpanded || false,
              isFuzzMode: step.requestStep.isFuzzMode || false,
              fuzzSettings: step.requestStep.fuzzSettings
            };
            
            // Ensure the request has a valid dump if missing
            ensureValidRawRequest(convertedStep);
          }
          
          if (step.scriptStep) {
            // Preserve the original step ID or generate a new one if missing
            const stepId = step.scriptStep.id || crypto.randomUUID();
            
            convertedStep.scriptStep = {
              id: stepId,
              name: step.scriptStep.name,
              content: step.scriptStep.content
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
    if (convertedGleipFlows.length > 0 && currentActiveIndex >= convertedGleipFlows.length) {
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
    let newActiveIndex = currentActiveIndex;
    if (currentActiveIndex >= filteredGleips.length) {
      newActiveIndex = Math.max(0, filteredGleips.length - 1);
    }
    
    activeGleipFlowIndex.set(newActiveIndex);
    
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
    // Ensure sortingIndex is set
    if (!gleipFlow.sortingIndex) {
      const currentGleips = get(gleipFlows);
      if (currentGleips.length > 0) {
        gleipFlow.sortingIndex = Math.max(...currentGleips.map(g => g.sortingIndex)) + 1;
      } else {
        gleipFlow.sortingIndex = 1;
      }
    }
    
    const gleipFlowToSave = {
      ...gleipFlow
    };
    
    // Save and get the updated gleipFlow with potentially new ID
    const savedGleipFlow = await SaveGleipFlow(backend.GleipFlow.createFrom(gleipFlowToSave));
    
    // If the ID changed, we need to update our store accordingly
    if (savedGleipFlow.id !== gleipFlow.id) {
      console.log(`GleipFlow ID changed from ${gleipFlow.id} to ${savedGleipFlow.id}`);
      // We need to update our local store with the new ID
      const currentGleips = get(gleipFlows);
      const updatedGleips = currentGleips.map(g => {
        if (g.id === gleipFlow.id) {
          return { ...g, id: savedGleipFlow.id };
        }
        return g;
      });
      gleipFlows.set(updatedGleips);
    }
    
    return true;
  } catch (error) {
    console.error('Failed to save GleipFlow:', error);
    return false;
  }
};

export const addStep = async (type: 'request' | 'script') => {
  const $activeGleipFlowIndex = get(activeGleipFlowIndex);
  const $gleipFlows = get(gleipFlows);
  
  if ($activeGleipFlowIndex === null || $activeGleipFlowIndex >= $gleipFlows.length) return;
  
  const gleipFlow = $gleipFlows[$activeGleipFlowIndex];
  
  try {
    // Call backend to add the step
    const newStep = await AddStepToGleipFlow(gleipFlow.id, type);
    
    // Update frontend state with the new step from backend
    gleipFlows.update($gleipFlows => {
      const updatedGleips = [...$gleipFlows];
      const updatedGleipFlow = {...updatedGleips[$activeGleipFlowIndex]};
      updatedGleipFlow.steps = [...updatedGleipFlow.steps, newStep as GleipFlowStep];
      updatedGleips[$activeGleipFlowIndex] = updatedGleipFlow;
      return updatedGleips;
    });
    
    // Set the new step as active
    activeStepIndex.set(gleipFlow.steps.length);
    
    return newStep;
  } catch (error) {
    console.error('Failed to add step:', error);
    return null;
  }
};

export const deleteStep = (index: number) => {
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
  
  // Save the updated gleipFlow to the backend
  saveGleipFlow(gleipFlow);
  
  // Update activeStepIndex if needed
  if ($activeStepIndex === index) {
    activeStepIndex.set(null);
  } else if ($activeStepIndex !== null && $activeStepIndex > index) {
    activeStepIndex.set($activeStepIndex - 1);
  }
  
  return true;
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