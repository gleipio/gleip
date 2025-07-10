<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { backend } from '../../../../wailsjs/go/models';
  
  export let isOpen = false;
  export let filters: backend.RequestFilters;
  
  const dispatch = createEventDispatcher();
  
  // Available HTTP methods
  const availableMethods = ['GET', 'POST', 'PUT', 'DELETE', 'PATCH', 'HEAD', 'OPTIONS', 'TRACE'];
  
  let popupElement: HTMLDivElement;
  let methodDropdownOpen = false;
  
  function closePopup() {
    isOpen = false;
    methodDropdownOpen = false;
    dispatch('close');
  }
  
  function triggerFilter() {
    dispatch('apply', filters);
  }
  
  function clearFilters() {
    filters = new backend.RequestFilters({
      query: '',
      hasParams: '',
      statusCodes: '',
      methods: [],
      responseSize: { operator: '>', value: '' },
      hosts: ''
    });
    dispatch('apply', filters);
    closePopup();
  }
  
  function handleMethodChange(method: string, checked: boolean) {
    if (checked) {
      filters.methods = [...filters.methods, method];
    } else {
      filters.methods = filters.methods.filter(m => m !== method);
    }
    triggerFilter();
  }
  
    let eventListenersAdded = false;

  function handleClickOutside(event: MouseEvent) {
    if (!isOpen || !popupElement) return;
    
    const target = event.target as Element;
    if (!popupElement.contains(target)) {
      closePopup();
      return;
    }
    
    // Close method dropdown when clicking outside of it but inside popup
    methodDropdownOpen = false;
  }

  function handleKeyDown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      closePopup();
    }
  }

  function addEventListeners() {
    if (!eventListenersAdded) {
      document.addEventListener('click', handleClickOutside);
      document.addEventListener('keydown', handleKeyDown);
      eventListenersAdded = true;
    }
  }

  function removeEventListeners() {
    if (eventListenersAdded) {
      document.removeEventListener('click', handleClickOutside);
      document.removeEventListener('keydown', handleKeyDown);
      eventListenersAdded = false;
    }
    methodDropdownOpen = false;
  }

  function setupPopup(node: HTMLElement) {
    addEventListeners();
    
    return {
      destroy() {
        removeEventListeners();
      }
    };
  }
</script>

{#if isOpen}
  <div class="fixed inset-0 z-[9999] flex items-start justify-center pt-20">
    <div class="absolute inset-0 bg-black/50"></div>
    
    <div 
      bind:this={popupElement}
      use:setupPopup
      class="relative bg-[var(--color-midnight-light)] border border-[var(--color-midnight-darker)] rounded-lg shadow-xl w-96 max-h-[80vh] overflow-hidden"
    >
      <!-- Header -->
      <div class="px-4 py-3 border-b border-[var(--color-midnight-darker)] flex items-center justify-between">
        <h3 class="text-lg font-medium text-gray-50">Filter Requests</h3>
        <button 
          on:click={closePopup}
          class="text-gray-400 hover:text-gray-50 p-1 rounded"
        >
          ✕
        </button>
      </div>
      
      <!-- Filter options -->
      <div class="p-4 space-y-4 overflow-y-auto max-h-[60vh]">
        <!-- Has Parameters -->
        <div class="flex items-center justify-between">
          <span class="text-sm text-gray-50">Has Parameters</span>
          <label class="relative inline-flex items-center cursor-pointer">
            <input 
              type="checkbox" 
              class="sr-only peer"
              checked={filters.hasParams === 'yes'}
              on:change={(e) => {
                filters.hasParams = e.currentTarget.checked ? 'yes' : '';
                triggerFilter();
              }}
            />
            <div class="w-9 h-5 bg-gray-600 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-[var(--color-midnight-accent)]"></div>
          </label>
        </div>

        <!-- Methods -->
        <div class="space-y-2">
          <div class="relative">
            <button 
              type="button"
              data-dropdown-button
              on:click|stopPropagation={() => methodDropdownOpen = !methodDropdownOpen}
              class="w-full bg-[var(--color-search-bar-bg)] text-[var(--color-search-bar-text)] px-3 py-2 rounded border border-[var(--color-midnight-darker)] focus:border-[var(--color-midnight-accent)] focus:outline-none focus:ring-1 focus:ring-[var(--color-midnight-accent)] text-left flex items-center justify-between"
            >
              <span>{filters.methods.length > 0 ? filters.methods.join(', ') : 'HTTP Methods'}</span>
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
              </svg>
            </button>
            
            {#if methodDropdownOpen}
              <div data-dropdown-menu class="absolute z-10 w-full mt-1 bg-[var(--color-search-bar-bg)] border border-[var(--color-midnight-darker)] rounded shadow-lg">
                <div class="p-2 grid grid-cols-2 gap-1">
                  {#each availableMethods as method}
                    <label class="flex items-center space-x-2 text-sm px-2 py-1 hover:bg-[var(--color-midnight-darker)] rounded cursor-pointer" on:click|stopPropagation>
                      <input
                        type="checkbox"
                        checked={filters.methods.includes(method)}
                        on:change={(e) => handleMethodChange(method, e.currentTarget.checked)}
                        on:click|stopPropagation
                        class="rounded border-[var(--color-midnight-darker)] bg-[var(--color-search-bar-bg)] text-[var(--color-midnight-accent)] focus:ring-[var(--color-midnight-accent)]"
                      />
                      <span class="text-gray-50">{method}</span>
                    </label>
                  {/each}
                </div>
              </div>
            {/if}
          </div>
        </div>

        <!-- Status Codes -->
        <div class="space-y-2">
          <input
            type="text"
            placeholder="Status codes (200-299,404,500)"
            bind:value={filters.statusCodes}
            on:input={triggerFilter}
            class="w-full bg-[var(--color-search-bar-bg)] text-[var(--color-search-bar-text)] px-3 py-2 rounded border border-[var(--color-midnight-darker)] focus:border-[var(--color-midnight-accent)] focus:outline-none focus:ring-1 focus:ring-[var(--color-midnight-accent)]"
          />
        </div>
        
        <!-- Response Size -->
        <div class="flex gap-2">
          <select 
            bind:value={filters.responseSize.operator}
            on:change={triggerFilter}
            class="bg-[var(--color-search-bar-bg)] text-[var(--color-search-bar-text)] px-3 py-2 rounded border border-[var(--color-midnight-darker)] focus:border-[var(--color-midnight-accent)] focus:outline-none focus:ring-1 focus:ring-[var(--color-midnight-accent)]"
            style="appearance: none; -webkit-appearance: none; -moz-appearance: none;"
          >
            <option value=">">&gt;</option>
            <option value="<">&lt;</option>
          </select>
          <input
            type="text"
            placeholder="Response size (bytes)"
            bind:value={filters.responseSize.value}
            pattern="[0-9]*"
            on:input={triggerFilter}
            class="flex-1 bg-[var(--color-search-bar-bg)] text-[var(--color-search-bar-text)] px-3 py-2 rounded border border-[var(--color-midnight-darker)] focus:border-[var(--color-midnight-accent)] focus:outline-none focus:ring-1 focus:ring-[var(--color-midnight-accent)]"
          />
        </div>
        
        <!-- Hosts -->
        <div class="space-y-2">
          <input
            type="text"
            placeholder="Hosts (example.com,api.test.com)"
            bind:value={filters.hosts}
            on:input={triggerFilter}
            class="w-full bg-[var(--color-search-bar-bg)] text-[var(--color-search-bar-text)] px-3 py-2 rounded border border-[var(--color-midnight-darker)] focus:border-[var(--color-midnight-accent)] focus:outline-none focus:ring-1 focus:ring-[var(--color-midnight-accent)]"
          />
        </div>
      </div>
      
      <!-- Footer -->
      <div class="px-4 py-3 border-t border-[var(--color-midnight-darker)] flex items-center justify-end gap-2">
        <button
          on:click={clearFilters}
          class="px-3 py-1.5 text-sm text-gray-400 hover:text-gray-50 rounded border border-[var(--color-midnight-darker)] hover:border-[var(--color-midnight-accent)]"
        >
          Clear
        </button>
      </div>
    </div>
  </div>
{/if} 