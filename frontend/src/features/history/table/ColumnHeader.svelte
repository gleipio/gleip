<script lang="ts">
  import type { Column } from '../../../types/TableTypes';

  // Props
  export let column: Column;
  // export let index: number;
  export let isLast: boolean = false;
  export let onResize: (width: number) => void;
  export let onSort: (columnId: string) => void;
  export let sortColumn: string = '';
  export let sortDirection: string = '';

  // State
  let isResizing = false;
  let resizeStartX = 0;
  let resizeStartWidth = 0;

  // Handle resize start
  function handleResizeStart(e: MouseEvent) {
    isResizing = true;
    resizeStartX = e.clientX;
    resizeStartWidth = column.width;
    e.preventDefault();

    // Add global event listeners
    window.addEventListener('mousemove', handleResizeMove);
    window.addEventListener('mouseup', handleResizeEnd);
  }

  // Handle resize move
  function handleResizeMove(e: MouseEvent) {
    if (!isResizing) return;
    
    const deltaX = e.clientX - resizeStartX;
    const minWidth = Math.max(column.minWidth, 30);
    const newWidth = Math.max(minWidth, resizeStartWidth + deltaX);
    
    // Notify parent
    onResize(newWidth);
  }

  // Handle resize end
  function handleResizeEnd() {
    isResizing = false;
    
    // Remove global event listeners
    window.removeEventListener('mousemove', handleResizeMove);
    window.removeEventListener('mouseup', handleResizeEnd);
  }

  // Handle column header click for sorting
  function handleHeaderClick(e: MouseEvent) {
    // Don't trigger sort if clicking on resize handle
    if (isResizing || (e.target as HTMLElement).closest('.resize-handle')) {
      return;
    }
    
    onSort(column.id);
  }

  // Check if this column is the primary sort column
  $: isPrimarySortColumn = sortColumn === column.id;
  
  // Get the sort direction for this column
  $: currentSortDirection = isPrimarySortColumn ? sortDirection : '';
</script>

<div 
  class="h-full flex items-center border-b border-[var(--color-table-border)] text-[var(--color-gray-400)] relative cursor-pointer hover:bg-[var(--color-table-row-hover)] transition-colors {isLast ? 'flex-1' : 'shrink-0'}"
  style="{isLast ? 'min-width: ' + column.width + 'px;' : 'width: ' + column.width + 'px; min-width: ' + Math.max(column.minWidth, 30) + 'px;'}"
  on:click={handleHeaderClick}
>
  <!-- Column header text with appropriate alignment -->
  {#if column.id === 'id' || column.id === 'length'}
    <div class="text-xs font-bold uppercase text-left w-full pl-2 pr-2 flex items-center">
      <span class="overflow-hidden" style="max-width: {isPrimarySortColumn ? 'calc(100% - 20px)' : '100%'};">
        {column.name}
      </span>
      {#if isPrimarySortColumn}
        <span class="text-[var(--color-midnight-accent)] ml-1 flex-shrink-0 w-3 text-center">
          {#if currentSortDirection === 'asc'}
            ↑
          {:else}
            ↓
          {/if}
        </span>
      {/if}
    </div>
  {:else}
    <div class="text-xs font-bold uppercase text-left w-full pl-2 pr-1 flex items-center">
      <span class="overflow-hidden" style="max-width: {isPrimarySortColumn ? 'calc(100% - 20px)' : '100%'};">
        {column.name}
      </span>
      {#if isPrimarySortColumn}
        <span class="text-[var(--color-midnight-accent)] ml-1 flex-shrink-0 w-3 text-center">
          {#if currentSortDirection === 'asc'}
            ↑
          {:else}
            ↓
          {/if}
        </span>
      {/if}
    </div>
  {/if}
  
  <!-- Resize handle - only for resizable columns that are not the last -->
  {#if !isLast && column.resizable}
    <button 
      class="absolute right-0 top-0 bottom-0 w-3 translate-x-[50%] cursor-col-resize z-10 group border-0 bg-transparent p-0 resize-handle"
      aria-label="Resize {column.name} column"
      on:mousedown={handleResizeStart}
    >
      <!-- Line centered exactly at the boundary -->
      <div class="absolute inset-0 flex items-center justify-center">
        <div class="h-4 w-[2px] bg-[var(--color-table-header-separator)] group-hover:bg-[var(--color-midnight-accent)] group-hover:h-5"></div>
      </div>
    </button>
  {/if}
</div> 