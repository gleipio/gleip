<script lang="ts">
  import { onMount } from 'svelte';
  import { GetInterceptedRequests, SetInterceptEnabled, ModifyInterceptedRequest, ModifyInterceptedResponse, ForwardRequestAndWaitForResponse, ForwardRequestImmediately, DropRequest, CopyRequestToClipboard, CopyRequestToSelectedFlow, ProcessInterceptedResponseForDisplay } from '../../../wailsjs/go/backend/App';
  import { network } from '../../../wailsjs/go/models';
  import { getMethodColor } from '../../shared/utils/httpColors';
  import MonacoEditor from '../../components/monaco/MonacoEditor.svelte';
  import BrowserButton from '../../components/ui/BrowserButton.svelte';
  import ContextMenu from '../../shared/components/ContextMenu.svelte';
  import Notification from '../../shared/components/Notification.svelte';
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
  
  // Context menu state
  let showContextMenu = false;
  let contextMenuX = 0;
  let contextMenuY = 0;
  let contextMenuRequest: HTTPTransaction | null = null;
  let showCopiedNotification = false;
  let notificationMessage = '';
  
  // UI state for resizing
  let splitPosition = 55; // Default 55% for top section (request list)
  let isDragging = false;
  let detailsSplitPosition = 50; // Default 50% for request panel
  let isDetailsDragging = false;
  let containerRef: HTMLDivElement;
  let detailsContainerRef: HTMLDivElement;
  
  // Sync local variable with store
  $: localInterceptEnabled = $interceptEnabled;
  $: if (localInterceptEnabled !== $interceptEnabled) {
    updateInterceptState(localInterceptEnabled);
  }

  // Auto-select the first request if none is selected and requests exist
  $: if (requests.length > 0 && !selectedRequest) {
    selectRequest(requests[0]);
  }
  
  // Load intercepted requests
  async function loadRequests() {
    try {
      // Only load requests if interception is enabled
      if ($interceptEnabled) {
        const newRequests = await GetInterceptedRequests();
        
        // Handle null/undefined response from backend
        if (!newRequests || !Array.isArray(newRequests)) {
          console.warn('GetInterceptedRequests returned invalid data:', newRequests);
          return;
        }
        
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
        
        // Check if the selected request still exists in the new requests list
        if (selectedRequest && !newRequests.find(req => req.id === selectedRequest!.id)) {
          // Selected request was removed (likely forwarded or dropped), clear selection
          selectedRequest = null;
          editedRequest = null;
          requestContent = '';
          responseContent = '';
          isWaitingForResponse = false;
          console.log('Selected request was removed from intercept queue');
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

  // Select next request after current one is removed
  function selectNextRequest(removedRequestId: string) {
    if (requests.length === 0) {
      selectedRequest = null;
      editedRequest = null;
      requestContent = '';
      responseContent = '';
      isWaitingForResponse = false;
      return;
    }

    const currentIndex = requests.findIndex(req => req.id === removedRequestId);
    if (currentIndex !== -1) {
      // Select next request (or first if this was the last)
      const nextIndex = currentIndex < requests.length - 1 ? currentIndex : 0;
      const nextRequest = requests[nextIndex];
      if (nextRequest) {
        selectRequest(nextRequest);
      }
    } else {
      // If we can't find the removed request, just select the first one
      selectRequest(requests[0]);
    }
  }
  
  // Modify request and forward it
  async function handleModifyRequest() {
    if (!selectedRequest) return;
    
    const requestId = selectedRequest.id;
    
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
        
        // Response modification removes the request from intercept queue
        // Clear UI state immediately
        selectedRequest = null;
        editedRequest = null;
        requestContent = '';
        responseContent = '';
        isWaitingForResponse = false;
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
        
        // Forward immediately removes the request from intercept queue transparently
        // Clear UI state immediately
        selectedRequest = null;
        editedRequest = null;
        requestContent = '';
        responseContent = '';
        isWaitingForResponse = false;
        
        // Also remove from the requests array immediately to prevent lingering
        requests = requests.filter(req => req.id !== requestId);
      }
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

  // Handle right-click on table rows
  function handleRowContextMenu(event: MouseEvent, req: HTTPTransaction) {
    event.preventDefault();
    contextMenuRequest = req;
    contextMenuX = event.clientX;
    contextMenuY = event.clientY;
    showContextMenu = true;
  }

  // Context menu functions
  function handleContextMenuClose() {
    showContextMenu = false;
    contextMenuRequest = null;
  }

  // Copy request to clipboard
  async function copyRequestToClipboard() {
    if (!contextMenuRequest) return;
    
    try {
      await CopyRequestToClipboard(contextMenuRequest.id);
      
      notificationMessage = "Request copied to clipboard";
      showCopiedNotification = true;
      setTimeout(() => {
        showCopiedNotification = false;
      }, 3000);
      
      handleContextMenuClose();
    } catch (error) {
      console.error('Failed to copy request:', error);
    }
  }

  // Copy request to selected flow
  async function copyRequestToSelectedFlow() {
    if (!contextMenuRequest) return;
    
    try {
      await CopyRequestToSelectedFlow(contextMenuRequest.id);
      
      notificationMessage = "Request added to selected flow";
      showCopiedNotification = true;
      setTimeout(() => {
        showCopiedNotification = false;
      }, 3000);
      
      handleContextMenuClose();
    } catch (error) {
      console.error('Failed to copy request to selected flow:', error);
    }
  }

  // Get context menu items
  function getContextMenuItems() {
    return [
      { label: 'Copy Request', onClick: copyRequestToClipboard },
      { label: 'Copy to Current Flow', onClick: copyRequestToSelectedFlow }
    ];
  }

  // Keyboard navigation for request list
  function handleKeyDown(e: KeyboardEvent) {
    if (!requests.length) return;
    
    // Don't interfere with context menu navigation
    if (showContextMenu) return;
    
    if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
      e.preventDefault();
      
      const currentIndex = selectedRequest ? requests.findIndex(req => req.id === selectedRequest!.id) : -1;
      let newIndex = currentIndex;
      
      if (e.key === 'ArrowUp') {
        newIndex = currentIndex > 0 ? currentIndex - 1 : requests.length - 1;
      } else if (e.key === 'ArrowDown') {
        newIndex = currentIndex < requests.length - 1 ? currentIndex + 1 : 0;
      }
      
      if (newIndex >= 0 && newIndex < requests.length) {
        selectRequest(requests[newIndex]);
      }
    }
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
  
  // Handle mouse down for vertical split position
  function handleMouseDown(e: MouseEvent): void {
    e.preventDefault();
    isDragging = true;
  }
  
  // Handle mouse down for horizontal split position
  function handleDetailsMouseDown(e: MouseEvent): void {
    e.preventDefault();
    isDetailsDragging = true;
  }
  
  // Handle mouse move for vertical split
  function handleMouseMove(e: MouseEvent): void {
    if (!isDragging || !containerRef) return;
    
    const containerRect = containerRef.getBoundingClientRect();
    const containerHeight = containerRect.height;
    const relativeY = e.clientY - containerRect.top;
    
    // Calculate percentage (constrained between 20% and 80%)
    const newPosition = Math.min(80, Math.max(20, (relativeY / containerHeight) * 100));
    splitPosition = newPosition;
  }
  
  // Handle mouse move for horizontal split
  function handleDetailsMouseMove(e: MouseEvent): void {
    if (!isDetailsDragging || !detailsContainerRef) return;
    
    const containerRect = detailsContainerRef.getBoundingClientRect();
    const containerWidth = containerRect.width;
    const relativeX = e.clientX - containerRect.left;
    
    // Calculate percentage (constrained between 20% and 80%)
    const newPosition = Math.min(80, Math.max(20, (relativeX / containerWidth) * 100));
    detailsSplitPosition = newPosition;
  }
  
  // Handle mouse up for both splits
  function handleMouseUp() {
    isDragging = false;
    isDetailsDragging = false;
  }
  
  // Combine the keydown handlers for the vertical resize handle
  function handleCombinedKeyDown(e: KeyboardEvent): void {
    // Only handle keyboard events when the resize handle has focus
    if (!e.currentTarget || e.currentTarget !== document.activeElement) return;
    
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      isDragging = true;
    } else if (isDragging) {
      if (e.key === 'ArrowUp') {
        splitPosition = Math.max(20, splitPosition - 1);
        e.preventDefault();
      } else if (e.key === 'ArrowDown') {
        splitPosition = Math.min(80, splitPosition + 1);
        e.preventDefault();
      } else if (e.key === 'Escape') {
        isDragging = false;
        e.preventDefault();
      }
    }
  }
  
  // Combine the keydown handlers for the horizontal resize handle
  function handleCombinedDetailsKeyDown(e: KeyboardEvent): void {
    // Only handle keyboard events when the resize handle has focus
    if (!e.currentTarget || e.currentTarget !== document.activeElement) return;
    
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      isDetailsDragging = true;
    } else if (isDetailsDragging) {
      if (e.key === 'ArrowLeft') {
        detailsSplitPosition = Math.max(20, detailsSplitPosition - 1);
        e.preventDefault();
      } else if (e.key === 'ArrowRight') {
        detailsSplitPosition = Math.min(80, detailsSplitPosition + 1);
        e.preventDefault();
      } else if (e.key === 'Escape') {
        isDetailsDragging = false;
        e.preventDefault();
      }
    }
  }

  onMount(() => {
    // Load requests immediately on mount
    loadRequests();
    
    // Set up polling
    loadRequestsInterval = window.setInterval(loadRequests, 500);
    
    // Setup event listeners for dragging
    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mousemove', handleDetailsMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
    
    // Add keyboard listener for request navigation
    document.addEventListener('keydown', handleKeyDown);
    
    return () => {
      if (loadRequestsInterval) {
        clearInterval(loadRequestsInterval);
      }
      
      // Remove event listeners
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mousemove', handleDetailsMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
      document.removeEventListener('keydown', handleKeyDown);
    };
  });
</script>

<div bind:this={containerRef} class="flex flex-col h-full overflow-hidden relative">
  <!-- Top section - Request list -->
  <div 
    class="flex flex-col overflow-hidden"
    style={`height: ${splitPosition}%`}
  >
    <div class="h-14 border-b border-gray-700/50 bg-gray-800/30 px-4 flex items-center justify-between">
      <BrowserButton variant="primary" size="sm" />
      <div class="flex items-center gap-6">
        
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
              on:contextmenu|preventDefault={(e) => handleRowContextMenu(e, req)}
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

  <!-- Resize handle -->
  <button 
    class={`h-1 w-full bg-gray-700 hover:bg-gray-600 cursor-ns-resize absolute left-0 right-0 z-10 ${isDragging ? 'bg-gray-600' : ''} focus:outline-none focus:ring-2 focus:ring-gray-500`}
    style={`top: ${splitPosition}%; transform: translateY(-50%); border: none;`}
    on:mousedown={handleMouseDown}
    on:keydown={handleCombinedKeyDown}
    aria-label="Resize panels vertically"
  ></button>

  <!-- Bottom section - Request/Response details -->
  <div 
    class="flex flex-col border-t border-gray-700/50 relative overflow-hidden"
    style={`height: ${100 - splitPosition}%`}
  >
    {#if selectedRequest}
      <!-- Details header with buttons -->
      <div class="h-14 border-b border-gray-700/50 bg-gray-800/30 px-4 flex items-center justify-between">
        <h2 class="text-lg font-medium text-gray-100">Details</h2>
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
            class="px-3 py-1 text-sm bg-red-700 text-gray-50 rounded hover:bg-red-600"
            on:click={async () => {
              if (!selectedRequest) return;
              
              const requestId = selectedRequest.id;
              
              try {
                // Call the backend to drop the request
                await DropRequest(selectedRequest.id);
                
                // Clear UI state immediately
                selectedRequest = null;
                editedRequest = null;
                requestContent = '';
                responseContent = '';
                isWaitingForResponse = false;
                
                // Remove from the requests array immediately
                requests = requests.filter(req => req.id !== requestId);
              } catch (error) {
                console.error('Failed to drop request:', error);
              }
            }}
          >
            Drop
          </button>
        </div>
      </div>
      
      <div bind:this={detailsContainerRef} class="flex h-[calc(100%-3.5rem)] relative">
        <!-- Request section -->
        <div 
          class="h-full flex flex-col overflow-hidden border-r border-gray-700/50"
          style={`width: ${detailsSplitPosition}%`}
        >
          <div class="flex-1 bg-gray-800/30 flex flex-col overflow-hidden">
            <div class="px-4 py-2 border-b border-gray-700/50">
              <div class="flex items-center justify-between">
                <h3 class="text-sm font-medium text-gray-100">Request</h3>
              </div>
            </div>
            <div class="flex-1 overflow-hidden">
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
        </div>

        <!-- Horizontal Resize Handle -->
        <button 
          class={`w-1 h-full bg-gray-700 hover:bg-gray-600 cursor-col-resize absolute z-10 ${isDetailsDragging ? 'bg-gray-600' : ''} focus:outline-none focus:ring-2 focus:ring-gray-500`}
          style={`left: ${detailsSplitPosition}%; transform: translateX(-50%); border: none;`}
          on:mousedown={handleDetailsMouseDown}
          on:keydown={handleCombinedDetailsKeyDown}
          aria-label="Resize panels horizontally"
        ></button>

        <!-- Response section -->
        <div 
          class="h-full flex flex-col overflow-hidden"
          style={`width: ${100 - detailsSplitPosition}%`}
        >
          <div class="flex-1 bg-gray-800/30 flex flex-col overflow-hidden">
            <div class="px-4 py-2 border-b border-gray-700/50">
              <div class="flex items-center gap-2">
                <h3 class="text-sm font-medium text-gray-100">Response</h3>
                {#if isWaitingForResponse}
                  <span class="text-yellow-400 text-sm">(Waiting...)</span>
                {/if}
              </div>
            </div>
            <div class="flex-1 overflow-hidden">
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
        </div>
      </div>
    {:else}
      <!-- Empty state when no request is selected -->
      <div class="flex-1 flex items-center justify-center">
        <div class="text-center text-gray-400">
          <div class="text-lg font-medium mb-2">No Request Selected</div>
          <div class="text-sm">Intercepted requests will appear here when you select them from the list above</div>
          <div class="text-xs mt-2 text-gray-500">Use ↑/↓ arrows to navigate between requests</div>
          <div class="text-xs mt-1 text-gray-500">Right-click on a request for more options</div>
        </div>
      </div>
    {/if}
  </div>
</div>

<!-- Context Menu -->
{#if showContextMenu && contextMenuRequest}
  <ContextMenu 
    x={contextMenuX} 
    y={contextMenuY} 
    onClose={handleContextMenuClose}
    items={getContextMenuItems()}
  />
{/if}

<!-- Notification -->
{#if showCopiedNotification}
  <Notification 
    message={notificationMessage}
    type="success"
    duration={3000}
  />
{/if} 