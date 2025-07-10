import { get } from 'svelte/store';
import { ExecuteGleipFlow, ExecuteSingleStep } from '../../../../wailsjs/go/backend/App';
import { EventsOn } from '../../../../wailsjs/runtime/runtime';
import type { ExecutionResult, StepExecutionEvent } from '../types';
import { gleipFlows, activeGleipFlowIndex, isExecuting, saveGleipFlow, loadGleipFlows } from '../store/gleipStore';

/**
 * Service for handling gleip execution
 */
export class GleipFlowExecutionService {
  private static initialized = false;

  /**
   * Initialize event listeners for gleip execution
   */
  public static init() {
    if (this.initialized) return;

    console.log("Initializing GleipFlowExecutionService event listeners");

    // Set up event listener for real-time step execution results
    EventsOn('gleipFlow:stepExecuted', (data: any) => {
      // Handle the mismatch between backend and frontend field names
      const eventData: StepExecutionEvent = {
        gleipId: data.gleipFlowId,  // Map from backend's gleipFlowId to our gleipId
        currentStepIndex: data.currentStepIndex,
        results: data.results || []
      };
      
      console.log("Received step execution update:", eventData);
      
      // Only process if we have an active gleip and it matches the event gleip ID
      const currentGleipFlows = get(gleipFlows);
      const currentActiveIndex = get(activeGleipFlowIndex);
      
      if (currentActiveIndex !== null && 
          currentActiveIndex < currentGleipFlows.length &&
          currentGleipFlows[currentActiveIndex].id === eventData.gleipId) {
        
        // Update request with actual sent data if available
        if (eventData.results && eventData.results.length > 0) {
          for (const result of eventData.results) {
            if (result.actualRawRequest) {
              console.log(`Received actualRawRequest for step ${result.stepId}, length: ${result.actualRawRequest.length}`);
              
              // Find the corresponding step
              const stepIndex = currentGleipFlows[currentActiveIndex].steps.findIndex(step => 
                (step.stepType === 'request' && step.requestStep && step.requestStep.stepAttributes.id === result.stepId)
              );
              
              if (stepIndex !== -1 && currentGleipFlows[currentActiveIndex].steps[stepIndex].requestStep) {
                const requestStep = currentGleipFlows[currentActiveIndex].steps[stepIndex].requestStep;
                if (requestStep) {
                  console.log(`Automatically updating request editor for step ${result.stepId} with actual request`);
                  requestStep.request.dump = result.actualRawRequest;
                  gleipFlows.set([...currentGleipFlows]); // Trigger reactivity
                  // DON'T save during execution - this overwrites execution results!
                  // saveGleipFlow(currentGleipFlows[currentActiveIndex]);
                }
              }
            }
          }
        }
        
        // Update execution results
        this.updateExecutionResults(eventData);
      } else {
        console.log("Ignoring step execution update - no matching gleip found", eventData);
      }
    });

    this.initialized = true;
    console.log("GleipFlowExecutionService initialized successfully");
  }

  /**
   * Execute an entire gleip
   */
  public static async executeGleipFlow(gleipId: string): Promise<boolean> {
    try {
      isExecuting.set(true);
      
      const currentGleipFlows = get(gleipFlows);
      const currentActiveIndex = get(activeGleipFlowIndex);
      
      if (currentActiveIndex === null || currentActiveIndex >= currentGleipFlows.length) {
        isExecuting.set(false);
        return false;
      }
      
      const gleip = currentGleipFlows[currentActiveIndex];
      
      // Check if gleipId is valid - if not, use the ID from the active flow
      if (!gleipId || gleipId.trim() === '') {
        console.warn("Empty gleipId provided, using active flow ID instead:", gleip.id);
        gleipId = gleip.id;
      }
      
      // Ensure the ID matches the active flow
      if (gleipId !== gleip.id) {
        console.warn("GleipId mismatch, using active flow ID instead. Provided:", gleipId, "Active:", gleip.id);
        gleipId = gleip.id;
      }
      
      if (!gleipId || gleipId.trim() === '') {
        console.error("Cannot execute gleipFlow with empty ID");
        isExecuting.set(false);
        return false;
      }
      
      // Only clear results for steps being executed in this run
      const selectedStepIds = gleip.steps
        .filter(step => step.selected)
        .map(step => step.stepType === 'request' ? step.requestStep?.stepAttributes.id : 
                     step.stepType === 'script' ? step.scriptStep?.stepAttributes.id : 
                     gleip.id + '-variables')
        .filter(id => id !== undefined);
        
      // Filter out results for steps being executed, keep others in the flow
      const currentResults = gleip.executionResults || [];
      const filteredResults = currentResults.filter((result: ExecutionResult) => !selectedStepIds.includes(result.stepId));
      
      // Update the flow with filtered results
      const updatedFlow = { ...gleip, executionResults: filteredResults };
      currentGleipFlows[currentActiveIndex] = updatedFlow;
      gleipFlows.set([...currentGleipFlows]);
      
      console.log("Executing gleip with steps:", gleip.steps);
      
      try {
        // Start execution - results will come in through the event listener
        const results = await ExecuteGleipFlow(gleipId);  // Pass the gleipId string
        console.log("GleipFlow execution completed, results:", results);
        
        // Add final results if they exist and have not already been processed by events
        if (results && results.length > 0) {
          console.log("Processing final execution results");
          this.mergeExecutionResults(results);
          
          // Force a UI update after receiving the final results
          gleipFlows.set([...get(gleipFlows)]);
        }
        
        // Reload flow data from backend to sync merged variables
        console.log("Reloading flow data to sync merged variables from backend");
        await loadGleipFlows();
      } catch (error) {
        console.error("Error during GleipFlow execution:", error);
        // Ensure we still try to update the UI even if there was an error
        gleipFlows.set([...get(gleipFlows)]);
      }
      
      // Force UI update by setting isExecuting to false
      isExecuting.set(false);
      
      return true;
    } catch (error) {
      console.error('Failed to execute gleip:', error);
      isExecuting.set(false);
      return false;
    }
  }

  /**
   * Execute a single step from a gleip
   */
  public static async executeSingleStep(stepIndex: number): Promise<boolean> {
    console.log("🚨 SINGLE STEP METHOD CALLED WITH INDEX:", stepIndex);
    alert("🚨 SINGLE STEP METHOD CALLED WITH INDEX: " + stepIndex);
    
    const currentGleipFlows = get(gleipFlows);
    const currentActiveIndex = get(activeGleipFlowIndex);
    
    if (currentActiveIndex === null || currentActiveIndex >= currentGleipFlows.length) return false;
    if (stepIndex < 0 || stepIndex >= currentGleipFlows[currentActiveIndex].steps.length) return false;
    
    try {
      isExecuting.set(true);
      
      const gleip = currentGleipFlows[currentActiveIndex];
      const step = gleip.steps[stepIndex];
      
      // Ensure gleip has a valid ID
      if (!gleip.id || gleip.id.trim() === '') {
        console.error("Cannot execute step in gleipFlow with empty ID");
        isExecuting.set(false);
        return false;
      }
      
      // Only allow executing request steps with Send Req button
      if (step.stepType !== 'request') {
        console.error("Send Req button can only execute request steps, got:", step.stepType);
        isExecuting.set(false);
        return false;
      }
      
      console.log(`Executing single step ${stepIndex} from gleip ${gleip.id}`);
      
      // Call the backend method that handles single step execution logic
      const results = await ExecuteSingleStep(gleip.id, stepIndex);
      console.log("Single step execution completed, results:", results);
      
      // Process final results if they exist
      if (results && results.length > 0) {
        console.log("Processing final single step execution results");
        this.mergeExecutionResults(results);
      }
      
      // Reload flow data from backend to sync merged variables
      console.log("Reloading flow data to sync merged variables from backend (single step)");
      await loadGleipFlows();
      
      isExecuting.set(false);
      return true;
    } catch (error) {
      console.error('Failed to execute single step:', error);
      isExecuting.set(false);
      return false;
    }
  }

  /**
   * Update execution results from a step execution event
   */
  private static updateExecutionResults(data: StepExecutionEvent): void {
    // Update the execution results in the active flow
    const currentGleipFlows = get(gleipFlows);
    const currentActiveIndex = get(activeGleipFlowIndex);
    
    if (currentActiveIndex === null || currentActiveIndex >= currentGleipFlows.length) return;
    
    const activeFlow = currentGleipFlows[currentActiveIndex];
    const newResults = [...data.results];
    
    // Get IDs of steps that were actually executed
    const executedStepIds = newResults.map(r => r.stepId);
    
    // Keep previous results for steps that weren't executed
    const currentResults = activeFlow.executionResults || [];
    for (const oldResult of currentResults) {
      // If this result is not in the new results (step wasn't executed), preserve it
      if (!executedStepIds.includes(oldResult.stepId)) {
        newResults.push(oldResult);
      }
    }
    
    // Update the flow with new execution results
    const updatedFlow = { ...activeFlow, executionResults: newResults };
    currentGleipFlows[currentActiveIndex] = updatedFlow;
    gleipFlows.set([...currentGleipFlows]);
    
    // If all steps are complete, update isExecuting flag
    if (data.currentStepIndex === activeFlow.steps.length - 1) {
      isExecuting.set(false);
    }
  }

  /**
   * Merge new execution results with existing ones
   */
  private static mergeExecutionResults(results: ExecutionResult[]): void {
    const currentGleipFlows = get(gleipFlows);
    const currentActiveIndex = get(activeGleipFlowIndex);
    
    if (currentActiveIndex === null || currentActiveIndex >= currentGleipFlows.length) return;
    
    const activeFlow = currentGleipFlows[currentActiveIndex];
    const currentResults = activeFlow.executionResults || [];
    
    // Create new array with new results taking precedence
    const merged = [
      ...results,
      ...currentResults.filter(r => !results.some(nr => nr.stepId === r.stepId))
    ];
    
    // Update the flow
    const updatedFlow = { ...activeFlow, executionResults: merged };
    currentGleipFlows[currentActiveIndex] = updatedFlow;
    gleipFlows.set([...currentGleipFlows]);
  }
} 