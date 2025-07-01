<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  
  export let variables: Record<string, string> = {};
  export let isExpanded: boolean = false;
  
  // New variable being added
  let newVariableName = '';
  let newVariableValue = '';
  
  const dispatch = createEventDispatcher();
  
  // Add a new variable
  function addVariable() {
    if (newVariableName.trim() === '') return;
    
    // Check if variable with the same name already exists
    if (Object.keys(variables).includes(newVariableName.trim())) {
      dispatch('notification', { message: `Variable '${newVariableName.trim()}' already exists`, type: 'error' });
      return;
    }
    
    // Add the new variable
    const updatedVariables = {
      ...variables,
      [newVariableName.trim()]: newVariableValue.trim()
    };
    
    // Reset input fields
    newVariableName = '';
    newVariableValue = '';
    
    // Dispatch event to update store
    dispatch('update', { variables: updatedVariables });
  }
  
  // Remove a variable
  function removeVariable(variableName: string) {
    const updatedVariables = { ...variables };
    delete updatedVariables[variableName];
    dispatch('update', { variables: updatedVariables });
  }
  
  // Update a variable name
  function updateVariableName(oldName: string, newName: string) {
    if (oldName === newName) return;
    
    if (Object.keys(variables).includes(newName.trim())) {
      dispatch('notification', { message: `Variable '${newName.trim()}' already exists`, type: 'error' });
      return;
    }
    
    const updatedVariables = { ...variables };
    updatedVariables[newName.trim()] = updatedVariables[oldName];
    delete updatedVariables[oldName];
    
    dispatch('update', { variables: updatedVariables });
  }
  
  // Update a variable value
  function updateVariableValue(name: string, value: string) {
    const updatedVariables = { ...variables };
    updatedVariables[name] = value.trim();
    
    dispatch('update', { variables: updatedVariables });
  }
</script>

<div class="w-full">
  {#if !isExpanded}
    <!-- Collapsed view -->
    <div class="flex flex-col space-y-2">
      <div class="text-sm font-medium text-gray-100">Variables</div>
      {#if Object.keys(variables).length > 0}
        <div class="text-xs text-gray-50 mb-1">
          {Object.keys(variables).length} variable{Object.keys(variables).length !== 1 ? 's' : ''} defined
        </div>
        <div class="flex flex-wrap gap-1 overflow-hidden">
          {#each Object.entries(variables) as [name, value]}
            <div class="bg-[var(--color-midnight-darker)] border border-[var(--color-table-border)] rounded px-2 py-1 text-xs max-w-full min-w-0">
              <div class="break-all line-clamp-5" style="padding-left: .8rem; text-indent: -.8rem;">
                <span class="text-blue-300 font-medium">{name}</span><span class="text-gray-400">=</span><span class="text-green-300">{value}</span>
              </div>
            </div>
          {/each}
        </div>
      {:else}
        <div class="text-xs text-gray-500">No variables defined</div>
      {/if}
    </div>
  {:else}
    <!-- Expanded view -->
    <div class="flex flex-col gap-4">
      <!-- Add new variable form -->
      <div class="border-b border-gray-700 pb-4">
        <div class="text-sm font-medium text-gray-50 mb-2">Add New Variable</div>
        <div class="grid grid-cols-5 gap-2">
          <div class="col-span-2">
            <input
              type="text"
              class="w-full bg-[var(--color-midnight-darker)] border border-[var(--color-table-border)] text-white px-3 py-2 rounded text-sm"
              placeholder="Name"
              bind:value={newVariableName}
            />
          </div>
          <div class="col-span-2">
            <input
              type="text"
              class="w-full bg-[var(--color-midnight-darker)] border border-[var(--color-table-border)] text-white px-3 py-2 rounded text-sm"
              placeholder="Value"
              bind:value={newVariableValue}
            />
          </div>
          <div class="col-span-1">
            <button
              class="w-full px-3 py-2 bg-[var(--color-midnight-accent)] hover:bg-[var(--color-midnight-accent)]/80 text-[var(--color-button-text)] rounded text-sm"
              on:click={addVariable}
            >
              Add Variable
            </button>
          </div>
        </div>
      </div>
      
      <!-- Variables list -->
      <div>
        <div class="text-sm font-medium text-gray-50 mb-2">Variables</div>
        {#if Object.keys(variables).length === 0}
          <div class="text-gray-500 text-sm p-4 bg-[var(--color-midnight-darker)] rounded">
            No variables defined. Add some using the form above.
          </div>
        {:else}
          <div class="bg-[var(--color-midnight-darker)] rounded overflow-hidden">
            <table class="w-full border-collapse">
              <thead>
                <tr class="bg-[var(--color-midnight-darker)] text-gray-50 text-xs">
                  <th class="py-2 px-3 text-left border-b border-gray-700 w-1/4">NAME</th>
                  <th class="py-2 px-3 text-left border-b border-gray-700 w-1/2">VALUE</th>
                  <th class="py-2 px-3 text-right border-b border-gray-700 w-1/12">ACTION</th>
                </tr>
              </thead>
              <tbody>
                {#each Object.entries(variables) as [name, value]}
                  <tr class="border-b border-gray-700/40 hover:bg-[var(--color-table-row-hover)]">
                    <td class="py-2 px-2 w-1/4">
                      <input
                        type="text"
                        class="w-full bg-transparent border border-transparent hover:border-gray-700 focus:border-blue-500 text-white px-2 py-1 rounded text-sm"
                        value={name}
                        on:change={(e) => {
                          const target = e.currentTarget;
                          if (target instanceof HTMLInputElement) {
                            updateVariableName(name, target.value);
                          }
                        }}
                      />
                    </td>
                    <td class="py-2 px-2 w-1/2">
                      <input
                        type="text"
                        class="w-full bg-transparent border border-transparent hover:border-gray-700 focus:border-blue-500 text-white px-2 py-1 rounded text-sm"
                        value={value}
                        on:change={(e) => {
                          const target = e.currentTarget;
                          if (target instanceof HTMLInputElement) {
                            updateVariableValue(name, target.value);
                          }
                        }}
                      />
                    </td>
                    <td class="py-2 pl-3 text-center w-[1%]">
                      <button
                        class="text-red-400 hover:text-red-300"
                        on:click={() => removeVariable(name)}
                        title="Remove variable"
                      >
                        ×
                      </button>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
      
      <!-- Variable Usage Instructions -->
      <div class="mt-4 text-sm text-gray-50 bg-[var(--color-midnight-darker)] rounded p-3">
        <div class="font-medium mb-1">How to use variables:</div>
        <p>Use <code class="px-1 py-0.5 bg-gray-700 text-blue-300 rounded">{"{{variableName}}"}</code> syntax in your requests to reference variables.</p>
        <div class="mt-2">
          <div class="mb-1">Examples:</div>
          <ul class="list-disc pl-5 text-xs space-y-1">
            <li>URL: <code class="px-1 py-0.5 bg-gray-700 text-xs">/api/v1/users/{"{{userId}}"}</code></li>
            <li>Header: <code class="px-1 py-0.5 bg-gray-700 text-xs">Authorization: Bearer {"{{token}}"}</code></li>
            <li>Body: <code class="px-1 py-0.5 bg-gray-700 text-xs">"username": "{"{{username}}"}"</code></li>
          </ul>
        </div>
      </div>
    </div>
  {/if}
</div> 