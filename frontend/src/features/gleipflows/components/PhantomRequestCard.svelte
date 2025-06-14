<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte';
  import { GetRequestMethod, GetRequestURL, GetRequestHeaders, GetRequestBody, GetRequestPath } from '../../../../wailsjs/go/network/HTTPHelper';
  
  export let phantomRequest: any;
  export let index: number;
  
  const dispatch = createEventDispatcher();
  
  function addPhantomRequest() {
    dispatch('addPhantomRequest', { phantomRequest, index });
  }

  // Reactive variables for parsed request data
  let requestMethod: string = '';
  let requestURL: string = '';
  let requestPath: string = '';
  let requestHeaders: Record<string, string> = {};
  let requestBody: string = '';

  // Load request data when phantomRequest changes
  $: if (phantomRequest) {
    loadRequestData();
  }

  async function loadRequestData() {
    if (phantomRequest) {
      try {
        requestMethod = await GetRequestMethod(phantomRequest);
        requestURL = await GetRequestURL(phantomRequest);
        requestPath = await GetRequestPath(phantomRequest);
        requestHeaders = await GetRequestHeaders(phantomRequest);
        requestBody = await GetRequestBody(phantomRequest);
      } catch (error) {
        console.error('Failed to parse phantom request:', error);
        // Fallback values
        requestMethod = 'GET';
        requestURL = '/';
        requestHeaders = {};
        requestBody = '';
      }
    }
  }
</script>

<div class="bg-gray-800/50 border border-gray-600/50 rounded-lg p-3 mb-2 hover:bg-gray-700/50 transition-colors">
  <!-- Header with method and URL -->
  <div class="flex items-center justify-between mb-2">
    <div class="flex items-center space-x-2">
      <span class="http-method text-sm font-bold px-2 py-1 rounded bg-blue-600/20">
        {requestMethod}
      </span>
      <span class="http-path text-sm text-blue-300 truncate">
        {requestPath}
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
    {#if requestHeaders && Object.keys(requestHeaders).length > 0}
      <div class="mb-1">
        {#each Object.entries(requestHeaders).slice(0, 2) as [key, value]}
          <div class="truncate">
            <span class="http-header-key">{key}:</span>
            <span class="http-header-value">{value}</span>
          </div>
        {/each}
        {#if Object.keys(requestHeaders).length > 2}
          <div class="text-gray-500">... +{Object.keys(requestHeaders).length - 2} more headers</div>
        {/if}
      </div>
    {/if}
    
    {#if requestBody}
      <div class="text-gray-300 truncate">
        Body: {requestBody.substring(0, 50)}{requestBody.length > 50 ? '...' : ''}
      </div>
    {/if}
  </div>
</div> 