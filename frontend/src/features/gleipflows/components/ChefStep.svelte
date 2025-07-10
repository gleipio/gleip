<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte';
  import { GetAvailableChefActions, GetChefStepSequentialPreview } from '../../../../wailsjs/go/backend/App';
  import { chef } from '../../../../wailsjs/go/models';
  import { updateChefStep, addChefAction, removeChefAction, updateChefAction, activeGleipFlow } from '../store/gleipStore';
  import type { ChefStep, ExecutionResult } from '../types';
  
  export let executionResult: ExecutionResult | undefined = undefined;
  export let stepIndex: number;
  export let isExpanded: boolean = false;
  
  // Get chefStep directly from store
  $: chefStep = $activeGleipFlow?.steps[stepIndex]?.chefStep;
  

  
  const dispatch = createEventDispatcher();
  
  // Available variables from previous steps (will be passed from parent)
  export let availableVariables: string[] = [];
  
  // Variable values for preview (will be passed from parent if available)
  export let variableValues: Record<string, string> = {};
  
  // Ensure availableVariables is always an array
  $: safeAvailableVariables = Array.isArray(availableVariables) ? availableVariables : [];
  
  // Ensure chefStep.actions is always an array
  $: safeActions = Array.isArray(chefStep?.actions) ? chefStep.actions : [];
  
  // Don't render if no chefStep
  $: showComponent = !!chefStep;
  
  // Available chef actions
  let availableActions: Record<string, string>[] = [];
  
  // Searchable dropdown state - initialize as empty object
  let searchableDropdowns: Record<number, {
    isOpen: boolean;
    searchTerm: string;
    filteredActions: Record<string, string>[];
    userIsTyping: boolean; // Track if user is actively typing
  }> = {};
  
  // Load available actions on mount
  onMount(() => {
    // Load available actions
    const loadActions = async () => {
      try {
        const actions = await GetAvailableChefActions();
        availableActions = Array.isArray(actions) ? actions : [];
      } catch (error) {
        console.error('ChefStep: Failed to load chef actions:', error);
        availableActions = [];
      }
    };
    
    loadActions();
    
    // Add global click listener
    document.addEventListener('click', handleGlobalClick);
    
    return () => {
      // Cleanup
      document.removeEventListener('click', handleGlobalClick);
    };
  });

  // Initialize dropdowns when availableActions and safeActions are ready
  $: if (availableActions.length > 0 && safeActions.length > 0) {
    // Initialize dropdown state for all actions
    safeActions.forEach((action, index) => {
      initDropdownState(index);
    });
    
    // Update search terms for existing dropdowns to show the selected action names
    // Only update if user is not actively typing
    safeActions.forEach((action, index) => {
      if (action.actionType && searchableDropdowns[index] && !searchableDropdowns[index].userIsTyping) {
        const actionName = availableActions.find(a => a.id === action.actionType)?.name || '';
        if (actionName && searchableDropdowns[index].searchTerm !== actionName) {
          searchableDropdowns[index].searchTerm = actionName;
        }
      }
    });
    searchableDropdowns = { ...searchableDropdowns }; // Trigger reactivity
  }
  
  // Initialize dropdown state for an action
  function initDropdownState(index: number) {
    if (!searchableDropdowns[index]) {
      // Get the current action type to initialize the search term
      const currentAction = safeActions[index];
      const currentActionType = currentAction?.actionType;
      const currentActionName = currentActionType && availableActions.length > 0 ? 
        availableActions.find(a => a.id === currentActionType)?.name || '' : '';
      
      searchableDropdowns[index] = {
        isOpen: false,
        searchTerm: currentActionName,
        filteredActions: availableActions,
        userIsTyping: false
      };
      
      // Force reactivity update
      searchableDropdowns = { ...searchableDropdowns };
    }
  }
  
  // Filter actions based on search term
  function filterActions(index: number, searchTerm: string) {
    if (!searchTerm.trim()) {
      searchableDropdowns[index].filteredActions = availableActions;
    } else {
      const lowercaseSearch = searchTerm.toLowerCase();
      searchableDropdowns[index].filteredActions = availableActions.filter(action => 
        action.name.toLowerCase().includes(lowercaseSearch) ||
        (action.description && action.description.toLowerCase().includes(lowercaseSearch))
      );
    }
  }
  
  // Handle search input
  function handleSearchInput(index: number, value: string) {
    initDropdownState(index);
    searchableDropdowns[index].searchTerm = value;
    searchableDropdowns[index].isOpen = true;
    searchableDropdowns[index].userIsTyping = true; // User is actively typing
    filterActions(index, value);
    searchableDropdowns = { ...searchableDropdowns }; // Trigger reactivity
  }

  // Handle focus - prepare for user to potentially replace the current action
  function handleFocus(index: number) {
    initDropdownState(index);
    searchableDropdowns[index].userIsTyping = true;
    searchableDropdowns[index].isOpen = true;
    filterActions(index, searchableDropdowns[index].searchTerm);
    searchableDropdowns = { ...searchableDropdowns };
  }
  
  // Toggle dropdown
  function toggleDropdown(index: number) {
    initDropdownState(index);
    searchableDropdowns[index].isOpen = !searchableDropdowns[index].isOpen;
    if (searchableDropdowns[index].isOpen) {
      filterActions(index, searchableDropdowns[index].searchTerm);
    }
    searchableDropdowns = { ...searchableDropdowns }; // Trigger reactivity
  }
  
  // Select action
  function selectAction(index: number, actionId: string, actionName: string) {
    searchableDropdowns[index].searchTerm = actionName;
    searchableDropdowns[index].isOpen = false;
    searchableDropdowns[index].userIsTyping = false; // User finished selecting
    searchableDropdowns = { ...searchableDropdowns }; // Trigger reactivity
    updateActionType(index, actionId);
  }
  
  // Close dropdown when clicking outside
  function closeDropdown(index: number) {
    if (searchableDropdowns[index]) {
      searchableDropdowns[index].isOpen = false;
      searchableDropdowns[index].userIsTyping = false; // Reset typing flag when closing
      searchableDropdowns = { ...searchableDropdowns }; // Trigger reactivity
    }
  }
  
  // Add a new action
  async function addAction() {
    try {
      await addChefAction(stepIndex);
    } catch (error) {
      console.error('ChefStep: Error in addAction:', error);
    }
  }
  
  // Remove an action
  async function removeAction(index: number) {
    // Reset dropdown states
    searchableDropdowns = {};
    
    await removeChefAction(stepIndex, index);
    
    // Regenerate previews after removal
      regenerateAllPreviews();
  }
  
  // Update action type
  async function updateActionType(index: number, actionType: string) {
    try {
      await updateChefAction(stepIndex, index, { 
          actionType,
          preview: '' // Reset preview when action type changes
      });

      // Update the search term to show the selected action name
      if (searchableDropdowns[index] && actionType && availableActions.length > 0) {
        const actionName = availableActions.find(a => a.id === actionType)?.name || '';
        if (actionName) {
          searchableDropdowns[index].searchTerm = actionName;
          searchableDropdowns[index].userIsTyping = false; // This is a programmatic update
          searchableDropdowns = { ...searchableDropdowns }; // Trigger reactivity
        }
      }
        
      // Regenerate all previews sequentially since changing one action affects all subsequent ones
      if (actionType) {
        regenerateAllPreviews();
      }
    } catch (error) {
      console.error('ChefStep: Error in updateActionType:', error);
    }
  }
  
  // Update input variable
  async function updateInputVariable(variable: string) {
    await updateChefStep(stepIndex, { inputVariable: variable });
    
    // Regenerate previews for all actions sequentially
    regenerateAllPreviews();
  }
  
  // Regenerate all previews sequentially using backend logic
  async function regenerateAllPreviews() {
    const currentActions = Array.isArray(chefStep?.actions) ? chefStep.actions : [];
    
    if (currentActions.length === 0) {
      return;
    }
    
    try {
      // Get the actual input value from available variables
      const inputVariableName = chefStep?.inputVariable;
      
      if (!inputVariableName) {
        return;
      }
      
      // Find the actual value of the input variable
      const inputValue = variableValues[inputVariableName] || 
                        (availableVariables.includes(inputVariableName) 
                          ? `{{${inputVariableName}}}` // Use variable placeholder for preview
                          : "No variable value available");
      
      // Convert actions to proper chef.ChefAction objects for backend
      const chefActions = currentActions.map(action => chef.ChefAction.createFrom({
        id: action.id,
        actionType: action.actionType || '',
        options: action.options || {},
        preview: action.preview || ''
      }));
      
      // Use backend function to get sequential previews with actual input value
      const previews = await GetChefStepSequentialPreview(chefActions, inputValue);
      
      // Apply previews to actions via store
      for (let i = 0; i < Math.min(previews.length, currentActions.length); i++) {
        await updateChefAction(stepIndex, i, { preview: previews[i] });
      }
    } catch (error) {
      console.error('ChefStep: Failed to regenerate previews:', error);
    }
  }
  
  // Update output variable
  async function updateOutputVariable(variable: string) {
    await updateChefStep(stepIndex, { outputVariable: variable });
  }
  
  // Update step name
  function updateName(name: string) {
    dispatch('update', { name });
  }
  
  // Execute step
  function executeStep() {
    dispatch('execute');
  }
  
  // Handle clicks outside dropdowns to close them
  function handleGlobalClick(event: MouseEvent) {
    const target = event.target as Element;
    
    // Check if click is outside any dropdown
    Object.keys(searchableDropdowns).forEach(indexStr => {
      const index = parseInt(indexStr);
      if (searchableDropdowns[index]?.isOpen) {
        // Find the dropdown container
        const dropdownContainer = target.closest(`[data-dropdown-index="${index}"]`);
        if (!dropdownContainer) {
          closeDropdown(index);
        }
      }
    });
  }
</script>

<div class="flex flex-col h-full p-4 space-y-4">
  {#if !isExpanded}
    <!-- Collapsed view -->
    <div class="flex flex-col space-y-2">
      <div class="text-sm font-medium text-gray-100">Chef</div>
      <div class="text-xs text-gray-50">
        Input: <span class="text-blue-300">{chefStep?.inputVariable || 'Not set'}</span>
      </div>
      <div class="text-xs text-gray-50">
        Actions: {chefStep?.actions?.length || 0}
      </div>
      <div class="text-xs text-gray-50">
        Output: <span class="text-green-300">{chefStep?.outputVariable || 'Not set'}</span>
      </div>
      
      <!-- Chef result -->
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
    <!-- Input/Output Variables -->
    <div class="flex items-center space-x-2">
      <!-- Input Variable Selection -->
      <select
        class="flex-1 px-3 py-1 bg-gray-800 border border-gray-600 rounded text-gray-100 text-sm focus:outline-none focus:border-blue-400"
        value={chefStep?.inputVariable || ''}
        on:change={(e) => updateInputVariable(e.currentTarget.value)}
      >
        <option value="">Select input variable...</option>
        {#each safeAvailableVariables as variable}
          <option value={variable}>{variable}</option>
        {/each}
      </select>
      
      <!-- Arrow -->
      <svg class="w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 8l4 4m0 0l-4 4m4-4H3"></path>
      </svg>
      
      <!-- Output Variable -->
      <input
        type="text"
        class="flex-1 px-3 py-1 bg-gray-800 border border-gray-600 rounded text-gray-100 text-sm focus:outline-none focus:border-blue-400"
        value={chefStep?.outputVariable || ''}
        on:input={(e) => updateOutputVariable(e.currentTarget.value)}
        placeholder="Output variable name"
      />
    </div>

    <!-- Actions List -->
    <div class="flex-1 flex flex-col space-y-2 min-h-0">
      <div class="flex-1 overflow-y-auto space-y-2 min-h-0 pr-4">
        {#each safeActions as action, index}
          
          <div class="border border-gray-600 rounded p-3 bg-gray-800/50">
            <div class="flex items-center justify-between mb-2">
              <!-- Custom Searchable Dropdown -->
              <div class="flex-1 relative" data-dropdown-index={index}>
                {#if searchableDropdowns[index]}
                  
                  <div class="flex">
                    <input
                      type="text"
                      class="flex-1 px-2 py-1 bg-gray-700 border border-gray-600 rounded-l text-gray-100 text-sm focus:outline-none focus:border-blue-400"
                      placeholder="Search actions..."
                      bind:value={searchableDropdowns[index].searchTerm}
                      on:input={(e) => handleSearchInput(index, e.currentTarget.value)}
                      on:focus={() => handleFocus(index)}
                    />
                    <button
                      type="button"
                      class="px-2 py-1 bg-gray-700 border border-l-0 border-gray-600 rounded-r text-gray-100 text-sm hover:bg-gray-600 focus:outline-none focus:border-blue-400"
                      aria-label="Toggle actions dropdown"
                      on:click={() => toggleDropdown(index)}
                    >
                      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
                      </svg>
                    </button>
                  </div>
                  
                  <!-- Dropdown List -->
                  {#if searchableDropdowns[index].isOpen}
                    <div class="absolute z-10 w-full mt-1 bg-gray-700 border border-gray-600 rounded shadow-lg max-h-48 overflow-y-auto">
                      {#if searchableDropdowns[index].filteredActions.length === 0}
                        <div class="px-3 py-2 text-gray-400 text-sm">No actions found</div>
                      {:else}
                        {#each searchableDropdowns[index].filteredActions as availableAction}
                          <button
                            type="button"
                            class="w-full px-3 py-2 text-left text-gray-100 text-sm hover:bg-gray-600 focus:outline-none focus:bg-gray-600"
                            on:click={() => selectAction(index, availableAction.id, availableAction.name)}
                          >
                            <div class="font-medium">{availableAction.name}</div>
                            {#if availableAction.description}
                              <div class="text-xs text-gray-400 mt-1">{availableAction.description}</div>
                            {/if}
                          </button>
                        {/each}
                      {/if}
                    </div>
                  {/if}
                {/if}
              </div>
              <button
                class="ml-2 px-2 py-1 bg-red-600 hover:bg-red-700 text-white rounded text-sm"
                on:click={() => removeAction(index)}
              >
                Remove
              </button>
            </div>
            
            {#if action.preview}
              <div class="mt-2">
                <h3 class="text-xs text-gray-400">
                  Preview:
                </h3>
                <div class="mt-1 p-2 bg-gray-900 rounded text-xs text-gray-300 font-mono max-h-20 overflow-y-auto">
                  {action.preview}
                </div>
              </div>
            {/if}
          </div>
        {/each}
        
        <!-- Add Action Button at bottom -->
        <div class="flex justify-center pt-2">
          <button
            class="w-full border border-gray-600 rounded p-3 bg-gray-800/50 hover:bg-gray-700/50 transition-colors duration-200 flex items-center justify-center space-x-2 text-gray-300 hover:text-gray-100"
            on:click={addAction}
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
            </svg>
            <span class="text-sm font-medium">Add Action</span>
          </button>
        </div>
      </div>
    </div>

    <!-- Execution Result -->
    {#if executionResult}
      <div class="border-t border-gray-600 pt-4">
        <div class="flex items-center space-x-2 mb-2">
          <span class="text-sm font-medium text-gray-100">Result:</span>
          <span class={`px-2 py-1 text-xs rounded ${executionResult.success ? 'bg-green-500/30 text-green-300' : 'bg-red-500/30 text-red-300'}`}>
            {executionResult.success ? 'Success' : 'Failed'}
          </span>
          {#if executionResult.executionTime !== undefined}
            <span class="text-xs text-gray-400">{executionResult.executionTime}ms</span>
          {/if}
        </div>
        
        {#if !executionResult.success && executionResult.errorMessage}
          <div class="text-xs text-red-400 bg-red-900/20 p-2 rounded">
            {executionResult.errorMessage}
          </div>
        {/if}
        
        {#if executionResult.success && executionResult.variables}
          <div class="text-xs text-gray-300">
            <div class="font-medium mb-1">Variables set:</div>
            {#each Object.entries(executionResult.variables) as [name, value]}
              <div class="ml-2">
                <span class="text-blue-300">{name}</span> = 
                <span class="text-gray-100">{value}</span>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
  {/if}
</div> 