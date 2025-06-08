<script lang="ts">
  import { onMount } from 'svelte';
  import { GetInterceptedRequests, SetInterceptEnabled, ModifyInterceptedRequest, ModifyInterceptedResponse, ForwardRequestAndWaitForResponse, ForwardRequestImmediately } from '../../../wailsjs/go/backend/App';
  import { network } from '../../../wailsjs/go/models';
  import { getMethodColor } from '../../shared/utils/httpColors';
  import MonacoEditor from '../../components/monaco/MonacoEditor.svelte';
  import { interceptEnabled, updateInterceptState } from './store/interceptStore';
  import { GetRequestMethod, GetRequestURL } from '../../../wailsjs/go/network/HTTPHelper';

  type HTTPTransaction = network.HTTPTransaction;
  
  // State
  let requests: HTTPTransaction[] = [];
  let selectedRequest: HTTPTransaction | null = null;
  let editedRequest: any = null;
  let loadRequestsInterval: number;
  let requestContent = '';
  let responseContent = '';
  let isWaitingForResponse = false;
  
  // Local variable that syncs with the store for binding
  let localInterceptEnabled = false;
  
  // Sync local variable with store
  $: localInterceptEnabled = $interceptEnabled;
  $: if (localInterceptEnabled !== $interceptEnabled) {
    updateInterceptState(localInterceptEnabled);
  }
  
  // Load intercepted requests
  async function loadRequests() {
    try {
      // Only load requests if interception is enabled
      if ($interceptEnabled) {
        const newRequests = await GetInterceptedRequests();
        
        // Sort by timestamp to maintain consistent order
        newRequests.sort((a, b) => {
          const timeA = new Date(a.timestamp).getTime();
          const timeB = new Date(b.timestamp).getTime();
          
          // Primary sort by timestamp
          if (timeA !== timeB) {
            return timeA - timeB;
          }
          
          // Secondary sort by ID for stable sorting when timestamps are equal
          return a.id.localeCompare(b.id);
        });
        
        // Check if the selected request was updated with a response
        if (selectedRequest && isWaitingForResponse) {
          const updatedRequest = newRequests.find(req => req.id === selectedRequest!.id);
          if (updatedRequest && updatedRequest.response && !selectedRequest.response) {
            // The selected request now has a response, update the UI
            selectedRequest = updatedRequest;
            editedRequest = JSON.parse(JSON.stringify(updatedRequest));
            responseContent = updatedRequest.response.dump || '';
            isWaitingForResponse = false;
            console.log('Response received for intercepted request:', updatedRequest.id);
          }
        }
        
        requests = newRequests;
      } else {
        // Clear the list when interception is disabled
        requests = [];
        selectedRequest = null;
        editedRequest = null;
      }
    } catch (error) {
      console.error('Failed to load intercepted requests:', error);
    }
  }
  
  // Toggle interception
  async function handleInterceptToggle(event: Event) {
    const checked = (event.target as HTMLInputElement).checked;
    try {
      await SetInterceptEnabled(checked);
      updateInterceptState(checked);
      
      // If disabling interception, clear the requests list immediately
      if (!checked) {
        requests = [];
        selectedRequest = null;
        editedRequest = null;
      }
    } catch (error) {
      console.error('Failed to toggle interception:', error);
    }
  }
  
  // Modify request and forward it
  async function handleModifyRequest() {
    if (!selectedRequest) return;
    
    try {
      if (selectedRequest.response) {
        // We have a response, so modify it
        const lines = responseContent.split('\n');
        const statusLine = lines[0] || '';
        const statusMatch = statusLine.match(/HTTP\/\d\.\d\s+(\d+)\s+(.+)/);
        const statusCode = statusMatch ? parseInt(statusMatch[1]) : 200;
        const status = statusMatch ? statusMatch[2] : 'OK';
        
        await ModifyInterceptedResponse(
          selectedRequest.id,
          responseContent
        );
      } else {
        // No response yet, so forward the request immediately
        const lines = requestContent.split('\n');
        const requestLine = lines[0] || '';
        const requestMatch = requestLine.match(/(\w+)\s+(.+?)\s+HTTP/);
        const method = requestMatch ? requestMatch[1] : (await GetRequestMethod(selectedRequest.request));
        let url = requestMatch ? requestMatch[2] : (await GetRequestURL(selectedRequest.request));
        
        // If URL is relative, construct full URL from Host header
        if (url && !url.startsWith('http')) {
          const hostLine = lines.find(line => line.toLowerCase().startsWith('host:'));
          if (hostLine) {
            const host = hostLine.split(':')[1]?.trim();
            if (host) {
              // Determine if it should be https or http based on common patterns
              const isHttps = host.includes('443') || host.includes('ssl') || !host.includes(':');
              url = `${isHttps ? 'https' : 'http'}://${host}${url.startsWith('/') ? url : '/' + url}`;
            }
          }
        }
        
        await ForwardRequestImmediately(
          selectedRequest.id,
          method,
          url,
          {},
          requestContent
        );
      }
      
      // Remove the forwarded request from the list immediately
      requests = requests.filter(req => req.id !== selectedRequest?.id);
      selectedRequest = null;
      editedRequest = null;
      requestContent = '';
      responseContent = '';
      isWaitingForResponse = false;
    } catch (error) {
      console.error('Failed to modify request:', error);
    }
  }
  
  // Select a request to edit
  function selectRequest(req: HTTPTransaction) {
    selectedRequest = req;
    editedRequest = JSON.parse(JSON.stringify(req));
    requestContent = req.request.dump || '';
    responseContent = req.response?.dump || '';
    isWaitingForResponse = req.waitingForResponse || false;
  }

  // Forward request and wait for response
  async function handleForwardAndWaitForResponse() {
    if (!selectedRequest) return;
    
    try {
      isWaitingForResponse = true;
      
      // Parse request content to extract method and URL
      const lines = requestContent.split('\n');
      const requestLine = lines[0] || '';
      const requestMatch = requestLine.match(/(\w+)\s+(.+?)\s+HTTP/);
      const method = requestMatch ? requestMatch[1] : (await GetRequestMethod(selectedRequest.request));
      let url = requestMatch ? requestMatch[2] : (await GetRequestURL(selectedRequest.request) || '/');
      
      // If URL is relative, construct full URL from Host header
      if (url && !url.startsWith('http')) {
        const hostLine = lines.find(line => line.toLowerCase().startsWith('host:'));
        if (hostLine) {
          const host = hostLine.split(':')[1]?.trim();
          if (host) {
            // Determine if it should be https or http based on common patterns
            const isHttps = host.includes('443') || host.includes('ssl') || !host.includes(':');
            url = `${isHttps ? 'https' : 'http'}://${host}${url.startsWith('/') ? url : '/' + url}`;
          }
        }
      }
      
      await ForwardRequestAndWaitForResponse(
        selectedRequest.id,
        method,
        url,
        {},
        requestContent
      );
      // The request will remain in the list but will be updated when response arrives
    } catch (error) {
      console.error('Failed to forward request and wait for response:', error);
      isWaitingForResponse = false;
    }
  }
  

  
  onMount(() => {
    // Load requests immediately on mount
    loadRequests();
    
    // Set up polling
    loadRequestsInterval = window.setInterval(loadRequests, 500);
    
    return () => {
      if (loadRequestsInterval) {
        clearInterval(loadRequestsInterval);
      }
    };
  });
</script>

<div class="grid grid-cols-2 h-full">
  <!-- Left column - Request list -->
  <div class="border-r border-gray-700/50">
    <div class="h-14 border-b border-gray-700/50 bg-gray-800/30 px-4 grid grid-cols-[1fr,auto] items-center">
      <h2 class="text-lg font-medium text-gray-100">Intercepted Requests</h2>
      <label class="flex items-center cursor-pointer">
        <div class="relative">
          <input
            type="checkbox"
            class="sr-only peer"
            bind:checked={localInterceptEnabled}
            on:change={handleInterceptToggle}
          />
          <div class="w-11 h-6 bg-red-900 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-gray-300 after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-green-600"></div>
        </div>
        <span class="ml-3 text-sm text-gray-100">Enable Interception</span>
      </label>
    </div>

    <div class="overflow-auto h-[calc(100%-3.5rem)]">
      <table class="w-full border-collapse">
        <thead class="sticky top-0 bg-gray-800/50 backdrop-blur">
          <tr>
            <th class="text-left p-3 text-xs font-medium text-gray-50 uppercase w-24">Method</th>
            <th class="text-left p-3 text-xs font-medium text-gray-50 uppercase w-24">Type</th>
            <th class="text-left p-3 text-xs font-medium text-gray-50 uppercase">URL</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-700/50">
          {#each requests as req}
            <tr
              on:click={() => selectRequest(req)}
              class={`cursor-pointer ${
                selectedRequest?.id === req.id 
                  ? 'bg-gray-700/50' 
                  : 'hover:bg-gray-800/50'
              }`}
            >
              <td class="p-3">
                {#await GetRequestMethod(req.request) then method}
                  <span class={`px-2 py-1 text-xs font-medium rounded ${getMethodColor(method)}`}>
                    {method}
                  </span>
                {/await}
              </td>
              <td class="p-3">
                <span class={`text-xs font-medium ${
                  req.response ? 'text-orange-400' : 'text-blue-400'
                }`}>
                  {req.response ? 'Response' : 'Request'}
                </span>
              </td>
              <td class="p-3">
                <div class="text-sm text-gray-100 truncate max-w-[400px]">
                  {#await GetRequestURL(req.request) then url}
                    {url}
                  {/await}
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>

  <!-- Right column - Details -->
  <div class="flex flex-col">
    <div class="h-14 border-b border-gray-700/50 bg-gray-800/30 px-4 flex items-center justify-between">
      <h2 class="text-lg font-medium text-gray-100">Details</h2>
      {#if selectedRequest}
        <div class="flex gap-2">
          {#if !selectedRequest.response && !isWaitingForResponse}
            <button 
              on:click={handleForwardAndWaitForResponse}
              class="px-3 py-1 text-sm bg-blue-700 text-gray-50 rounded hover:bg-blue-600"
            >
              Forward & Intercept Response
            </button>
          {/if}
          <button 
            on:click={handleModifyRequest}
            class="px-3 py-1 text-sm bg-gray-700 text-gray-50 rounded hover:bg-gray-600"
          >
            Forward
          </button>
          <button 
            class="px-3 py-1 text-sm bg-gray-700 text-gray-50 rounded hover:bg-gray-600"
            on:click={() => {
              selectedRequest = null;
              editedRequest = null;
              requestContent = '';
              responseContent = '';
              isWaitingForResponse = false;
            }}
          >
            Cancel
          </button>
        </div>
      {/if}
    </div>

    <div class="flex-1 flex flex-col overflow-hidden">
      {#if selectedRequest}
        <!-- Request Editor -->
        <div class="flex-1 flex flex-col border-b border-gray-700/50">
          <div class="h-8 bg-gray-800/30 px-4 flex items-center border-b border-gray-700/50">
            <h3 class="text-sm font-medium text-gray-100">Request</h3>
          </div>
          <div class="flex-1">
            <MonacoEditor
              bind:value={requestContent}
              language="http"
              readOnly={false}
              fontSize={12}
              on:change={(e) => {
                requestContent = e.detail.value;
              }}
            />
          </div>
        </div>

        <!-- Response Editor -->
        <div class="flex-1 flex flex-col">
          <div class="h-8 bg-gray-800/30 px-4 flex items-center border-b border-gray-700/50">
            <h3 class="text-sm font-medium text-gray-100">
              Response
              {#if isWaitingForResponse}
                <span class="text-yellow-400 ml-2">(Waiting...)</span>
              {/if}
            </h3>
          </div>
          <div class="flex-1">
            <MonacoEditor
              bind:value={responseContent}
              language="http"
              readOnly={!selectedRequest.response}
              fontSize={12}
              on:change={(e) => {
                responseContent = e.detail.value;
              }}
            />
          </div>
        </div>
      {:else}
        <div class="flex-1 flex items-center justify-center text-gray-50">
          Select a request to view details
        </div>
      {/if}
    </div>
  </div>
</div> 