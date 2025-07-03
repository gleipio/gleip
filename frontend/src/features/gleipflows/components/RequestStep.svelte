<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte';
  import MonacoEditor from '../../../components/monaco/MonacoEditor.svelte';
  import type { ExecutionResult, RequestStep as RequestStepType, VariableExtract, FuzzResult } from '../types';
  import { parseRawHttpRequest } from '../utils/httpUtils';
  import { StopFuzzing } from '../../../../wailsjs/go/backend/App';
  import { GetRequestMethod, GetResponseStatusCode } from '../../../../wailsjs/go/network/HTTPHelper';
  import { fuzzResults } from '../store/gleipStore';
  
  export let requestStep: RequestStepType;
  export let executionResult: ExecutionResult | undefined = undefined;
  export let isExecuting: boolean = false;
  export let isExpanded: boolean = false;
  
  const dispatch = createEventDispatcher();
  
  // Reactive variables for request data
  let requestMethod: string = '';
  let responseStatusCode: number | string = '';
  
  // Load request method when step changes
  $: if (requestStep) {
    loadRequestMethod();
  }
  
  // Load response status code when execution result changes
  $: if (executionResult?.transaction?.response) {
    loadResponseStatusCode();
  }
  
  async function loadRequestMethod() {
    if (requestStep) {
      try {
        requestMethod = await GetRequestMethod(requestStep.request);
      } catch (error) {
        console.error('Failed to get request method:', error);
        requestMethod = ''; // fallback
      }
    }
  }
  
  async function loadResponseStatusCode() {
    if (executionResult?.transaction?.response) {
      try {
        responseStatusCode = await GetResponseStatusCode(executionResult.transaction.response);
      } catch (error) {
        console.error('Failed to get response status code:', error);
        responseStatusCode = 'Unknown'; // fallback
      }
    } else {
      responseStatusCode = '';
    }
  }
  
  // Get method color classes
  function getMethodColorClass(method: string): string {
    switch (method) {
      case 'GET': return 'bg-green-500/20 text-green-400';
      case 'POST': return 'bg-blue-500/20 text-blue-400';
      case 'PUT': return 'bg-amber-500/20 text-amber-400';
      case 'DELETE': return 'bg-red-500/20 text-red-400';
      default: return 'bg-gray-500/20 text-gray-50';
    }
  }
    
  // Get method color classes
  function getStatusCodeColorClass(statusCode: number): string {
    if (statusCode >= 200 && statusCode < 300) return 'bg-green-500/20 text-green-400';
    if (statusCode >= 300 && statusCode < 400) return 'bg-amber-500/20 text-amber-400';
    if (statusCode >= 400 && statusCode < 500) return 'bg-red-500/20 text-red-400';
    if (statusCode >= 500 && statusCode < 600) return 'bg-purple-500/20 text-purple-400';
    return 'bg-gray-500/20 text-gray-50';
  }
  
  // Initialize isFuzzMode and isExpanded if they don't exist yet
  // This handles backward compatibility with older data
  $: {
    if (requestStep.isFuzzMode === undefined) {
      dispatch('update', { isFuzzMode: !!requestStep.fuzzSettings });
    }
    
    if (requestStep.stepAttributes.isExpanded === undefined) {
      dispatch('update', { isExpanded: false });
    }
  }
  
  // Use persistently stored mode rather than local state
  $: mode = requestStep.isFuzzMode ? 'fuzz' : 'parse';
  
  // Get fuzz results from separate store to avoid triggering gleip flow re-renders
  $: currentFuzzResults = $fuzzResults[requestStep.stepAttributes.id] || [];
  
  // Handle mode change
  function handleModeChange(newMode: 'parse' | 'fuzz') {
    if (mode !== newMode) {
      const isFuzzMode = newMode === 'fuzz';
      
      // Initialize proper settings when switching modes
      if (isFuzzMode && !requestStep.fuzzSettings) {
        dispatch('update', { 
          isFuzzMode,
          fuzzSettings: {
            delay: 0.3,
            currentWordlist: [],
            fuzzResults: []
          }
        });
      } else if (!isFuzzMode && requestStep.fuzzSettings) {
        // Just update the mode flag but keep fuzzSettings
        dispatch('update', { 
          isFuzzMode
        });
      } else {
        // Just update the mode flag
        dispatch('update', { isFuzzMode });
      }
      
      // Let parent know about mode change
      dispatch('modeChange', { mode: newMode });
    }
  }
  
  // Handle Monaco editor mount
  function handleRequestEditorMount(e: CustomEvent) {
    dispatch('editorMount', { editor: e.detail.editor, type: 'request', stepId: requestStep.stepAttributes.id });
  }
  
  // Handle request change
  function handleRequestChange(e: CustomEvent) {
    // Just store the raw text without extracting any values
    const updatedRawRequest = e.detail.value;
    
    dispatch('update', { 
      request: {
        ...requestStep.request,
        dump: updatedRawRequest,
      }
    });
  }
  
  // Handle host change
  function handleHostChange(e: Event) {
    const target = e.currentTarget;
    if (target instanceof HTMLInputElement) {
      dispatch('update', { 
        request: {
          ...requestStep.request,
          host: target.value
        }
      });
    }
  }
  
  // Handle TLS toggle
  function handleTLSToggle(e: Event) {
    const target = e.currentTarget;
    if (target instanceof HTMLInputElement) {
      dispatch('update', { 
        request: {
          ...requestStep.request,
          tls: target.checked
        }
      });
    }
  }
  
  // Handle setting toggle
  function handleSettingToggle(e: Event, setting: 'recalculateContentLength' | 'gunzipResponse') {
    const target = e.currentTarget;
    if (target instanceof HTMLInputElement) {
      dispatch('update', { [setting]: target.checked });
    }
  }
  
  // Execute this request
  function executeRequest() {
    console.log("🚨 SEND REQ BUTTON CLICKED - executeRequest() called");
    alert("🚨 SEND REQ BUTTON CLICKED - executeRequest() called");
    dispatch('execute');
    console.log("🚨 DISPATCHED 'execute' EVENT");
  }
  
  // Variable extractions
  let isVariableExtractionsExpanded = true;
  
  // Wordlist settings collapse state
  let isWordlistSettingsExpanded = true;
  
  function addVariableExtraction() {
    const newExtraction: VariableExtract = {
      name: '',
      source: 'header',
      selector: ''
    };
    
    dispatch('update', { 
      variableExtracts: [...(requestStep.variableExtracts || []), newExtraction]
    });
  }
  
  function removeVariableExtraction(index: number) {
    const updatedExtractions = [...(requestStep.variableExtracts || [])];
    updatedExtractions.splice(index, 1);
    
    dispatch('update', { 
      variableExtracts: updatedExtractions
    });
  }
  
  function updateVariableExtraction(index: number, field: keyof VariableExtract, value: string) {
    const updatedExtractions = [...(requestStep.variableExtracts || [])];
    updatedExtractions[index] = {
      ...updatedExtractions[index],
      [field]: value
    };
    
    dispatch('update', { 
      variableExtracts: updatedExtractions
    });
  }
  
  // Fuzz settings
  function handleDelayChange(e: Event) {
    const target = e.currentTarget;
    if (target instanceof HTMLInputElement) {
      const delay = parseFloat(target.value);
      dispatch('update', { 
        fuzzSettings: { 
          ...requestStep.fuzzSettings,
          delay 
        } 
      });
    }
  }
  
  // Debug function to check fuzz settings
  function logFuzzSettings() {
    console.log("Current fuzz settings:", requestStep.fuzzSettings);
    if (requestStep.fuzzSettings?.fuzzResults) {
      console.log("Fuzz results count:", requestStep.fuzzSettings.fuzzResults.length);
    } else {
      console.log("No fuzz results available");
    }
  }
  
  function handleFileUpload(e: Event) {
    const target = e.currentTarget;
    if (target instanceof HTMLInputElement && target.files && target.files.length > 0) {
      const file = target.files[0];
      const reader = new FileReader();
      
      reader.onload = (event) => {
        if (event.target && typeof event.target.result === 'string') {
          const content = event.target.result;
          const wordlist = content.split('\n')
            .map(line => line.trim())
            .filter(line => line.length > 0);
          
          console.log(`Loaded wordlist with ${wordlist.length} words`);
          
          dispatch('update', { 
            fuzzSettings: { 
              ...requestStep.fuzzSettings,
              currentWordlist: wordlist,
              // Initialize fuzzResults as an empty array if it doesn't exist
              fuzzResults: requestStep.fuzzSettings?.fuzzResults || []
            } 
          });
        }
      };
      
      reader.readAsText(file);
    }
  }
  
  function startFuzzing() {
    // Make sure fuzzSettings has a fuzzResults array
    if (!requestStep.fuzzSettings || !requestStep.fuzzSettings.fuzzResults) {
      console.log("Initializing fuzzResults array before starting fuzzing");
      dispatch('update', { 
        fuzzSettings: { 
          ...requestStep.fuzzSettings,
          delay: requestStep.fuzzSettings?.delay || 0.3,
          currentWordlist: requestStep.fuzzSettings?.currentWordlist || [],
          fuzzResults: []
        } 
      });
    }
    
    // Collapse the fuzz configuration when starting to fuzz
    dispatch('update', { isExpanded: false });
    
    // Log current state before starting
    console.log("Starting fuzzing with settings:", { 
      wordlist: requestStep.fuzzSettings?.currentWordlist?.length || 0,
      delay: requestStep.fuzzSettings?.delay || 0.3
    });
    
    dispatch('startFuzzing');
  }
  
  // Toggle fuzz configuration
  function toggleFuzzConfig() {
    isWordlistSettingsExpanded = !isWordlistSettingsExpanded;
  }
  
  // Fuzz result handling
  let selectedFuzzResult: FuzzResult | undefined = undefined;
  
  function selectFuzzResult(result: FuzzResult) {
    selectedFuzzResult = result;
  }
  
  // Fuzz results filtering and sorting
  let searchQuery = '';
  let sortColumn: 'word' | 'statusCode' | 'size' | 'time' = 'word';
  let sortDirection: 'asc' | 'desc' = 'asc';
  let statusFilter: 'all' | '2xx' | '3xx' | '4xx' | '5xx' = 'all';
  
  function handleSearchInput(e: Event) {
    const target = e.currentTarget;
    if (target instanceof HTMLInputElement) {
      searchQuery = target.value.toLowerCase();
    }
  }
  
  function handleStatusFilterChange(e: Event) {
    const target = e.currentTarget;
    if (target instanceof HTMLSelectElement) {
      statusFilter = target.value as 'all' | '2xx' | '3xx' | '4xx' | '5xx';
    }
  }
  
  function toggleSort(column: 'word' | 'statusCode' | 'size' | 'time') {
    if (sortColumn === column) {
      // Toggle direction if same column
      sortDirection = sortDirection === 'asc' ? 'desc' : 'asc';
    } else {
      // Set new column and default to ascending
      sortColumn = column;
      sortDirection = 'asc';
    }
  }
  
  function getSortIcon(column: 'word' | 'statusCode' | 'size' | 'time'): string {
    if (sortColumn !== column) return ''; 
    return sortDirection === 'asc' ? '↑' : '↓';
  }
  
  $: filteredAndSortedResults = currentFuzzResults.length > 0
    ? [...currentFuzzResults]
        // Filter by status code
        .filter(result => {
          if (statusFilter === 'all') return true;
          if (statusFilter === '2xx') return result.statusCode >= 200 && result.statusCode < 300;
          if (statusFilter === '3xx') return result.statusCode >= 300 && result.statusCode < 400;
          if (statusFilter === '4xx') return result.statusCode >= 400 && result.statusCode < 500;
          if (statusFilter === '5xx') return result.statusCode >= 500;
          return true;
        })
        // Filter by search query
        .filter(result => {
          if (!searchQuery) return true;
          // Search in response content and word
          return (
            result.word.toLowerCase().includes(searchQuery) || 
            (result.response && result.response.toLowerCase().includes(searchQuery))
          );
        })
        // Sort by selected column
        .sort((a, b) => {
          // Get values to compare based on column
          let valueA = a[sortColumn];
          let valueB = b[sortColumn];
          
          // For string values, convert to lowercase for comparison
          if (typeof valueA === 'string') {
            valueA = (valueA as string).toLowerCase();
          }
          if (typeof valueB === 'string') {
            valueB = (valueB as string).toLowerCase();
          }
          
          // Compare values
          if (valueA < valueB) return sortDirection === 'asc' ? -1 : 1;
          if (valueA > valueB) return sortDirection === 'asc' ? 1 : -1;
          return 0;
        })
    : [];

  $: fuzzStats = currentFuzzResults.length > 0
    ? {
        total: currentFuzzResults.length,
        status: {
          '2xx': currentFuzzResults.filter(r => r.statusCode >= 200 && r.statusCode < 300).length,
          '3xx': currentFuzzResults.filter(r => r.statusCode >= 300 && r.statusCode < 400).length,
          '4xx': currentFuzzResults.filter(r => r.statusCode >= 400 && r.statusCode < 500).length,
          '5xx': currentFuzzResults.filter(r => r.statusCode >= 500).length,
        },
        avgTime: Math.round(currentFuzzResults.reduce((sum, r) => sum + r.time, 0) / currentFuzzResults.length),
        avgSize: Math.round(currentFuzzResults.reduce((sum, r) => sum + r.size, 0) / currentFuzzResults.length),
        minTime: Math.min(...currentFuzzResults.map(r => r.time)),
        maxTime: Math.max(...currentFuzzResults.map(r => r.time)),
      }
    : null;

  // Handle stopping the fuzzing operation
  async function handleStopFuzzing() {
    try {
      await StopFuzzing();
      console.log("Fuzzing stopped by user");
    } catch (error) {
      console.error("Error stopping fuzzing:", error);
    }
  }
</script>

<div class="w-full">
  {#if !isExpanded}
    <!-- Collapsed view -->
    <div class="flex flex-col">
      <!-- Method and URL -->
      <div class="flex items-center text-xs text-gray-50 mb-1">
        <span class={`px-1.5 py-0.5 mr-2 rounded ${requestStep.request.tls ? 'bg-green-500/20 text-green-400' : 'bg-red-500/20 text-red-400'}`}>
          {requestStep.request.tls ? 'TLS' : 'Cleartext'}
        </span>
        <span class={`px-1.5 py-0.5 rounded ${getMethodColorClass(requestMethod)}`}>
          {requestMethod}
        </span>
        
        <!-- Fuzz mode indicator if in fuzz mode -->
        {#if requestStep.isFuzzMode}
          <span class="ml-2 px-2 py-0.5 bg-[var(--color-secondary-accent)] text-black text-xs rounded">
            Fuzz 
            {#if currentFuzzResults.length > 0}
              ({currentFuzzResults.length})
            {/if}
          </span>
        {/if}
      </div>
      {#if requestStep.request.host}
        <div class="flex items-center text-xs text-gray-50 my-2">
          <span class="ml-1 text-gray-100 max-w-[160px] max-h-16 overflow-hidden break-all">{requestStep.request.host}</span>
        </div>
      {/if}

      <!-- Execution information -->
      {#if executionResult}
        <div class="flex items-center justify-between mb-1 text-xs">
          <span class={`px-1.5 py-0.5 rounded ${getStatusCodeColorClass(Number(responseStatusCode))}`}>
            {executionResult.success ? (executionResult.transaction?.response ? responseStatusCode : 'Success') : 'Failed'}
          </span>
          {#if executionResult.executionTime !== undefined}
            <span class="text-gray-50">{executionResult.executionTime}ms</span>
          {/if}
          {#if executionResult.transaction?.response?.dump}
            <span class="text-gray-50">{executionResult.transaction.response.dump.length} bytes</span>
          {/if}
        </div>
        {/if}
        {#if requestStep.request.dump && requestStep.request.dump.length > 0}
          <span class="text-xs text-gray-50 mt-2">Request</span>
          <!-- Request preview -->
          <div class="text-xs text-gray-50 py-2 font-mono bg-[var(--color-table-row-hover)] border border-[var(--color-midnight-darker)] p-1 rounded mt-1 max-h-40 overflow-hidden">
            {#each requestStep.request.dump.split('\n') as line, i}
              {#if i < 50}
                <div class="truncate max-w-[180px]" style="min-height: 1.2em;">{line || ' '}</div>
              {:else if i === 50}
                <div class="text-gray-500">...</div>
              {/if}
            {/each}
          </div>
        {/if}

        <!-- Response preview for successful requests -->
        {#if executionResult && executionResult.success && executionResult.transaction?.response?.dump && executionResult.transaction.response.dump.length > 0}
        <span class="text-xs text-gray-50 mt-2">Response</span>
          <div class="text-xs text-gray-50 py-2 font-mono bg-[var(--color-table-row-hover)] border border-[var(--color-midnight-darker)] p-1 rounded mt-1 max-h-80 overflow-hidden">
            {#each executionResult.transaction.response.dump.split('\n') as line, i}
              {#if i < 50}
                <div class="truncate max-w-[180px]" style="min-height: 1.2em;">{line || ' '}</div>
              {:else if i === 50}
                <div class="text-gray-500">...</div>
              {/if}
            {/each}
          </div>
        {/if}
    </div>
  {:else}
    <!-- Expanded view -->
    <!-- Mode selector with indicator in card header -->
    <div class="flex rounded overflow-hidden mb-4">
      <div class="flex w-full bg-[var(--color-midnight-darker)] rounded overflow-hidden h-8">
      <button 
        class={`flex-1 text-center ${mode === 'parse' ? 'bg-[var(--color-secondary-accent)] text-black' : 'text-gray-50 hover:bg-[var(--color-midnight)] hover:text-white'}`}
        on:click={() => handleModeChange('parse')}
      >
        Default
      </button>
      <button 
        class={`flex-1 text-center ${mode === 'fuzz' ? 'bg-[var(--color-secondary-accent)] text-black' : 'text-gray-50 hover:bg-[var(--color-midnight)] hover:text-white'}`}
        on:click={() => handleModeChange('fuzz')}
      >
        Fuzz
      </button>
      </div>
    </div>

    <!-- Wordlist settings - only shown in fuzz mode -->
    {#if mode === 'fuzz'}
      <div 
        class="flex justify-between items-center cursor-pointer p-2 bg-[var(--color-midnight-darker)] rounded mb-2"
        role="button"
        tabindex="0"
        on:click={toggleFuzzConfig}
        on:keydown={(e) => e.key === 'Enter' && toggleFuzzConfig()}
      >
        <div class="flex items-center">
          <span class="text-sm font-medium text-gray-50">
              {isWordlistSettingsExpanded 
              ? 'Wordlist Settings' 
              : `Wordlist Settings (${requestStep.fuzzSettings?.currentWordlist?.length || 0} words, ${requestStep.fuzzSettings?.delay || 0.3}s delay)`}
          </span>
          {#if currentFuzzResults.length > 0}
            <span class="ml-2 px-2 py-0.5 bg-[var(--color-secondary-accent)] text-black text-xs rounded">
              {currentFuzzResults.length} results
            </span>
          {/if}
        </div>
        <span class="text-gray-50">{isWordlistSettingsExpanded ? '▲' : '▼'}</span>
      </div>
      
      {#if isWordlistSettingsExpanded}
        <div class="p-3 border border-[var(--color-table-border)] bg-[var(--color-midnight)] rounded mb-4">
          <div class="flex flex-col gap-3">
            <!-- Wordlist upload -->
            <div>
              <div class="flex items-center gap-2">
                <label for="wordlist-upload-{requestStep.stepAttributes.id}" class="px-3 py-2 bg-[var(--color-midnight-darker)] hover:bg-[var(--color-midnight)] border border-[var(--color-table-border)] text-gray-50 text-sm rounded cursor-pointer transition-colors">
                  Choose File
                </label>
                <input
                  id="wordlist-upload-{requestStep.stepAttributes.id}"
                  type="file"
                  class="hidden"
                  accept=".txt"
                  on:change={handleFileUpload}
                />
                <span class="text-xs text-gray-500">
                  {requestStep.fuzzSettings?.currentWordlist?.length || 0} words loaded
                </span>
              </div>
            </div>
            
            <!-- Delay setting -->
            <div>
              <label for="delay-input-{requestStep.stepAttributes.id}" class="block text-sm font-medium text-gray-50 mb-1">Delay between requests (seconds)</label>
              <input
                id="delay-input-{requestStep.stepAttributes.id}"
                type="number"
                min="0"
                step="0.1"
                class="w-full bg-[var(--color-midnight-darker)] border border-[var(--color-table-border)] text-white px-3 py-2 rounded text-sm"
                value={requestStep.fuzzSettings?.delay || 0.3}
                on:change={handleDelayChange}
              />
            </div>
            
            <!-- Help text -->
            <div class="text-xs text-gray-50 mt-1">
              Use the <code class="text-[var(--color-warning)] px-1 py-0.5 rounded">{`{{fuzz}}`}</code> variable in your request to indicate where to insert the wordlist values.
            </div>
          </div>
        </div>
      {/if}
    {/if}

    <!-- Stacked layout for request and response -->
    <div class="flex flex-col gap-4">
      <!-- Request section -->
      <div class="w-full">
        <div class="flex flex-col gap-2 mb-2">
          <!-- <label for="host-input-{requestStep.stepAttributes.id}" class="block text-sm font-medium text-gray-50">Host</label> -->
          <div class="flex items-center gap-2">
            <!-- TLS Switch -->
            <div class="flex items-center">
              <label class="flex items-center cursor-pointer">
                <input 
                  type="checkbox" 
                  class="sr-only"
                  checked={requestStep.request.tls || false}
                  on:change={handleTLSToggle}
                />
                <div class={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${requestStep.request.tls ? 'bg-green-600' : 'bg-gray-600'}`}>
                  <span class={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${requestStep.request.tls ? 'translate-x-6' : 'translate-x-1'}`}></span>
                </div>
                <span class="ml-2 text-xs text-gray-50 min-w-[45px]">
                  {requestStep.request.tls ? 'HTTPS' : 'HTTP'}
                </span>
              </label>
            </div>
            
            <input
              id="host-input-{requestStep.stepAttributes.id}"
              type="text"
              class="bg-[var(--color-midnight-darker)] border border-[var(--color-table-border)] text-white px-3 py-2 rounded text-sm flex-grow"
              placeholder="e.g. api.example.com"
              value={requestStep.request.host}
              on:change={handleHostChange}
            />
            {#if mode === 'parse'}
              <button
                class="px-3 py-2 bg-[var(--color-secondary-accent)] hover:bg-opacity-90 text-[var(--color-button-text)] rounded text-sm"
                on:click={executeRequest}
                disabled={isExecuting}
                title="Execute only this step"
              >
                {isExecuting ? 'Executing...' : 'Send Req'}
              </button>
            {:else if mode === 'fuzz'}
              <button
                class={`px-3 py-2 hover:bg-opacity-90 text-sm rounded ${isExecuting ? 'bg-red-600 hover:bg-red-700 text-white' : 'bg-[var(--color-secondary-accent)] text-[var(--color-button-text)]'}`}
                on:click={isExecuting ? handleStopFuzzing : startFuzzing}
                disabled={!isExecuting && !requestStep.fuzzSettings?.currentWordlist?.length}
                title={isExecuting ? "Stop fuzzing" : "Start fuzzing this step"}
              >
                {isExecuting ? 'Stop Fuzz' : 'Fuzz Req'}
              </button>
            {/if}
          </div>
        </div>
        
        <!-- Request settings -->
        <div class="flex items-center gap-4 mb-2">
          <label class="flex items-center cursor-pointer">
            <input 
              type="checkbox" 
              class="w-4 h-4 mr-2 accent-[var(--color-secondary-accent)]"
              checked={requestStep.recalculateContentLength}
              on:change={(e) => handleSettingToggle(e, 'recalculateContentLength')}
            />
            <span class="text-sm text-gray-50">Recalculate Content-Length</span>
          </label>
          
          <label class="flex items-center cursor-pointer">
            <input 
              type="checkbox" 
              class="w-4 h-4 mr-2 accent-[var(--color-secondary-accent)]"
              checked={requestStep.gunzipResponse}
              on:change={(e) => handleSettingToggle(e, 'gunzipResponse')}
            />
            <span class="text-sm text-gray-50">Decompress Response</span>
          </label>
        </div>
        
        <h3 id="http-editor-label-{requestStep.stepAttributes.id}" class="block text-sm font-medium text-gray-50 mb-1">Request</h3>
        <div class="w-full h-48 border border-[var(--color-table-border)] overflow-hidden" aria-labelledby="http-editor-label-{requestStep.stepAttributes.id}">
          <MonacoEditor
            language={mode === 'fuzz' ? 'httpWithVarsAndFuzz' : 'httpWithVars'}
            value={requestStep.request.dump}
            on:mount={handleRequestEditorMount}
            on:change={handleRequestChange}
          />
        </div>
        
        {#if mode === 'parse'}
          <!-- Response section for Parse mode -->
          <div class="w-full mt-2">
            <div class="flex justify-between items-center mb-1">
              <h3 class="text-sm font-medium text-gray-50">Response</h3>
              {#if executionResult}
                {#if executionResult.executionTime !== undefined}
                  <div class="flex items-center">
                    <span class="text-xs text-gray-50 mr-2">{executionResult.executionTime} ms</span>
                    <span class={`px-2 py-0.5 text-xs ${executionResult.success ? 'bg-green-500/30 text-green-300' : 'bg-red-500/30 text-red-300'}`}>
                      {executionResult.success ? 'Success' : 'Failed'}
                    </span>
                  </div>
                {/if}
              {/if}
            </div>
            
            {#if executionResult}
              {#if executionResult.transaction?.response}
                <div class="h-48 border border-[var(--color-table-border)] overflow-hidden">
                  <MonacoEditor
                    value={executionResult.transaction.response.dump}
                    readOnly={true}
                    language={executionResult.transaction.response.dump.startsWith('{') ? 'json' : 'http'}
                  />
                </div>

                {#if executionResult.variables && Object.keys(executionResult.variables).length > 0}
                  <div class="mt-2">
                    <div class="text-xs font-medium text-gray-50 mb-1">Extracted Variables</div>
                    <div class="bg-[var(--color-midnight-darker)] p-2 text-xs">
                      {#each Object.entries(executionResult.variables) as [name, value]}
                        <div class="hover:bg-[var(--color-midnight)] rounded px-1 py-0.5 transition-colors">
                          <span class="text-blue-400 select-text cursor-text">{name}</span>
                          <span class="text-gray-500 select-text"> = </span>
                          <span class="text-green-400 select-text cursor-text">{value}</span>
                        </div>
                      {/each}
                    </div>
                  </div>
                {/if}
              {:else if executionResult && executionResult.errorMessage}
                <div class="h-48 border border-[var(--color-table-border)] overflow-hidden">
                  <MonacoEditor
                    value={executionResult.errorMessage}
                    readOnly={true}
                    language="text"
                  />
                </div>
              {/if}
            {:else}
              <div class="flex items-center justify-center h-48 text-gray-50 border border-[var(--color-table-border)] bg-[var(--color-midnight)]">
                Execute the flow to see response here
              </div>
            {/if}
          </div>
          
          <!-- Parse Variables section (collapsed by default) -->
          <div class="w-full mt-2">
            <div 
              class="flex justify-between items-center cursor-pointer p-2 bg-[var(--color-midnight-darker)] rounded" 
              role="button"
              tabindex="0"
              on:click={() => isVariableExtractionsExpanded = !isVariableExtractionsExpanded}
              on:keydown={(e) => e.key === 'Enter' && (isVariableExtractionsExpanded = !isVariableExtractionsExpanded)}
            >
              <span class="text-sm font-medium text-gray-50">Parse Variables from Response</span>
              <span class="text-gray-50">{isVariableExtractionsExpanded ? '▲' : '▼'}</span>
            </div>
            
            {#if isVariableExtractionsExpanded}
              <div class="p-3 border border-[var(--color-table-border)] bg-[var(--color-midnight)] mt-1 rounded">
                <!-- Variable extraction list -->
                {#if requestStep.variableExtracts && requestStep.variableExtracts.length > 0}
                  <div class="mb-3">
                    {#each requestStep.variableExtracts as extraction, i}
                      <div class="flex items-center gap-2 mb-2 p-2 border border-[var(--color-table-border)] rounded">
                        <div class="flex-grow grid grid-cols-3 gap-2">
                          <!-- Variable name -->
                          <div>
                            <label for="var-name-{requestStep.stepAttributes.id}-{i}" class="block text-xs text-gray-50 mb-1">Variable Name</label>
                            <input
                              id="var-name-{requestStep.stepAttributes.id}-{i}"
                              type="text"
                              class="w-full bg-[var(--color-midnight-darker)] border border-[var(--color-table-border)] text-white px-2 py-1 rounded text-sm"
                              placeholder="variableName"
                              value={extraction.name}
                              on:change={(e) => updateVariableExtraction(i, 'name', e.currentTarget.value)}
                            />
                          </div>
                          
                          <!-- Source type -->
                          <div>
                            <label for="var-source-{requestStep.stepAttributes.id}-{i}" class="block text-xs text-gray-50 mb-1">Source</label>
                            <select
                              id="var-source-{requestStep.stepAttributes.id}-{i}"
                              class="w-full bg-[var(--color-midnight-darker)] border border-[var(--color-table-border)] text-white px-2 py-1 rounded text-sm"
                              value={extraction.source}
                              on:change={(e) => updateVariableExtraction(i, 'source', e.currentTarget.value)}
                            >
                              <option value="header">Header</option>
                              <option value="body-json">JSON</option>
                              <option value="cookie">Cookie</option>
                              <option value="body-regex">Regex</option>
                              <option value="status">Status Code</option>
                            </select>
                          </div>
                          
                          <!-- Selector -->
                          <div>
                            <label for="var-selector-{requestStep.stepAttributes.id}-{i}" class="block text-xs text-gray-50 mb-1">
                              {#if extraction.source === 'header'}
                                Header Name
                              {:else if extraction.source === 'body-json'}
                                JSON Path
                              {:else if extraction.source === 'cookie'}
                                Cookie Name
                              {:else if extraction.source === 'body-regex'}
                                Regex Pattern
                              {:else}
                                Selector
                              {/if}
                            </label>
                            <input
                              id="var-selector-{requestStep.stepAttributes.id}-{i}"
                              type="text"
                              class="w-full bg-[var(--color-midnight-darker)] border border-[var(--color-table-border)] text-white px-2 py-1 rounded text-sm"
                              placeholder={
                                extraction.source === 'header' ? 'Content-Type' : 
                                extraction.source === 'body-json' ? 'data.id' :
                                extraction.source === 'cookie' ? 'session' :
                                extraction.source === 'body-regex' ? '([a-z]+)' :
                                ''
                              }
                              value={extraction.selector}
                              on:change={(e) => updateVariableExtraction(i, 'selector', e.currentTarget.value)}
                            />
                          </div>
                        </div>
                        
                        <!-- Remove button -->
                        <button
                          class="p-1 text-red-400 hover:text-red-300 bg-[var(--color-midnight-darker)] rounded"
                          on:click={() => removeVariableExtraction(i)}
                          title="Remove"
                        >
                          ✕
                        </button>
                      </div>
                    {/each}
                  </div>
                {:else}
                  <div class="text-center py-3 text-gray-50 text-sm">
                    No variable extractions configured.
                  </div>
                {/if}
                
                <!-- Add variable button -->
                <button
                  class="w-full px-3 py-2 bg-[var(--color-table-border)] hover:bg-opacity-80 text-white rounded text-sm mt-2"
                  on:click={addVariableExtraction}
                >
                  + Add Variables to parse from response
                </button>
              </div>
            {/if}
          </div>
        {:else}
          <!-- Fuzz Mode section -->
          <div class="w-full mt-2">          
            <!-- Fuzz results list - always visible when results are available -->
            <div class="border border-[var(--color-table-border)] rounded overflow-hidden mt-2">
              <div class="bg-[var(--color-midnight-darker)] px-3 py-2 flex justify-between items-center">
                <h3 class="text-sm font-medium text-gray-50">
                  Fuzz Results 
                  {#if currentFuzzResults.length > 0}
                    ({filteredAndSortedResults.length}/{currentFuzzResults.length})
                  {:else}
                    (0)
                  {/if}
                </h3>
                {#if isExecuting}
                  <span class="text-xs text-gray-50 animate-pulse">Fuzzing in progress...</span>
                {/if}
              </div>
              
              <!-- Search and filters -->
              <div class="px-3 py-2 bg-[var(--color-midnight)] flex flex-col md:flex-row gap-2">
                <div class="flex-grow">
                  <label for="search-input-{requestStep.stepAttributes.id}" class="sr-only">Search in responses</label>
                  <input
                    id="search-input-{requestStep.stepAttributes.id}"
                    type="text"
                    placeholder="Search in responses..."
                    class="w-full bg-[var(--color-midnight-darker)] border border-[var(--color-table-border)] text-white px-3 py-1.5 rounded text-sm"
                    value={searchQuery}
                    on:input={handleSearchInput}
                  />
                </div>
                <div class="w-full md:w-40">
                  <label for="status-filter-{requestStep.stepAttributes.id}" class="sr-only">Filter by status code</label>
                  <select
                    id="status-filter-{requestStep.stepAttributes.id}"
                    class="w-full bg-[var(--color-midnight-darker)] border border-[var(--color-table-border)] text-white px-3 py-1.5 rounded text-sm"
                    value={statusFilter}
                    on:change={handleStatusFilterChange}
                  >
                    <option value="all">All Status</option>
                    <option value="2xx">2xx Success</option>
                    <option value="3xx">3xx Redirect</option>
                    <option value="4xx">4xx Client Error</option>
                    <option value="5xx">5xx Server Error</option>
                  </select>
                </div>
              </div>
              
              <!-- Statistics summary -->
              {#if currentFuzzResults.length > 0 && fuzzStats}
                <div class="p-2 bg-[var(--color-midnight-darker)] border-t border-b border-[var(--color-table-border)]">
                  <div class="flex flex-wrap gap-2 text-xs">
                    <div class="bg-[var(--color-midnight)] rounded px-2 py-1">
                      <span class="text-gray-50">Total:</span> 
                      <span class="text-white">{fuzzStats.total}</span>
                    </div>
                    
                    <div class="bg-green-500/20 text-green-300 rounded px-2 py-1">
                      <span>2xx:</span> 
                      <span>{fuzzStats.status['2xx']}</span>
                    </div>
                    
                    <div class="bg-purple-500/20 text-purple-300 rounded px-2 py-1">
                      <span>3xx:</span> 
                      <span>{fuzzStats.status['3xx']}</span>
                    </div>
                    
                    <div class="bg-orange-500/20 text-orange-300 rounded px-2 py-1">
                      <span>4xx:</span> 
                      <span>{fuzzStats.status['4xx']}</span>
                    </div>
                    
                    <div class="bg-red-500/20 text-red-300 rounded px-2 py-1">
                      <span>5xx:</span> 
                      <span>{fuzzStats.status['5xx']}</span>
                    </div>
                    
                    <div class="bg-[var(--color-midnight)] rounded px-2 py-1">
                      <span class="text-gray-50">Avg time:</span> 
                      <span class="text-white">{fuzzStats.avgTime} ms</span>
                    </div>
                    
                    <div class="bg-[var(--color-midnight)] rounded px-2 py-1">
                      <span class="text-gray-50">Min/Max time:</span> 
                      <span class="text-white">{fuzzStats.minTime}/{fuzzStats.maxTime} ms</span>
                    </div>
                    
                    <div class="bg-[var(--color-midnight)] rounded px-2 py-1">
                      <span class="text-gray-50">Avg size:</span> 
                      <span class="text-white">{fuzzStats.avgSize} B</span>
                    </div>
                  </div>
                </div>
              {/if}
              
              {#if currentFuzzResults.length > 0}
                <!-- Results table in a fixed height scrollable viewport -->
                <div class="h-60 overflow-y-auto">
                  <table class="w-full text-sm text-left text-gray-100">
                    <thead class="text-xs text-gray-50 uppercase bg-[var(--color-midnight-darker)] sticky top-0">
                      <tr>
                        <th 
                          class="px-3 py-2 cursor-pointer hover:text-white"
                          on:click={() => toggleSort('word')}
                        >
                          Word {getSortIcon('word')}
                        </th>
                        <th 
                          class="px-3 py-2 cursor-pointer hover:text-white"
                          on:click={() => toggleSort('statusCode')}
                        >
                          Status {getSortIcon('statusCode')}
                        </th>
                        <th 
                          class="px-3 py-2 cursor-pointer hover:text-white"
                          on:click={() => toggleSort('size')}
                        >
                          Size {getSortIcon('size')}
                        </th>
                        <th 
                          class="px-3 py-2 cursor-pointer hover:text-white"
                          on:click={() => toggleSort('time')}
                        >
                          Time {getSortIcon('time')}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {#each filteredAndSortedResults as result}
                        <tr 
                          class={`border-b border-[var(--color-table-border)] hover:bg-[var(--color-midnight-darker)] cursor-pointer ${selectedFuzzResult === result ? 'bg-[var(--color-midnight-darker)]' : ''}`}
                          on:click={() => selectFuzzResult(result)}
                        >
                          <td class="px-3 py-2">{result.word}</td>
                          <td class="px-3 py-2">
                            <span class={`px-2 py-0.5 text-xs rounded 
                              ${result.statusCode >= 200 && result.statusCode < 300 ? 'bg-green-500/30 text-green-300' : 
                              result.statusCode >= 300 && result.statusCode < 400 ? 'bg-purple-500/30 text-purple-300' :
                              result.statusCode >= 400 && result.statusCode < 500 ? 'bg-orange-500/30 text-orange-300' :
                              result.statusCode >= 500 ? 'bg-red-500/30 text-red-300' : ''}`}
                            >
                              {result.statusCode}
                            </span>
                          </td>
                          <td class="px-3 py-2">{result.size} B</td>
                          <td class="px-3 py-2">{result.time} ms</td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                </div>
              {:else}
                <div class="p-4 text-sm text-gray-50 text-center">
                  {isExecuting ? 'Waiting for results...' : 'No fuzz results yet. Start fuzzing to see results here.'}
                </div>
              {/if}
            </div>
            
            <!-- Selected result details - appears only when a result is clicked -->
            {#if currentFuzzResults.length > 0 && selectedFuzzResult}
              <div class="mt-4 grid grid-cols-1 gap-4">
                <div>
                  <h4 class="block text-sm font-medium text-gray-50 mb-1">Request for "{selectedFuzzResult.word}"</h4>
                  <div class="h-48 border border-[var(--color-table-border)] overflow-hidden">
                    <MonacoEditor
                      value={selectedFuzzResult.request}
                      readOnly={true}
                      language="http"
                    />
                  </div>
                </div>
                
                <div>
                  <h4 class="block text-sm font-medium text-gray-50 mb-1">Response ({selectedFuzzResult.statusCode})</h4>
                  <div class="h-48 border border-[var(--color-table-border)] overflow-hidden">
                    <MonacoEditor
                      value={selectedFuzzResult.response}
                      readOnly={true}
                      language={selectedFuzzResult.response.startsWith('{') ? 'json' : 'http'}
                    />
                  </div>
                </div>
              </div>
            {/if}
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div> 