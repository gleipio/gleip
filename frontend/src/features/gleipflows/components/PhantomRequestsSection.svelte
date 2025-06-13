<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { GetPhantomRequests } from '../../../../wailsjs/go/backend/App';
  import PhantomRequestCard from './PhantomRequestCard.svelte';
  
  export let gleipFlowId: string;
  export let lastRequestInFlow: any = null;
  
  const dispatch = createEventDispatcher();
  
  let phantomRequests: any[] = [];
  let isLoading = false;
  let error = '';
  
  // Load phantom requests when component mounts or when lastRequestInFlow changes
  $: if (gleipFlowId && lastRequestInFlow) {
    loadPhantomRequests();
  }
  
  async function loadPhantomRequests() {
    if (!lastRequestInFlow) {
      phantomRequests = [];
      return;
    }
    
    isLoading = true;
    error = '';
    
    try {
      const result = await GetPhantomRequests(gleipFlowId, lastRequestInFlow);
      phantomRequests = result || [];
    } catch (err) {
      console.error('Failed to load phantom requests:', err);
      error = 'Failed to load suggestions';
      phantomRequests = [];
    } finally {
      isLoading = false;
    }
  }
  
  function handleAddPhantomRequest(event: CustomEvent) {
    dispatch('addPhantomRequest', event.detail);
  }
</script>

<div class="flex-shrink-0 w-80 bg-gray-900/30 border border-gray-600/30 rounded-lg p-4">
  <div class="flex items-center justify-between mb-3">
    <h3 class="text-sm font-medium text-gray-300">Suggested Requests</h3>
    {#if !isLoading && phantomRequests.length > 0}
      <button
        class="text-xs text-gray-500 hover:text-gray-300 transition-colors"
        on:click={loadPhantomRequests}
      >
        Refresh
      </button>
    {/if}
  </div>
  
  <div class="space-y-0">
    {#if isLoading}
      <div class="flex items-center justify-center py-8 text-gray-500">
        <div class="animate-spin w-4 h-4 border-2 border-gray-400 border-t-transparent rounded-full mr-2"></div>
        Loading suggestions...
      </div>
    {:else if error}
      <div class="text-center py-8 text-red-400 text-sm">
        {error}
      </div>
    {:else if phantomRequests.length === 0}
      <div class="text-center py-8 text-gray-500 text-sm">
        {lastRequestInFlow ? 'No suggestions available' : 'Add a request to see suggestions'}
      </div>
    {:else}
      {#each phantomRequests as phantomRequest, index}
        <PhantomRequestCard
          {phantomRequest}
          {index}
          on:addPhantomRequest={handleAddPhantomRequest}
        />
      {/each}
    {/if}
  </div>
</div> 