<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  
  export let phantomRequest: any;
  export let index: number;
  
  const dispatch = createEventDispatcher();
  
  function addPhantomRequest() {
    dispatch('addPhantomRequest', { phantomRequest, index });
  }
</script>

<div class="bg-gray-800/50 border border-gray-600/50 rounded-lg p-3 mb-2 hover:bg-gray-700/50 transition-colors">
  <!-- Header with method and URL -->
  <div class="flex items-center justify-between mb-2">
    <div class="flex items-center space-x-2">
      <span class="http-method text-sm font-bold px-2 py-1 rounded bg-blue-600/20">
        {phantomRequest.method || 'GET'}
      </span>
      <span class="http-path text-sm text-blue-300 truncate">
        {phantomRequest.url || phantomRequest.path || '/'}
      </span>
    </div>
    <button
      class="px-2 py-1 bg-green-600 hover:bg-green-700 text-white text-xs rounded transition-colors"
      on:click={addPhantomRequest}
    >
      Add
    </button>
  </div>
  
  <!-- Preview of request content -->
  <div class="text-xs text-gray-400 font-mono">
    {#if phantomRequest.headers && Object.keys(phantomRequest.headers).length > 0}
      <div class="mb-1">
        {#each Object.entries(phantomRequest.headers).slice(0, 2) as [key, value]}
          <div class="truncate">
            <span class="http-header-key">{key}:</span>
            <span class="http-header-value">{value}</span>
          </div>
        {/each}
        {#if Object.keys(phantomRequest.headers).length > 2}
          <div class="text-gray-500">... +{Object.keys(phantomRequest.headers).length - 2} more headers</div>
        {/if}
      </div>
    {/if}
    
    {#if phantomRequest.body}
      <div class="text-gray-300 truncate">
        Body: {typeof phantomRequest.body === 'string' ? phantomRequest.body.substring(0, 50) + '...' : JSON.stringify(phantomRequest.body).substring(0, 50) + '...'}
      </div>
    {/if}
  </div>
</div> 