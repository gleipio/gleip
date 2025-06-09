<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import MonacoEditor from '../../../components/monaco/MonacoEditor.svelte';
  import type { ExecutionResult, ScriptStep as ScriptStepType } from '../types';
  
  export let scriptStep: ScriptStepType;
  export let executionResult: ExecutionResult | undefined = undefined;
  export let isExecuting: boolean = false;
  export let isExpanded: boolean = false;
  
  const dispatch = createEventDispatcher();
  
  // Handle Monaco editor mount
  function handleScriptEditorMount(e: CustomEvent) {
    dispatch('editorMount', { editor: e.detail.editor, type: 'script', stepId: scriptStep.stepAttributes.id });
  }
  
  // Handle script change
  function handleScriptChange(e: CustomEvent) {
    dispatch('update', { content: e.detail.value });
  }
  
  // Execute this script
  function executeScript() {
    dispatch('execute');
  }
</script>

<div class="w-full">
  {#if !isExpanded}
    <!-- Collapsed view -->
    <div class="flex flex-col space-y-2">
      <div class="text-sm font-medium text-gray-100">Script</div>
      <div class="text-xs text-gray-50 overflow-hidden font-mono">
        {scriptStep.content}
      </div>
      
      <!-- Script result -->
      {#if executionResult}
        <div class="flex items-center space-x-2">
          <span class={`px-2 py-0.5 text-xs rounded ${executionResult.success ? 'bg-green-500/30 text-green-300' : 'bg-red-500/30 text-red-300'}`}>
            {executionResult.success ? 'Success' : 'Failed'}
          </span>
          {#if executionResult.executionTime !== undefined}
            <span class="text-xs text-gray-50">{executionResult.executionTime}ms</span>
          {/if}
        </div>
      {:else}
        <div class="text-xs text-gray-500">Not executed</div>
      {/if}
    </div>
  {:else}
    <!-- Expanded view -->
    <div class="flex flex-col gap-4">
      <!-- Script section -->
      <div class="w-full">
        <div class="flex justify-between items-center mb-1">
          <h3 class="text-sm font-medium text-gray-50">Script</h3>
          <button
            class="px-3 py-1 bg-[var(--color-secondary-accent)] hover:bg-opacity-90 text-[var(--color-button-text)] rounded text-sm"
            on:click={executeScript}
            disabled={isExecuting}
            title="Execute only this step"
          >
            {isExecuting ? 'Executing...' : 'Run Script'}
          </button>
        </div>
        <div class="w-full h-48 border border-[var(--color-table-border)] overflow-hidden" aria-labelledby="script-editor-label-{scriptStep.stepAttributes.id}">
          <MonacoEditor
            language="javascript"
            value={scriptStep.content || ''}
            on:mount={handleScriptEditorMount}
            on:change={handleScriptChange}
          />
        </div>
      </div>
      
      <!-- Execution result section -->
      <div class="w-full">
        <div class="flex justify-between items-center mb-1">
          <h3 class="text-sm font-medium text-gray-50">Execution Result</h3>
          {#if executionResult}
            <div class="mt-auto flex items-center">
              <span class={`px-2 py-0.5 text-xs rounded ${executionResult.success ? 'bg-green-500/30 text-green-300' : 'bg-red-500/30 text-red-300'}`}>
                {executionResult.success ? 'Success' : 'Failed'}
              </span>
              {#if executionResult.executionTime !== undefined}
                <span class="ml-2 text-xs text-gray-50">{executionResult.executionTime}ms</span>
              {/if}
            </div>
          {/if}
        </div>
        
        {#if executionResult}
          {#if executionResult.errorMessage}
            <div class="h-48 border border-[var(--color-table-border)] overflow-hidden">
              <MonacoEditor
                value={executionResult.errorMessage}
                readOnly={true}
                language="text"
              />
            </div>
          {:else if executionResult.variables && Object.keys(executionResult.variables).length > 0}
            <div class="h-48 border border-[var(--color-table-border)] overflow-hidden">
              <MonacoEditor
                value={Object.entries(executionResult.variables)
                  .map(([name, value]) => `${name} = ${value}`)
                  .join('\n')}
                readOnly={true}
                language="javascript"
              />
            </div>
          {:else}
            <div class="h-48 border border-[var(--color-table-border)] overflow-hidden">
              <MonacoEditor
                value="Script executed successfully with no variable changes."
                readOnly={true}
                language="text"
              />
            </div>
          {/if}
        {:else}
          <div class="flex items-center justify-center h-48 text-gray-50 border border-[var(--color-table-border)] bg-[var(--color-midnight)]">
            Execute the flow to see results here
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div> 