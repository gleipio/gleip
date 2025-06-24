<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte';
  import { GetRequestMethod, GetRequestPath } from '../../../../wailsjs/go/network/HTTPHelper';
  import MonacoEditor from '../../../components/monaco/MonacoEditor.svelte';
  
  export let phantomRequest: any;
  export let index: number;
  
  const dispatch = createEventDispatcher();
  
  function addPhantomRequest() {
    dispatch('addPhantomRequest', { phantomRequest, index });
  }

  // Reactive variables for header display only
  let requestMethod: string = '';
  let requestPath: string = '';

  // Load minimal request data for header display
  $: if (phantomRequest) {
    loadRequestData();
  }

  async function loadRequestData() {
    if (phantomRequest) {
      try {
        requestMethod = await GetRequestMethod(phantomRequest);
        requestPath = await GetRequestPath(phantomRequest);
      } catch (error) {
        console.error('Failed to parse phantom request:', error);
        // Fallback values
        requestMethod = 'GET';
        requestPath = '/';
      }
    }
  }
</script>

<div class="bg-gray-800/50 border border-gray-600/50 rounded-lg p-3 mb-2 hover:bg-gray-700/50 transition-colors">
  <!-- Header with method and URL -->
  <div class="flex items-center justify-between mb-2">
    <div class="flex items-center space-x-2 min-w-0 flex-1">
      <span class="http-method text-sm font-bold px-2 py-1 rounded bg-blue-600/20 flex-shrink-0">
        {requestMethod}
      </span>
      <span class="http-path text-sm text-blue-300 truncate min-w-0">
        {requestPath}
      </span>
    </div>
    <button
      class="px-2 py-1 bg-green-600 hover:bg-green-700 text-white text-xs rounded transition-colors flex-shrink-0 ml-2"
      on:click={addPhantomRequest}
    >
      Add
    </button>
  </div>
  
  <!-- Raw request preview in Monaco editor -->
  <div class="mt-2" style="height: 15vh;">
    <MonacoEditor
      value={phantomRequest?.dump || ''}
      language="http"
      readOnly={true}
      fontSize={11}
    />
  </div>
</div> 