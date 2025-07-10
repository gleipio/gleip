<script lang="ts">
  import { onMount } from 'svelte';
  import { GetProxyRequests, GetProxyRequestsAfter, GetTransactionDetails, SearchProxyRequests, GetTransactionMetadata, GetTransactionChunk, SearchProxyRequestsWithSort, SetRequestHistorySorting, GetRequestHistorySorting } from '../../../wailsjs/go/backend/App';
  import { network, backend } from '../../../wailsjs/go/models';
  import { EventsOn, EventsEmit, Quit } from '../../../wailsjs/runtime/runtime';
  import ReqListCanvas from './components/ReqListCanvas.svelte';
  import { getStatusColor } from '../../shared/utils/httpColors';
  import BrowserButton from '../../components/ui/BrowserButton.svelte';
  import MonacoEditor from '../../components/monaco/MonacoEditor.svelte';
  import FilterPopup from './components/FilterPopup.svelte';
  
  // Import type extensions for HTTPResponse.printable import '../../types/extensions';
  
  type HTTPTransactionSummary = network.HTTPTransactionSummary;
  type HTTPTransaction = network.HTTPTransaction;
  
  // State
  let requests: HTTPTransactionSummary[] = [];
  let filteredRequests: HTTPTransactionSummary[] = [];
  let selectedRequestDetails: HTTPTransaction | null = null;
  let selectedRequestSummary: HTTPTransactionSummary | null = null;
  let isLoadingDetails = false;
  let requestEditor: any;
  let responseEditor: any;
  let prettyPrint = { request: false, response: false };
  let requestContent = 'Select a request to view details';
  let responseContent = 'Select a request to view details';
  let requestLanguage = 'http';
  let responseLanguage = 'http';
  let lastRequestId = "";
  let isFetching = false;
  let currentLoadingId: string | null = null; // Track which request is being loaded
  let isSearching = false; // Track if a search is in progress
  
  // Sorting state
  let sortColumn = 'id';
  let sortDirection = 'desc';
  
  // Filter state (unified with search)
  let isFilterPopupOpen = false;
  let filters = new backend.RequestFilters({
    query: '',
    hasParams: '',
    statusCodes: '',
    methods: [],
    responseSize: { operator: '>', value: '' },
    hosts: ''
  });
  let hasActiveFilters = false;
  
  // UI state
  let listContainerHeight: number;
  let splitPosition = 55; // Default 55% for top section
  let isDragging = false;
  let detailsSplitPosition = 50; // Default 50% for request panel
  let isDetailsDragging = false;
  let containerRef: HTMLDivElement;
  let detailsContainerRef: HTMLDivElement;
  let isMounted = true;
  let loadingTimeoutId: number | null = null;
  let searchTimeoutId: number | null = null; // For debouncing search
  let updateContentTimeout: number | null = null;
  
  // Chunked loading state
  let isLoadingRequestChunks = false;
  let isLoadingResponseChunks = false;
  let requestLoadingProgress = 0;
  let responseLoadingProgress = 0;
  let requestChunkBuffers = new Map<string, string[]>(); // transactionId -> chunks
  let responseChunkBuffers = new Map<string, string[]>(); // transactionId -> chunks
  
  // Handle search input with debounce
  async function handleSearchInput() {
    // Clear previous timeout if it exists
    if (searchTimeoutId !== null) {
      clearTimeout(searchTimeoutId);
    }
    
    // Set a new timeout (300ms debounce)
    searchTimeoutId = setTimeout(async () => {
      if (isMounted) {
        await performSearch();
      }
    }, 300) as unknown as number;
  }
  
  // Perform the actual search
  async function performSearch() {
    if (isSearching) return;
    
    isSearching = true;
    
    try {
      // Use current filters with search query
      const searchResults = await SearchProxyRequestsWithSort(filters, sortColumn, sortDirection);
      filteredRequests = searchResults;
      
      console.log(`Search found ${filteredRequests.length} matching requests`);
    } catch (error) {
      console.error('Search failed:', error);
      filteredRequests = [];
    } finally {
      isSearching = false;
    }
  }
  
  // Check if filters are active
  function updateHasActiveFilters() {
    hasActiveFilters = !!(
      filters.query ||
      filters.hasParams ||
      filters.statusCodes ||
      filters.methods.length > 0 ||
      filters.responseSize.value ||
      filters.hosts
    );
  }
  
  // Handle filter changes
  function handleFilterApply(event: CustomEvent) {
    filters = event.detail;
    updateHasActiveFilters();
    performSearch();
  }
  
  // Open filter popup  
  function openFilterPopup() {
    isFilterPopupOpen = true;
  }
  
  // Handle sorting event from ReqListCanvas
  async function handleSort(event: CustomEvent<{ column: string; direction: string }>) {
    const { column, direction } = event.detail;
    
    try {
      // Update backend sorting state
      await SetRequestHistorySorting(column, direction);
      
      // Update local sorting state
      sortColumn = column;
      sortDirection = direction;
      
      // Re-fetch requests with new sorting
      await performSearch();
      
      console.log(`Sort applied: ${column} ${direction}`);
    } catch (error) {
      console.error('Failed to apply sorting:', error);
    }
  }
  
  // Load initial sorting state from backend
  async function loadSortingState() {
    try {
      const sortingState = await GetRequestHistorySorting();
      sortColumn = sortingState.sortColumn || 'id';
      sortDirection = sortingState.sortDirection || 'desc';
      
      // Ensure we never have empty direction strings
      if (sortDirection === '') {
        sortDirection = 'desc';
      }
      
      console.log(`Loaded sorting state: ${sortColumn} ${sortDirection}`);
    } catch (error) {
      console.error('Failed to load sorting state:', error);
      // Use defaults
      sortColumn = 'id';
      sortDirection = 'desc';
    }
  }
  
  // Watch for changes to the search query
  $: if (filters.query !== undefined) {
    handleSearchInput();
  }
  
  // Handle row click: set summary, fetch details with chunked loading
  async function handleRowClick(summary: HTTPTransactionSummary) {
    // If already loading this request, do nothing
    if (isLoadingDetails && currentLoadingId === summary.id) {
      return;
    }
    
    // Cancel any previous loading operation
    if (loadingTimeoutId) {
      clearTimeout(loadingTimeoutId);
      loadingTimeoutId = null;
    }
    
    selectedRequestSummary = summary; // Highlight row immediately
    selectedRequestDetails = null; // Clear old details
    isLoadingDetails = true;
    currentLoadingId = summary.id;
    
    // Reset Monaco editor content immediately
    requestContent = 'Loading request...';
    responseContent = 'Loading response...';
    requestLanguage = 'http';
    responseLanguage = 'http';
    
    console.log(`Starting chunked loading for ID: ${summary.id}`);
    
    try {
      // Check if chunked loading functions are available
      if (typeof GetTransactionMetadata === 'undefined' || typeof GetTransactionChunk === 'undefined') {
        console.log('Chunked loading functions not available, falling back to old method');
        await loadWithOldMethod(summary.id);
        return;
      }
      
      // Get metadata about the transaction to know chunk counts
      const metadata = await GetTransactionMetadata(summary.id);
      console.log(`Transaction metadata:`, metadata);
      
      if (!metadata.hasRequest && !metadata.hasResponse) {
        requestContent = 'No request data available';
        responseContent = 'No response data available';
        return;
      }
      
      // Start loading request and response chunks concurrently
      const promises = [];
      
      if (metadata.hasRequest) {
        promises.push(loadDataInChunks(summary.id, 'request', metadata.requestChunks || 1));
      } else {
        requestContent = 'No request data available';
      }
      
      if (metadata.hasResponse) {
        promises.push(loadDataInChunks(summary.id, 'response', metadata.responseChunks || 1));
      } else {
        responseContent = 'No response data available';
      }
      
      // Wait for all chunks to load
      await Promise.all(promises);
      
      console.log(`Chunked loading completed for ID: ${summary.id}`);
      
    } catch (error) {
      console.error(`Failed to load chunks for ${summary.id}:`, error);
      console.log('Falling back to old method due to error');
      await loadWithOldMethod(summary.id);
    } finally {
      if (isMounted && currentLoadingId === summary.id) {
        isLoadingDetails = false;
        currentLoadingId = null;
        isLoadingRequestChunks = false;
        isLoadingResponseChunks = false;
        
        if (loadingTimeoutId) {
          clearTimeout(loadingTimeoutId);
          loadingTimeoutId = null;
        }
      }
    }
  }
  
  // Load data in chunks and progressively update the Monaco editor
  async function loadDataInChunks(transactionId: string, dataType: 'request' | 'response', totalChunks: number) {
    console.log(`🔄 loadDataInChunks STARTED for ${dataType} of ${transactionId}, totalChunks: ${totalChunks}`);
    const isRequest = dataType === 'request';
    
    if (isRequest) {
      isLoadingRequestChunks = true;
      requestLoadingProgress = 0;
      requestContent = '';
    } else {
      isLoadingResponseChunks = true;
      responseLoadingProgress = 0;
      responseContent = '';
    }
    
    let assembledContent = '';
    
    for (let chunkIndex = 0; chunkIndex < totalChunks; chunkIndex++) {
      console.log(`🔄 Processing chunk ${chunkIndex} for ${dataType}`);
      // Check if we're still loading the same transaction
      if (currentLoadingId !== transactionId) {
        console.log(`❌ Loading cancelled for ${transactionId} at chunk ${chunkIndex}, currentLoadingId: ${currentLoadingId}`);
        return;
      }
      
      try {
        const chunk = await GetTransactionChunk(transactionId, dataType, chunkIndex);
        console.log(`✅ Got chunk ${chunkIndex} for ${dataType}, data length: ${chunk.chunkData?.length || 0}`);
        
        // Append chunk data to assembled content
        assembledContent += chunk.chunkData;
        
        // Update the Monaco editor with progressive content
        if (isRequest) {
          requestContent = assembledContent;
          requestLoadingProgress = Math.round(((chunkIndex + 1) / totalChunks) * 100);
          console.log(`📝 Set requestContent, length: ${requestContent.length}`);
        } else {
          responseContent = assembledContent;
          responseLoadingProgress = Math.round(((chunkIndex + 1) / totalChunks) * 100);
          console.log(`📝 Set responseContent, length: ${responseContent.length}`);
        }
        
        // Force Monaco editor to update (small delay to allow DOM updates)
        await new Promise(resolve => setTimeout(resolve, 10));
        
      } catch (error) {
        console.error(`Failed to load chunk ${chunkIndex} for ${dataType}:`, error);
        if (isRequest) {
          requestContent = assembledContent + '\n\n[Error loading remaining content]';
        } else {
          responseContent = assembledContent + '\n\n[Error loading remaining content]';
        }
        break;
      }
    }
    
    // Mark loading as complete
    if (isRequest) {
      isLoadingRequestChunks = false;
      requestLoadingProgress = 100;
      console.log(`🏁 Request loading completed for ${transactionId}`);
    } else {
      isLoadingResponseChunks = false;
      responseLoadingProgress = 100;
      console.log(`🏁 Response loading completed for ${transactionId}`);
    }
  }
  
  // Function to extract headers from dump
  function extractHeaders(dump: string): { [key: string]: string } {
    if (!dump) return {};
    
    const headers: { [key: string]: string } = {};
    const lines = dump.split('\r\n');
    
    // Skip the first line (request/response line)
    for (let i = 1; i < lines.length; i++) {
      const line = lines[i];
      if (!line) break; // Empty line indicates end of headers
      
      const [key, ...values] = line.split(':');
      if (key && values.length > 0) {
        headers[key.toLowerCase()] = values.join(':').trim();
      }
    }
    
    return headers;
  }
  
  // Function to extract content type from headers
  function getContentType(headers: { [key: string]: string }): string {
    const contentType = headers['content-type'];
    if (!contentType) return '';
    return contentType.split(';')[0].toLowerCase();
  }

  // Function to get Monaco language from content type
  function getMonacoLanguage(contentType: string, isRequest: boolean = false): string {
    if (isRequest) return 'http';
    
    switch (contentType) {
      // case 'application/json':
      //   return 'json';
      // case 'application/xml':
      // case 'text/xml':
      //   return 'xml';
      // case 'text/html':
      //   return 'html';
      // case 'text/css':
      //   return 'css';
      // case 'text/javascript':
      // case 'application/javascript':
      //   return 'javascript';
      // case 'text/yaml':
      // case 'application/yaml':
      //   return 'yaml';
      // case 'text/plain':
      //   return 'http';
      default:
        return 'http';
    }
  }
  
  // Function to extract headers and body from dump
  function extractParts(dump: string): { headers: string, body: string } {
    if (!dump) return { headers: '', body: '' };
    
    const parts = dump.split('\r\n\r\n');
    if (parts.length < 2) return { headers: dump, body: '' };
    return {
      headers: parts[0],
      body: parts.slice(1).join('\r\n\r\n')
    };
  }
    // Function to get status code from response
  function getStatusCode(response: string): number | null {
    if (!response) return null;
    
    // The status code is in the first line of the response
    const firstLine = response.split('\r\n')[0];
    const match = firstLine.match(/HTTP\/[\d.]+ (\d+)/);
    
    if (match && match[1]) {
      return parseInt(match[1], 10);
    }
    
    return null;
  }
  
  // Initial load of requests and setup event listener
  async function loadInitialRequests() {
    isFetching = true;
    try {
      // Load requests with current sorting applied
      const emptyFilters = new backend.RequestFilters({});
      const sortedRequests = await SearchProxyRequestsWithSort(emptyFilters, sortColumn, sortDirection);
      
      // Debug log to track initial requests
      console.log(`Loaded ${sortedRequests.length} initial requests with sorting: ${sortColumn} ${sortDirection}`);
      
      // No need to deduplicate or sort manually since backend handles it
      requests = sortedRequests;
      filteredRequests = requests; // Initialize filtered requests with sorted requests
      
      // Select the first request if we have any
      if (requests.length > 0 && selectedRequestSummary === null) {
        handleRowClick(requests[0]);
      }
    } catch (error) {
      console.error("Failed to load initial requests:", error);
      // Fallback to old method if sorting fails
      try {
        const initialRequests = await GetProxyRequests();
        console.log(`Fallback: Loaded ${initialRequests.length} initial requests`);
        
        // Ensure no duplicates in the fallback load
        const uniqueRequestsMap = new Map();
        initialRequests.forEach(req => {
          uniqueRequestsMap.set(req.id, req);
        });
        
        // Convert map to array and sort by sequence number (descending)
        requests = Array.from(uniqueRequestsMap.values())
          .sort((a, b) => b.seqNumber - a.seqNumber);
        
        filteredRequests = requests;
        console.log(`Deduplicated to ${requests.length} unique requests`);
        
        // Select the first request if we have any
        if (requests.length > 0 && selectedRequestSummary === null) {
          handleRowClick(requests[0]);
        }
      } catch (fallbackError) {
        console.error("Fallback load also failed:", fallbackError);
      }
    } finally {
      isFetching = false;
    }
  }
  
  // Loading more requests from the backend
  async function loadNewRequests() {
    if (isFetching) return;
    isFetching = true;
    
    try {
      console.log("Current requests:", requests.length);
      
      // Get the latest timestamp if we have requests
      let latestRequestID = "";
      if (requests.length > 0) {
        // Get the ID of the most recent request
        latestRequestID = requests[0].id;
        console.log("Fetching requests after ID:", latestRequestID);
      }
      
      // Use the API to get new requests after the latest ID
      const newRequests = await GetProxyRequestsAfter(latestRequestID);
      
      console.log(`Loaded ${newRequests.length} new requests after ID ${latestRequestID || 'start'}`);
      
      if (newRequests.length > 0) {
        // Create a Set of existing IDs for fast lookup
        const existingIds = new Set(requests.map(req => req.id));
        
        // Filter out any duplicates from newRequests
        const uniqueNewRequests = newRequests.filter(req => !existingIds.has(req.id));
        
        console.log(`Adding ${uniqueNewRequests.length} unique new requests (filtered out ${newRequests.length - uniqueNewRequests.length} duplicates)`);
        
        // Add new requests and ensure they're sorted by seqNumber
        if (uniqueNewRequests.length > 0) {
          // Update the main requests array and sort by seqNumber (descending)
          requests = [...uniqueNewRequests, ...requests]
            .sort((a, b) => b.seqNumber - a.seqNumber);
          
          // Update filtered requests based on current search state
          if (!filters.query.trim()) {
            // If no search query, re-fetch with current sorting
            await performSearch(); // This will load sorted requests
          } else {
            // If active search, refresh the search to include new requests with sorting
            await performSearch(); // This already handles search + sorting
          }
          
          // If we're showing the list for the first time, select the first request
          if (selectedRequestSummary === null && filteredRequests.length > 0) {
            handleRowClick(filteredRequests[0]);
          }
        }
      }
    } catch (error) {
      console.error("Failed to load new requests:", error);
    } finally {
      isFetching = false;
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
  
  // Fallback to old loading method
  async function loadWithOldMethod(id: string) {
    try {
      const details = await GetTransactionDetails(id);
      
      if (details.request && details.request.dump) {
        requestContent = details.request.dump;
      } else {
        requestContent = 'No request data available';
      }
      
      if (details.response && details.response.printable) {
        responseContent = details.response.printable;
      } else {
        responseContent = 'No response data available';
      }
      
    } catch (error) {
      console.error(`Failed to load with old method for ${id}:`, error);
      requestContent = 'Error loading request data';
      responseContent = 'Error loading response data';
    }
  }
  
  onMount(() => {
    // Initialize filter state
    updateHasActiveFilters();
    
    // Load initial sorting state first
    loadSortingState().then(() => {
      // Then load initial requests with proper sorting
      loadInitialRequests();
    });
    
    // Listen for new transaction events from the backend
    const cleanup = EventsOn('new_transaction', () => {
      console.log("Received new transaction event from proxy");
      // Use a small timeout to batch potential rapid events
      setTimeout(() => {
        console.log("Loading new requests after event");
        loadNewRequests();
      }, 50);
    });
    

    
    // Handle global keyboard shortcuts, particularly Command+Q
    const handleKeyDown = (e: KeyboardEvent) => {
      // Allow save/open/new commands to pass through to the app
      if ((e.metaKey || e.ctrlKey) && (e.key === 's' || e.key === 'o' || e.key === 'n')) {
        // Don't prevent default - let these commands pass through to the app
        return;
      }
      
      // Explicitly handle Command+Q (or Ctrl+Q) for application quit
      if ((e.metaKey || e.ctrlKey) && e.key === 'q') {
        e.preventDefault(); // Prevent default browser behavior
        e.stopPropagation(); // Stop event propagation
        console.log("Command+Q detected, quitting application");
        // Call Wails Quit function directly
        Quit();
      }
    };
    
    // Add global keydown listener
    window.addEventListener('keydown', handleKeyDown);
    
    // Setup event listeners for dragging
    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mousemove', handleDetailsMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
    
    // We'll handle resize handle keyboard events directly on the elements, not globally
    
    return () => {
      cleanup();
      isMounted = false;
      

      
      // Remove event listeners
      window.removeEventListener('keydown', handleKeyDown);
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mousemove', handleDetailsMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
      
      // Clear update content timeout
      if (updateContentTimeout) {
        clearTimeout(updateContentTimeout);
      }
    };
  });
  
  // Debug function to check if events are being received
  function testEventEmission() {
    console.log("Manually emitting test event");
    EventsEmit("new_transaction");
  }
</script>

<style>
  /* Subtle toggle button styles */
  .toggle-btn {
    transition: all 0.2s ease;
  }
  
  .toggle-btn:hover {
    opacity: 0.9;
  }
  
  .toggle-btn-on {
    background-color: var(--color-secondary-accent);
    color: var(--color-button-text);
  }
  
  .toggle-btn-on:hover {
    background-color: var(--color-secondary-accent);
    box-shadow: 0 0 0 1px var(--color-secondary-accent);
  }
  
  .toggle-btn-off {
    background-color: var(--color-midnight-darker);
    color: var(--color-gray-300);
  }
  
  .toggle-btn-off:hover {
    background-color: var(--color-midnight-darker);
    box-shadow: 0 0 0 1px var(--color-midnight-accent);
  }
</style>

<div bind:this={containerRef} class="flex flex-col h-full overflow-hidden relative">
  <!-- Top section - Request list -->
  <div 
    class={`flex flex-col overflow-hidden ${selectedRequestSummary ? '' : 'h-full'}`}
    style={selectedRequestSummary ? `height: ${splitPosition}%` : ''}
  >
    <div class="flex flex-col h-full">
      <div class="border-b border-[var(--color-midnight-darker)] bg-[var(--color-midnight-light)]/70 px-4 py-3 flex items-center gap-2 shrink-0">
        <div class="relative flex-1">
          <input
            type="text"
            placeholder="Search requests..."
            bind:value={filters.query}
            class="w-full bg-[var(--color-search-bar-bg)] text-[var(--color-search-bar-text)] px-3 py-1.5 rounded border border-[var(--color-midnight-darker)] focus:border-[var(--color-midnight-accent)] focus:outline-none focus:ring-1 focus:ring-[var(--color-midnight-accent)] text-sm"
          />
          {#if filters.query}
            <button 
              on:click={() => {
                filters.query = '';
                performSearch();
              }}
              class="absolute right-2 top-1/2 -translate-y-1/2 text-[var(--color-search-bar-text)]/70 hover:text-[var(--color-search-bar-text)]"
            >
              ✕
            </button>
          {/if}
          {#if isSearching}
            <div class="absolute right-8 top-1/2 -translate-y-1/2">
              <svg class="animate-spin h-4 w-4 text-[var(--color-search-bar-text)]/70" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
            </div>
          {/if}
        </div>
        <div class="flex items-center gap-3 flex-shrink-0">
          <button
            on:click|stopPropagation={openFilterPopup}
            class={`px-3 py-1 text-sm rounded border transition-colors ${
              hasActiveFilters
                ? 'bg-[var(--color-midnight-darker)] text-[var(--color-midnight-accent)] border-[var(--color-midnight-accent)]'
                : 'bg-[var(--color-midnight-darker)] text-gray-300 border-[var(--color-midnight-darker)] hover:border-[var(--color-midnight-accent)]'
            }`}
            title="Filter requests"
          >
            <svg class="w-4 h-4 inline mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z"></path>
            </svg>
            Filter
          </button>
          <BrowserButton variant="primary" size="sm" />
        </div>
      </div>
      
      <!-- Use the canvas-based RequestList component -->
      <ReqListCanvas
        requests={filteredRequests}
        selectedRequestId={selectedRequestSummary?.id || null}
        {sortColumn}
        {sortDirection}
        on:select={event => handleRowClick(event.detail)}
        on:close={() => {
          selectedRequestSummary = null;
          selectedRequestDetails = null;
        }}
        on:sort={handleSort}
      />
    </div>
  </div>

  <!-- Resize handle -->
  {#if selectedRequestSummary}
    <button 
      class={`h-1 w-full bg-[var(--color-midnight-darker)] hover:bg-[var(--color-midnight-accent)] cursor-ns-resize absolute left-0 right-0 z-10 ${isDragging ? 'bg-[var(--color-midnight-accent)]' : ''} focus:outline-none focus:ring-2 focus:ring-[var(--color-midnight-accent)]`}
      style={`top: ${splitPosition}%; transform: translateY(-50%); border: none;`}
      on:mousedown={handleMouseDown}
      on:keydown={handleCombinedKeyDown}
      aria-label="Resize panels vertically"
    ></button>
  {/if}

  <!-- Bottom section - Request/Response details -->
  {#if selectedRequestSummary}
    <div 
      class="flex flex-col border-t border-[var(--color-midnight-darker)] relative overflow-hidden"
      style={`height: ${100 - splitPosition}%`}
    >
      <!-- Close button -->
      <button 
        on:click={() => {
          selectedRequestSummary = null;
          selectedRequestDetails = null;
        }}
        class="absolute top-2 right-2 z-20 text-gray-50 hover:text-gray-50 bg-[var(--color-midnight-darker)]/70 p-1 rounded-full hover:bg-[var(--color-midnight-darker)]"
      >
        ✕
      </button>
      
      <div bind:this={detailsContainerRef} class="flex h-[calc(100%-40px)] relative">
        <!-- Request section -->
        <div 
          class="h-full flex flex-col overflow-hidden border-r border-[var(--color-midnight-darker)]"
          style={`width: ${detailsSplitPosition}%`}
        >
          <div class="flex-1 bg-[var(--color-midnight-light)]/30 flex flex-col overflow-hidden">
            <div class="px-4 py-2 border-b border-[var(--color-midnight-darker)]">
              <div class="flex items-center justify-between">
                <h3 class="text-sm font-medium text-gray-50">Request</h3>
                {#if isLoadingRequestChunks}
                  <div class="flex items-center gap-2 text-xs text-gray-500">
                    <span>Loading {requestLoadingProgress}%</span>
                    <div class="w-16 h-1 bg-gray-700 rounded-full overflow-hidden">
                      <div 
                        class="h-full bg-[var(--color-midnight-accent)] transition-all duration-300"
                        style={`width: ${requestLoadingProgress}%`}
                      ></div>
                    </div>
                  </div>
                {/if}
              </div>
            </div>
            <div class="flex-1 overflow-hidden">
              <!-- Debug info -->
              <!-- <div class="text-xs text-gray-500 p-2">Request content length: {requestContent.length}, Language: {requestLanguage}</div> -->
              <MonacoEditor
                bind:this={requestEditor}
                value={requestContent}
                language={requestLanguage}
                readOnly={true}
                automaticLayout={true}
                fontSize={11}

              />
            </div>
          </div>
        </div>

        <!-- Horizontal Resize Handle -->
        <button 
          class={`w-1 h-full bg-[var(--color-midnight-darker)] hover:bg-[var(--color-midnight-accent)] cursor-col-resize absolute z-10 ${isDetailsDragging ? 'bg-[var(--color-midnight-accent)]' : ''} focus:outline-none focus:ring-2 focus:ring-[var(--color-midnight-accent)]`}
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
          <div class="flex-1 bg-[var(--color-midnight-light)]/30 flex flex-col overflow-hidden">
            <div class="px-4 py-2 border-b border-[var(--color-midnight-darker)]">
              <div class="flex items-center gap-2">
                <h3 class="text-sm font-medium text-gray-50">Response</h3>
                
                {#if isLoadingResponseChunks}
                  <div class="flex items-center gap-2 text-xs text-gray-500">
                    <span>Loading {responseLoadingProgress}%</span>
                    <div class="w-16 h-1 bg-gray-700 rounded-full overflow-hidden">
                      <div 
                        class="h-full bg-[var(--color-midnight-accent)] transition-all duration-300"
                        style={`width: ${responseLoadingProgress}%`}
                      ></div>
                    </div>
                  </div>
                {/if}
                
                <!-- Status code badge -->
                {#if responseContent && responseContent !== 'Loading response...' && responseContent !== ''}
                  {@const statusCode = getStatusCode(responseContent)}
                  {#if statusCode}
                    <div 
                      class="px-2 py-0.5 text-xs font-semibold rounded text-white" 
                      style={`background-color: ${getStatusColor(statusCode)}`}
                    >
                      {statusCode}
                    </div>
                  {/if}
                {/if}
              </div>
            </div>
            <div class="flex-1 overflow-hidden">
              <!-- Debug info -->
              <!-- <div class="text-xs text-gray-500 p-2">Response content length: {responseContent.length}, Language: {responseLanguage}</div> -->
              <MonacoEditor
                bind:this={responseEditor}
                value={responseContent}
                language={responseLanguage}
                readOnly={true}
                automaticLayout={true}
                fontSize={11}

              />
            </div>
          </div>
        </div>
      </div>

      <!-- Loading indicator -->
      {#if isLoadingDetails}
        <div class="absolute inset-0 bg-[var(--color-midnight-light)]/50 flex items-center justify-center z-10">
          <svg class="animate-spin h-8 w-8 text-[var(--color-midnight-accent)]" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
        </div>
      {/if}
    </div>
  {/if}
</div>

<!-- Filter Popup -->
<FilterPopup 
  isOpen={isFilterPopupOpen}
  {filters}
  on:apply={handleFilterApply}
  on:close={() => isFilterPopupOpen = false}
/>