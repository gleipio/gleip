<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte';
  import type { GleipFlowStep, ExecutionResult } from '../types';
  import RequestStep from './RequestStep.svelte';
  import ScriptStep from './ScriptStep.svelte';
  import ChefStep from './ChefStep.svelte';
  import VariablesStep from './VariablesStep.svelte';
  import { GetAvailableVariablesForStep, GetAvailableVariableValuesForStep } from '../../../../wailsjs/go/backend/App';
  import '../../../types/extensions';
  
  export let step: GleipFlowStep;
  export let stepIndex: number;
  export let isExpanded: boolean = false;
  export let executionResult: ExecutionResult | undefined = undefined;
  export let isExecuting: boolean = false;
  export let gleipFlowID: string = '';
  
  // Available variables for chef steps
  let availableVariables: string[] = [];
  let variableValues: Record<string, string> = {};
  
  const dispatch = createEventDispatcher();
  
  // Load available variables when step changes and it's a chef step
  $: if (step.stepType === 'chef' && gleipFlowID && stepIndex >= 0) {
    loadAvailableVariables();
  }
  
  async function loadAvailableVariables() {
    if (step.stepType === 'chef' && gleipFlowID) {
      try {
        // Adjust stepIndex to account for the fake variables step at index 0
        const actualStepIndex = stepIndex > 0 ? stepIndex - 1 : 0;
        
        // Get variable names
        const variables = await GetAvailableVariablesForStep(gleipFlowID, actualStepIndex);
        availableVariables = Array.isArray(variables) ? variables : [];
        
        // Get variable values
        const values = await GetAvailableVariableValuesForStep(gleipFlowID, actualStepIndex);
        variableValues = values || {};
        
        console.log('Loaded variables:', availableVariables);
        console.log('Loaded variable values:', variableValues);
      } catch (error) {
        console.error('Failed to get available variables:', error);
        availableVariables = [];
        variableValues = {};
      }
    } else {
      availableVariables = [];
      variableValues = {};
    }
  }
  
  // Get step type label
  function getStepTypeLabel(stepType: string): string {
    switch (stepType) {
      case 'request': return 'REQ';
      case 'script': return 'JS';
      case 'chef': return 'CHEF';
      case 'variables': return 'VAR';
      default: return 'STEP';
    }
  }
  
  // Get step type color classes
  function getStepTypeColorClass(stepType: string): string {
    switch (stepType) {
      case 'request': return 'bg-blue-500/30 text-blue-300';
      case 'script': return 'bg-amber-500/30 text-amber-300';
      case 'chef': return 'bg-purple-500/30 text-purple-300';
      case 'variables': return 'bg-green-500/30 text-green-300';
      default: return 'bg-gray-500/30 text-gray-100';
    }
  }
  
  // Toggle expand/collapse
  function toggleExpand() {
    dispatch('toggleExpand', { stepIndex });
  }
  
  // Delete step
  function deleteStep(event: MouseEvent) {
    event.stopPropagation();
    dispatch('deleteStep', { stepIndex });
  }
  
  // Update selection
  function updateSelection(event: Event) {
    event.stopPropagation();
    const target = event.currentTarget;
    if (target instanceof HTMLInputElement) {
      dispatch('updateSelection', { stepIndex, selected: target.checked });
    }
  }
  
  // Handle step execution
  function executeStep() {
    dispatch('executeStep', { stepIndex });
  }
  
  // Handle request step updates
  function handleRequestUpdate(event: CustomEvent) {
    if (!step.requestStep) return;
    
    const updates = event.detail;
    dispatch('updateStep', { 
      stepIndex, 
      stepType: 'request',
      updates 
    });
  }
  
  // Handle script step updates
  function handleScriptUpdate(event: CustomEvent) {
    if (!step.scriptStep) return;
    
    const updates = event.detail;
    dispatch('updateStep', { 
      stepIndex, 
      stepType: 'script',
      updates 
    });
  }
  
  // Handle chef step updates
  function handleChefUpdate(event: CustomEvent) {
    if (!step.chefStep) return;
    
    const updates = event.detail;
    dispatch('updateStep', { 
      stepIndex, 
      stepType: 'chef',
      updates 
    });
  }
  
  // Handle variables step updates
  function handleVariablesUpdate(event: CustomEvent) {
    if (!step.variablesStep) return;
    
    const updates = event.detail;
    dispatch('updateStep', { 
      stepIndex, 
      stepType: 'variables',
      updates 
    });
  }
  
  // Handle editor mount
  function handleEditorMount(event: CustomEvent) {
    dispatch('editorMount', event.detail);
  }
  
  // Handle mode change in request step
  function handleRequestModeChange(event: CustomEvent) {
    const { mode } = event.detail;
    dispatch('requestModeChange', { stepIndex, mode });
  }
  
  // Handle starting fuzzing process
  function handleStartFuzzing(event: CustomEvent) {
    dispatch('startFuzzing', { stepIndex });
  }
</script>

<div class="border border-[var(--color-table-border)] overflow-hidden w-[200px] transition-all duration-200 {isExpanded ? 'w-[600px]' : ''} h-[calc(100vh-126px)] flex-shrink-0 flex flex-col">
  <!-- Title bar -->
  <div 
    class="p-3 cursor-pointer {isExpanded ? 'hover:bg-[var(--color-table-row-hover)]' : 'bg-[var(--color-midnight-darker)] hover:bg-[var(--color-table-row-hover)] flex-grow'}"
    on:click={toggleExpand}
    on:keydown={(e) => e.key === 'Enter' && toggleExpand()}
    role="button"
    tabindex="0"
  >
    {#if isExpanded}
      <!-- Expanded header -->
      <div class="flex justify-between items-center w-full">
        <div class="flex items-center" on:mousedown|stopPropagation role="presentation">
          <!-- Simple checkbox for selection -->
          {#if step.stepType !== 'variables'}
          <input 
            type="checkbox" 
            class="mr-2 w-5 h-5 accent-[var(--color-secondary-accent)] cursor-pointer scale-150"
            checked={step.selected}
            on:click|stopPropagation
            on:change|stopPropagation={updateSelection}
            aria-label="Select this step to execute"
            disabled={step.stepType === 'variables'} 
          />
          <span class={`mr-2 text-xs px-1.5 py-0.5 rounded ${getStepTypeColorClass(step.stepType)}`}>
            {getStepTypeLabel(step.stepType)}
          </span>
          {/if}
          <span class={`
            text-sm font-medium text-gray-100
          `}>
            {
              step.stepType === 'request' && step.requestStep ? step.requestStep.stepAttributes.name : 
              step.stepType === 'script' && step.scriptStep ? step.scriptStep.stepAttributes.name :
              step.stepType === 'chef' && step.chefStep ? step.chefStep.stepAttributes.name :
              step.stepType === 'variables' ? 'Variables' :
              'Unnamed Step'
            }
          </span>
          {#if step.stepType === 'request' && step.requestStep}
            <!-- Fuzz mode indicator -->
            {#if step.requestStep.isFuzzMode}
              <span class="ml-2 px-2 py-0.5 bg-[var(--color-secondary-accent)] text-black text-xs rounded">
                Fuzz
                <!-- {#if step.requestStep.fuzzSettings?.fuzzResults && step.requestStep.fuzzSettings.fuzzResults.length > 0}
                  ({step.requestStep.fuzzSettings.fuzzResults.length})
                {/if} -->
              </span>
            {/if}
          {/if}
        </div>
        <div class="flex items-center">
          <!-- Only show delete button if not Variables step -->
          {#if !(stepIndex === 0 && step.stepType === 'variables')}
            <button 
              class="text-gray-500 hover:text-gray-100 ml-2"
              on:click={deleteStep}
            >
              ×
            </button>
          {/if}
          <span class="ml-2 text-gray-500">
            {isExpanded ? '▲' : '▼'}
          </span>
        </div>
      </div>
    {:else}
      <!-- Collapsed header -->
      <div class="flex flex-col h-full">
        {#if step.stepType !== 'variables'}
          <div class="flex items-center mb-2">
            <input 
              type="checkbox" 
              class="mr-2 w-4 h-4 accent-[var(--color-secondary-accent)] cursor-pointer"
              checked={step.selected}
              on:click|stopPropagation
              on:change|stopPropagation={updateSelection}
              aria-label="Select this step to execute"
              disabled={step.stepType === 'variables'} 
              tabindex="0"
            />
            <span class={`mr-2 text-xs px-1.5 py-0.5 rounded ${getStepTypeColorClass(step.stepType)}`}>
              {getStepTypeLabel(step.stepType)}
            </span>
            <span class="text-sm font-medium text-gray-50">
              {
                step.stepType === 'request' && step.requestStep ? step.requestStep.stepAttributes.name : 
                step.stepType === 'script' && step.scriptStep ? step.scriptStep.stepAttributes.name :
                step.stepType === 'chef' && step.chefStep ? step.chefStep.stepAttributes.name :
                step.stepType === 'variables' ? 'Variables' :
                'Unnamed Step'
              }
            </span>
            
            <!-- Add delete button for collapsed view (except Variables) -->
            {#if !(stepIndex === 0 && step.stepType === 'variables')}
              <button 
                class="text-gray-500 hover:text-gray-100 ml-auto"
                on:click={deleteStep}
              >
                ×
              </button>
            {/if}
          </div>
        {/if}        
        <!-- Render components in collapsed state -->
        {#if step.stepType === 'request' && step.requestStep}
          <RequestStep 
            requestStep={step.requestStep}
            executionResult={executionResult}
            isExecuting={isExecuting}
            isExpanded={false}
            on:update={handleRequestUpdate}
            on:execute={executeStep}
            on:editorMount={handleEditorMount}
            on:modeChange={handleRequestModeChange}
            on:startFuzzing={handleStartFuzzing}
          />
        {:else if step.stepType === 'script' && step.scriptStep}
          <ScriptStep 
            scriptStep={step.scriptStep}
            executionResult={executionResult}
            isExecuting={isExecuting}
            isExpanded={false}
            on:update={handleScriptUpdate}
            on:execute={executeStep}
            on:editorMount={handleEditorMount}
          />
        {:else if step.stepType === 'chef'}
          <ChefStep 
            executionResult={executionResult}
            stepIndex={stepIndex > 0 ? stepIndex - 1 : 0}
            availableVariables={availableVariables}
            variableValues={variableValues}
            isExpanded={false}
            on:update={handleChefUpdate}
            on:execute={executeStep}
          />
        {:else if step.stepType === 'variables' && step.variablesStep}
          <VariablesStep 
            variables={step.variablesStep || {}}
            isExpanded={false}
            on:update={handleVariablesUpdate}
          />
        {/if}
      </div>
    {/if}
  </div>
  
  <!-- Content section (only visible when expanded) -->
  {#if isExpanded}
    <div class="px-4 bg-[var(--color-midnight)] overflow-y-auto flex-1 flex-grow">
      {#if step.stepType === 'request' && step.requestStep}
        <RequestStep 
          requestStep={step.requestStep}
          executionResult={executionResult}
          isExecuting={isExecuting}
          isExpanded={isExpanded}
          on:update={handleRequestUpdate}
          on:execute={executeStep}
          on:editorMount={handleEditorMount}
          on:modeChange={handleRequestModeChange}
          on:startFuzzing={handleStartFuzzing}
        />
      {:else if step.stepType === 'script' && step.scriptStep}
        <ScriptStep 
          scriptStep={step.scriptStep}
          executionResult={executionResult}
          isExecuting={isExecuting}
          isExpanded={isExpanded}
          on:update={handleScriptUpdate}
          on:execute={executeStep}
          on:editorMount={handleEditorMount}
        />
      {:else if step.stepType === 'chef'}
        <ChefStep 
          executionResult={executionResult}
          stepIndex={stepIndex > 0 ? stepIndex - 1 : 0}
          availableVariables={availableVariables}
          variableValues={variableValues}
          isExpanded={isExpanded}
          on:update={handleChefUpdate}
          on:execute={executeStep}
        />
      {:else if step.stepType === 'variables' && step.variablesStep}
        <VariablesStep 
          variables={step.variablesStep || {}}
          isExpanded={isExpanded}
          on:update={handleVariablesUpdate}
        />
      {/if}
    </div>
  {/if}
</div> 