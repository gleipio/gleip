<script lang="ts">
  import { onMount, createEventDispatcher, onDestroy } from 'svelte';
  import { getMethodColor } from '../../../shared/utils/httpColors';
  import type { network } from '../../../../wailsjs/go/models';
  import { CopyRequestToClipboard, CopyRequestToSelectedFlow, SearchProxyRequests } from '../../../../wailsjs/go/backend/App';
  import ColumnHeader from '../table/ColumnHeader.svelte';
  import ContextMenu from '../../../shared/components/ContextMenu.svelte';
  import Notification from '../../../shared/components/Notification.svelte';
  import { CanvasRenderer } from '../services/CanvasRenderer';
  import { TableColumnManager } from '../services/TableColumnManager';
  import { TableKeyboardManager } from '../services/TableKeyboardManager';
  import type { Column, TableConfig } from '../../../types/TableTypes';
  import { truncateWithEllipsis, getCssVar } from '../../../shared/utils/domHelpers';
  
  type HTTPTransactionSummary = network.HTTPTransactionSummary;
  
  // Constants
  const ROW_HEIGHT = 24;
  const HEADER_HEIGHT = 24;
  const SCROLL_BUFFER_ROWS = 10;
  const MIN_SCROLL_OFFSET = 1; // Prevent edge case with zero scroll
  const RESIZE_HANDLE_WIDTH = 8; // Width of draggable area for resize handles (reduced from 12)
  const HEADER_PADDING_LEFT = 8; // Padding for header columns (reduced from 12)
  const ROW_PADDING_LEFT = 12; // Slightly more padding for row content (reduced from 16)
  const COLUMN_RIGHT_PADDING = 20; // Extra right padding for last column
  const RESIZE_HANDLE_OFFSET = 0; // Align exactly with column boundary (changed from -2)
  
  // Event dispatcher
  const dispatch = createEventDispatcher<{
    select: HTTPTransactionSummary;
    close: void; // Add new event for closing details panel
    sort: { column: string; direction: string }; // Add sorting event
  }>();
  
  // Props
  export let requests: HTTPTransactionSummary[] = [];
  export let selectedRequestId: string | null = null;
  
  // Sorting state - now comes from parent
  export let sortColumn: string = 'id';
  export let sortDirection: string = 'desc';
  
  // Column definitions
  let columns: Column[] = [
    { id: 'id', name: '#', width: 50, resizable: true, minWidth: 45 },
    { id: 'host', name: 'HOST', width: 180, resizable: true, minWidth: 80 },
    { id: 'method', name: 'METHOD', width: 62, resizable: true, minWidth: 62 },
    { id: 'url', name: 'URL', width: 510, resizable: true, minWidth: 100 },
    { id: 'params', name: 'PARAMS', width: 62, resizable: true, minWidth: 40 },
    { id: 'bytes', name: 'BYTES', width: 75, resizable: true, minWidth: 40 },
    { id: 'time', name: 'TIME', width: 60, resizable: true, minWidth: 50 } // Changed from autoExpand to fixed width
  ];
  
  // Canvas and context references
  let canvasContainerRef: HTMLDivElement;
  let canvasRef: HTMLCanvasElement;
  let ctx: CanvasRenderingContext2D;
  
  // UI state
  let visibleStartIndex = 0;
  let visibleEndIndex = 0;
  let focusedRowIndex = -1;
  let hoveredRowIndex = -1;
  let canvasWidth = 0;
  let canvasHeight = 0;
  let dpr = 1; // Device pixel ratio
  let totalColumnsWidth = 0; // Track total width of all columns for horizontal scrolling
  
  // Resizing state
  let resizingColumnIndex = -1;
  let resizeStartX = 0;
  let resizeStartWidth = 0;
  let isResizing = false;
  
  // Handle scroll event for both vertical and horizontal scrolling
  let scrollRAF: number | null = null;
  let totalContentHeight = 0; // Track total content height for scrollbar
  let lastScrollTop = 0; // Track last scroll position to detect reset
  
  // Track horizontal scroll position
  let horizontalScrollPos = 0;
  
  // Track if currently hovering over a valid row
  let isHoveringValidRow = false;
  
  // Computed effective width that ensures the container is at least 100% wide
  let effectiveContainerWidth = 0;
  
  // Update effective width when totalColumnsWidth changes
  $: {
    if (canvasContainerRef) {
      const containerWidth = canvasContainerRef.clientWidth;
      effectiveContainerWidth = Math.max(totalColumnsWidth || 0, containerWidth || 0);
    } else {
      effectiveContainerWidth = totalColumnsWidth || 800; // Fallback width
    }
  }
  
  // Context menu state
  let showContextMenu = false;
  let contextMenuX = 0;
  let contextMenuY = 0;
  let contextMenuRequest: HTTPTransactionSummary | null = null;
  let showCopiedNotification = false;
  let notificationMessage = '';
  
  // No longer needed - using selected flow directly
  
  // Services
  let columnManager: TableColumnManager;
  let renderer: CanvasRenderer<HTTPTransactionSummary>;
  let keyboardManager: TableKeyboardManager<HTTPTransactionSummary>;
  
  function handleScroll(e: Event) {
    const target = e.target as HTMLElement;
    
    // Update horizontal scroll position and sync header scroll
    horizontalScrollPos = target.scrollLeft;
    const headerContainer = document.querySelector('.header-scroll-container');
    if (headerContainer) {
      headerContainer.scrollLeft = horizontalScrollPos;
    }
    
    // If we already have a scheduled frame for vertical scroll handling, don't schedule another
    if (!scrollRAF) {
      scrollRAF = requestAnimationFrame(() => {
        updateVisibleRange();
        renderer?.redraw();
        scrollRAF = null;
      });
    }
  }

  // Calculate which row was clicked
  function handleCanvasClick(e: MouseEvent) {
    if (!canvasRef || !requests.length) return;
    
    const rect = canvasRef.getBoundingClientRect();
    const y = e.clientY - rect.top;
    const scrollTop = canvasContainerRef?.scrollTop || 0;
    
    // Calculate which row was clicked by adding scroll position to the y-coordinate
    // This accounts for scrolling with sticky positioned canvas
    const clickedRowIndex = Math.floor((y + scrollTop) / ROW_HEIGHT);
    
    console.log(`Click at y=${y}, scrollTop=${scrollTop}, clickedRow=${clickedRowIndex}, total rows=${requests.length}`);
    
    if (clickedRowIndex >= 0 && clickedRowIndex < requests.length) {
      // If this is a right-click or ctrl-click (Mac), show context menu
      if (e.button === 2 || (e.button === 0 && e.ctrlKey)) {
        e.preventDefault();
        contextMenuRequest = requests[clickedRowIndex];
        contextMenuX = e.clientX;
        contextMenuY = e.clientY;
        showContextMenu = true;
        return;
      }
      
      // Set focus to this row for regular clicks
      focusedRowIndex = clickedRowIndex;
      // Dispatch selection event
      dispatch('select', requests[clickedRowIndex]);
      // Force redraw
      renderer?.redraw();
    }
    
    // Hide context menu when clicking elsewhere
    showContextMenu = false;
  }
  
  // Handle mouse move for hover effect
  function handleCanvasMouseMove(e: MouseEvent) {
    if (!canvasRef || !requests.length) {
      const wasHovering = isHoveringValidRow;
      isHoveringValidRow = false;
      if (wasHovering) {
        applyCanvasStyles(); // Update cursor when hover state changes
      }
      return;
    }
    
    const rect = canvasRef.getBoundingClientRect();
    const y = e.clientY - rect.top;
    const scrollTop = canvasContainerRef?.scrollTop || 0;
    
    // Calculate row under mouse, accounting for scroll position
    const mouseRowIndex = Math.floor((y + scrollTop) / ROW_HEIGHT);
    
    // Update hover state for cursor style
    const wasHovering = isHoveringValidRow;
    isHoveringValidRow = mouseRowIndex >= 0 && mouseRowIndex < requests.length;
    
    // Update cursor if hover state changed
    if (wasHovering !== isHoveringValidRow) {
      applyCanvasStyles();
    }
    
    // Only redraw if the hovered row changed
    if (mouseRowIndex >= 0 && mouseRowIndex < requests.length && mouseRowIndex !== hoveredRowIndex) {
      hoveredRowIndex = mouseRowIndex;
      renderer?.redraw();
    } else if ((mouseRowIndex < 0 || mouseRowIndex >= requests.length) && hoveredRowIndex !== -1) {
      hoveredRowIndex = -1;
      renderer?.redraw();
    }
  }
  
  // Handle mouse leave
  function handleCanvasMouseLeave() {
    isHoveringValidRow = false;
    if (hoveredRowIndex !== -1) {
      hoveredRowIndex = -1;
      renderer?.redraw();
    }
    // Ensure canvas styles remain applied (Svelte 5 fix)
    applyCanvasStyles();
  }

  // Calculate column positions to find boundaries
  function getColumnPositions(): number[] {
    const positions: number[] = []; 
    let xOffset = HEADER_PADDING_LEFT; // Start with initial padding
    
    // Add each column boundary position
    columns.forEach(column => {
      positions.push(xOffset); // Start of column
      xOffset += column.width;
      positions.push(xOffset); // End of column without offset for better alignment
    });
    
    return positions;
  }
  
  // Calculate column boundaries for visual indicators
  function getColumnBoundaries(): number[] {
    const boundaries: number[] = [];
    let xOffset = HEADER_PADDING_LEFT;
    
    // Add each column end position with additional offset to match header separators
    for (let i = 0; i < columns.length; i++) {
      const column = columns[i];
      xOffset += column.width;
      // Add column width as offset to align with header separators
      boundaries.push(xOffset-8);
    }
    
    return boundaries;
  }
  
  // Handle mouse up globally (in case user releases outside the canvas)
  function handleGlobalMouseUp() {
    try {
      if (isResizing) {
        isResizing = false;
        resizingColumnIndex = -1;
        document.body.style.cursor = 'default';
      }
    } catch (error) {
      console.error('Error in handleGlobalMouseUp:', error);
    }
  }
  
  // Handle keyboard navigation
  function handleKeyDown(e: KeyboardEvent): void {
    // Only handle if no input is focused
    if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) {
      return;
    }
    
    // Allow save/open/new commands to pass through to the app
    if ((e.metaKey || e.ctrlKey) && (e.key === 's' || e.key === 'o' || e.key === 'n')) {
      // Don't prevent default - let these commands pass through to the app
      return;
    }
    
    // If no row is currently focused but we have a selected row, start navigation from there
    if (focusedRowIndex === -1 && selectedRequestId !== null) {
      const selectedIndex = requests.findIndex(req => req.id === selectedRequestId);
      if (selectedIndex >= 0) {
        focusedRowIndex = selectedIndex;
      }
    }
    
    // Function to update focused row and handle selection if needed
    const updateFocusedRow = (newIndex: number) => {
      if (newIndex >= 0 && newIndex < requests.length) {
        focusedRowIndex = newIndex;
        ensureRowVisible(focusedRowIndex);
        // Only dispatch selection event if details panel is already open
        if (selectedRequestId !== null) {
          dispatch('select', requests[focusedRowIndex]);
        }
        renderer?.redraw();
      }
    };
    
    e.preventDefault();
    
    switch (e.key) {
      case 'ArrowDown':
        if (focusedRowIndex < requests.length - 1) {
          updateFocusedRow(focusedRowIndex + 1);
        } else if (focusedRowIndex === -1 && requests.length > 0) {
          updateFocusedRow(0);
        }
        break;
        
      case 'ArrowUp':
        if (focusedRowIndex > 0) {
          updateFocusedRow(focusedRowIndex - 1);
        } else if (focusedRowIndex === -1 && requests.length > 0) {
          updateFocusedRow(0);
        }
        break;
        
      case 'Enter':
        if (focusedRowIndex >= 0 && focusedRowIndex < requests.length) {
          dispatch('select', requests[focusedRowIndex]);
        }
        break;
        
      case 'Home':
        if (requests.length > 0) {
          updateFocusedRow(0);
        }
        break;
        
      case 'End':
        if (requests.length > 0) {
          updateFocusedRow(requests.length - 1);
        }
        break;
        
      case 'Escape':
        // If details panel is open, close it
        if (selectedRequestId !== null) {
          dispatch('close');
        }
        break;
        
      default:
        // Don't prevent default for unhandled keys
        e.preventDefault = () => {}; // No-op to restore default behavior
    }
  }
  
  // Ensure a row is visible by scrolling if needed
  function ensureRowVisible(index: number): void {
    if (!canvasContainerRef) return;
    
    const rowTop = index * ROW_HEIGHT;
    const rowBottom = (index + 1) * ROW_HEIGHT;
    const viewportTop = canvasContainerRef.scrollTop;
    const viewportBottom = viewportTop + canvasContainerRef.clientHeight - HEADER_HEIGHT;
    
    if (rowTop < viewportTop) {
      // Scroll up to make the row visible
      canvasContainerRef.scrollTop = rowTop;
    } else if (rowBottom > viewportBottom) {
      // Scroll down to make the row visible
      canvasContainerRef.scrollTop = rowBottom - (canvasContainerRef.clientHeight - HEADER_HEIGHT);
    }
  }
  
  // Update visible range of rows based on scroll position
  function updateVisibleRange() {
    if (!canvasContainerRef || !canvasRef) return;
    
    const containerHeight = canvasContainerRef.clientHeight;
    const scrollTop = canvasContainerRef.scrollTop;
    
    // Calculate exact content height - CRITICAL for correct scrolling
    // Only include the actual rows, nothing more
    totalContentHeight = requests.length * ROW_HEIGHT + ROW_HEIGHT/2;
    
    // Calculate which rows should be visible
    const bufferRows = SCROLL_BUFFER_ROWS;
    visibleStartIndex = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - bufferRows);
    const visibleRowCount = Math.ceil(containerHeight / ROW_HEIGHT);
    visibleEndIndex = Math.min(
      requests.length - 1,
      visibleStartIndex + visibleRowCount + bufferRows
    );
    
    // Apply proper canvas styling to ensure consistent positioning
    applyCanvasStyles();
    
    // Update the horizontal scroll position
    horizontalScrollPos = canvasContainerRef.scrollLeft;
    const headerContainer = document.querySelector('.header-scroll-container');
    if (headerContainer) {
      headerContainer.scrollLeft = horizontalScrollPos;
    }
    
    // Inform the renderer of the updated visible range
    if (renderer) {
      renderer.redraw();
    }
  }
  
  // Get total width of all columns
  function getTotalColumnsWidth(): number {
    // Add extra padding to ensure the last column doesn't get cut off
    return columns.reduce((total, column) => total + column.width, 0) + COLUMN_RIGHT_PADDING;
  }
  
  // Apply proper canvas styling - ensures consistent positioning
  function applyCanvasStyles() {
    if (!canvasRef || !canvasContainerRef) return;
    
    const containerHeight = canvasContainerRef.clientHeight;
    
    // Always apply the complete set of styles for proper positioning
    canvasRef.style.transform = 'none';
    canvasRef.style.position = 'sticky';
    canvasRef.style.top = '0';
    canvasRef.style.left = '0';
    canvasRef.style.width = `${canvasWidth}px`;
    canvasRef.style.height = `${containerHeight}px`;
    canvasRef.style.zIndex = '10';
    canvasRef.style.pointerEvents = 'auto';
    canvasRef.style.cursor = isHoveringValidRow ? 'pointer' : 'default';
  }

  // Resize the canvas to match its container and handle horizontal scrolling
  function resizeCanvas() {
    if (!canvasRef || !canvasContainerRef || !renderer) return;
    
    // Get device pixel ratio - use the exact value
    dpr = window.devicePixelRatio || 1;
    
    // Get the actual container dimensions
    const containerWidth = canvasContainerRef.clientWidth;
    canvasHeight = canvasContainerRef.clientHeight;
    
    // Calculate total width of all columns
    totalColumnsWidth = columnManager.getTotalWidth();
    
    // Calculate effective container width (at least as wide as viewport)
    effectiveContainerWidth = Math.max(totalColumnsWidth, containerWidth);
    
    // Set canvas width to either container width or total columns width, whichever is larger
    canvasWidth = Math.max(containerWidth, totalColumnsWidth);
    
    // Calculate the ideal physical dimensions for the main canvas
    // Set higher resolution for the canvas to ensure crisp rendering
    const mainPhysicalWidth = Math.round(canvasWidth * dpr);
    const mainPhysicalHeight = Math.round(canvasHeight * dpr);
    
    // Set the canvas to its physical size (this affects resolution)
    canvasRef.width = mainPhysicalWidth;
    canvasRef.height = mainPhysicalHeight;
    
    // Apply proper canvas styling
    applyCanvasStyles();
    
    // Reset the transformation matrix and apply DPR scaling
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.scale(dpr, dpr);
    

    
    // Critical: Update the canvas dimensions in the renderer
    renderer.resize(canvasWidth, canvasHeight, dpr);
    
    // Apply canvas styles after renderer resize to prevent override
    applyCanvasStyles();
    
    // Redraw canvas contents after resize
    updateVisibleRange();
  }
  
  // Draw row based on column definitions
  function drawRow(req: HTTPTransactionSummary, rowY: number, isSelected: boolean, isFocused: boolean, isHovered: boolean) {
    // Draw row background
    if (isSelected) {
      // Use midnight accent with opacity for selected rows
      ctx.fillStyle = getCssVar('--color-midnight-accent');
      ctx.globalAlpha = 0.2;
    } else if (isFocused) {
      // Use secondary accent with opacity for focused rows
      ctx.fillStyle = getCssVar('--color-secondary-accent');
      ctx.globalAlpha = 0.15;
    } else if (isHovered) {
      // Use dedicated hover color for better contrast with the new row colors
      ctx.fillStyle = getCssVar('--color-table-row-hover');
      ctx.globalAlpha = 1.0;
    } else {
      // Use absolute row index for consistent alternating colors
      const rowIndex = requests.indexOf(req);
      ctx.fillStyle = getCssVar(rowIndex % 2 === 0 ? '--color-table-row-even' : '--color-table-row-odd');
      ctx.globalAlpha = 1.0;
    }
    ctx.fillRect(0, rowY, canvasWidth, ROW_HEIGHT);
    ctx.globalAlpha = 1.0; // Reset alpha
    
    // Draw row border - using the table border color for better contrast
    ctx.strokeStyle = getCssVar('--color-table-border', getCssVar('--color-midnight-darker'));
    ctx.globalAlpha = 0.6; // Increased from 0.5 for better visibility
    ctx.lineWidth = 1;
    const lineY = Math.floor(rowY + ROW_HEIGHT) - 0.5;
    ctx.beginPath();
    ctx.moveTo(0, lineY);
    ctx.lineTo(canvasWidth, lineY);
    ctx.stroke();
    
    // Get column boundaries for drawing indicators
    const columnBoundaries = getColumnBoundaries();
    
    // Draw column dividers for rows - using the table border color
    columnBoundaries.forEach((boundary, index) => {
      if (index < columnBoundaries.length - 1) {
        ctx.strokeStyle = getCssVar('--color-table-border', getCssVar('--color-midnight-darker'));
        ctx.globalAlpha = 0.5; // Increased from 0.4 for better visibility
        ctx.lineWidth = 1;
        ctx.beginPath();
        
        // Draw each column divider with its boundary position
        const exactLineX = Math.floor(boundary) + 0.5; // Add 0.5 for crisp lines
        ctx.moveTo(exactLineX, rowY);
        ctx.lineTo(exactLineX, rowY + ROW_HEIGHT);
        ctx.stroke();
        ctx.globalAlpha = 1.0; // Reset alpha
      }
    });
    
    // Draw selection/focus indicator
    if (isSelected) {
      ctx.fillStyle = getCssVar('--color-midnight-accent');
      ctx.fillRect(0, rowY, 3, ROW_HEIGHT);
    } else if (isFocused) {
      ctx.fillStyle = getCssVar('--color-secondary-accent');
      ctx.fillRect(0, rowY, 3, ROW_HEIGHT);
    }
    
    // Parse URL for consistent display
    let host = "";
    let path = "";
    let hasParams = false;
    
    try {
      const url = new URL(req.url);
      host = url.host;
      path = url.pathname;
      hasParams = url.search.length > 1; // > 1 to account for just "?"
    } catch (e) {
      // If URL parsing fails, use the full URL
      host = "unknown";
      path = req.url;
    }
    
    // Text rendering
    ctx.font = '11px system-ui, -apple-system, sans-serif';
    ctx.fillStyle = getCssVar('--color-gray-300');
    ctx.textBaseline = 'middle';
    
    // Center text vertically
    const textY = Math.round(rowY + ROW_HEIGHT / 2);
    
    // Draw all columns according to our column definitions
    let xOffset = ROW_PADDING_LEFT; // Initial padding - slightly more than headers
    
    // ID column - right-aligned
    ctx.fillStyle = getCssVar('--color-gray-200');
    ctx.font = 'bold 12px system-ui, -apple-system, sans-serif'; // Slightly larger and bold
    ctx.textAlign = 'right';
    ctx.fillText(req.seqNumber.toString(), Math.floor(xOffset + columns[0].width - ROW_PADDING_LEFT), textY);
    ctx.font = '11px system-ui, -apple-system, sans-serif'; // Reset font for other columns
    ctx.textAlign = 'left';
    xOffset += columns[0].width;
    
    // Host column
    ctx.fillStyle = getCssVar('--color-gray-300');
    const maxHostWidth = columns[1].width - 16; // Reduced from 24 to show more text
    ctx.fillText(truncateWithEllipsis(host, maxHostWidth, ctx), Math.floor(xOffset), textY);
    xOffset += columns[1].width;
    
    // Method column
    const methodColor = getMethodColorValue(req.method);
    ctx.fillStyle = methodColor;
    ctx.font = '600 11px system-ui, -apple-system, sans-serif';
    ctx.fillText(req.method, Math.floor(xOffset), textY);
    ctx.font = '11px system-ui, -apple-system, sans-serif';
    xOffset += columns[2].width;
    
    // URL/Path column
    ctx.fillStyle = getCssVar('--color-gray-300');
    const maxUrlWidth = columns[3].width - 14; // Reduced from 16 to show more text
    ctx.fillText(truncateWithEllipsis(path, maxUrlWidth, ctx), Math.floor(xOffset), textY);
    xOffset += columns[3].width;
    
    // Params indicator - completely centered without any padding issues
    const paramsColumnIndex = 4; // Index of the params column in the columns array
    const paramsColumnWidth = columns[paramsColumnIndex].width;
    
    // The start boundary is at the previous column's end
    const previousColumnEnd = columnBoundaries[paramsColumnIndex - 1];
    // The end boundary is this column's end
    const currentColumnEnd = columnBoundaries[paramsColumnIndex];
    
    // Calculate exact center of column based on boundaries
    const paramsColumnCenter = Math.floor((previousColumnEnd + currentColumnEnd) / 2);
    
    const hasPostParams = req.method === 'POST' || req.method === 'PUT' || req.method === 'PATCH';
    if (hasParams || hasPostParams) {
      // Draw the indicator dot exactly in the center of the column
      ctx.fillStyle = getCssVar('--color-success');
      ctx.beginPath();
      ctx.arc(paramsColumnCenter, textY, 4, 0, 2 * Math.PI);
      ctx.fill();
    }
    
    // Continue with normal xOffset tracking for other columns
    xOffset += paramsColumnWidth;
    
    // Length (Response size) - right-aligned
    ctx.fillStyle = getCssVar('--color-gray-300');
    let lengthText = "-";
    if (req.statusCode) { // If we have a response
      // Always display length in bytes with thousands separators
      const size = req.responseSize;
      lengthText = `${size.toLocaleString()}`;
    }
    ctx.textAlign = 'right';
    ctx.fillText(lengthText, Math.floor(xOffset + columns[5].width - 8), textY); // Reduced from ROW_PADDING_LEFT
    ctx.textAlign = 'left';
    xOffset += columns[5].width;
    
    // Time column
    const time = new Date(req.timestamp).toLocaleTimeString();
    ctx.fillText(time, Math.floor(xOffset), textY);
  }
  
  // Draw the main canvas with all content always visible
  function redrawCanvas() {
    if (!ctx || !canvasRef) return;
    
    // If no requests, draw empty state and return
    if (!requests.length) {
      ctx.clearRect(0, 0, canvasRef.width / dpr, canvasRef.height / dpr);
      ctx.font = '20px system-ui, -apple-system, sans-serif';
      ctx.fillStyle = getCssVar('--color-gray-400');
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      
      // Get the input element to check if there's a search query
      const searchInput = document.querySelector('input[placeholder="Search requests..."]') as HTMLInputElement;
      const hasSearchQuery = searchInput && searchInput.value.trim() !== '';
      
      if (hasSearchQuery) {
        // Show message for no search results
        ctx.fillText('No matching requests found', Math.round(canvasWidth / 2), Math.round(canvasHeight / 2));
      } else {
        // Show message for no requests at all
        ctx.fillText('No requests captured yet', Math.round(canvasWidth / 2), Math.round(canvasHeight / 2));
      }
      return;
    }
    
    // Clear entire canvas
    ctx.clearRect(0, 0, canvasRef.width / dpr, canvasRef.height / dpr);
    
    // Get scroll position directly
    const scrollTop = canvasContainerRef?.scrollTop || 0;
    
    // Calculate how many rows to draw
    const rowsToDraw = Math.min(
      requests.length - visibleStartIndex,
      Math.ceil(canvasHeight / ROW_HEIGHT) + 20 // Add extra buffer
    );
    
    // Iterate through visible rows
    for (let i = 0; i < rowsToDraw; i++) {
      const rowIndex = visibleStartIndex + i;
      if (rowIndex >= requests.length) break;
      
      const req = requests[rowIndex];
      
      // Calculate precise row position based on exact scroll offset
      const rowStartY = rowIndex * ROW_HEIGHT;
      const rowY = rowStartY - scrollTop;
      
      // Skip rows that are completely outside the visible area with margin
      if (rowY < -ROW_HEIGHT || rowY > canvasHeight) continue;
      
      const isSelected = req.id === selectedRequestId;
      const isFocused = rowIndex === focusedRowIndex;
      const isHovered = rowIndex === hoveredRowIndex;
      
      // Draw the row with all its columns
      drawRow(req, rowY, isSelected, isFocused, isHovered);
    }
    
    // Draw end of list indicator
    if (visibleStartIndex + rowsToDraw >= requests.length && requests.length > 0) {
      // Position the end indicator exactly after the last row
      const endY = requests.length * ROW_HEIGHT - scrollTop;
      
      if (endY <= canvasHeight) {
        // Ensure end indicator is always visible when scrolled to bottom
        const maxScroll = Math.max(0, totalContentHeight - canvasHeight);
        const isMaxScrolled = Math.abs(scrollTop - maxScroll) < 10; // With some tolerance
        
        const finalEndY = isMaxScrolled ? canvasHeight - ROW_HEIGHT / 2 : endY;
        
        // Use secondary accent color with low opacity for the end indicator
        ctx.fillStyle = getCssVar('--color-secondary-accent');
        ctx.globalAlpha = 0.1;
        ctx.fillRect(0, finalEndY, canvasWidth, ROW_HEIGHT / 2); // Half height indicator
        ctx.globalAlpha = 1.0; // Reset alpha
      }
    }
  }
  
  // Function to get method color as a CSS color value (not a class)
  function getMethodColorValue(method: string): string {
    // Get the CSS variable name (like "var(--color-method-get)")
    const cssVarName = getMethodColor(method);
    
    // Extract the variable name without the var() wrapper
    const varNameMatch = cssVarName.match(/var\((.*?)\)/);
    if (!varNameMatch || !varNameMatch[1]) return '#ffffff'; // Fallback to white
    
    const varName = varNameMatch[1];
    
    // Get the actual color value from CSS using our helper
    const computedValue = getCssVar(varName);
    
    // Return the computed color or fallback to method-specific colors if not found
    if (computedValue !== '') {
      return computedValue;
    }
    
    // Fallback colors if CSS variables aren't available
    switch (method.toUpperCase()) {
      case 'GET': return '#2563eb';     // blue-600
      case 'POST': return '#16a34a';    // green-600 
      case 'PUT': return '#ca8a04';     // yellow-600
      case 'DELETE': return '#dc2626';  // red-600
      case 'PATCH': return '#9333ea';   // purple-600
      default: return '#4b5563';        // gray-600
    }
  }
  
  // Handle global mouse move for column resizing
  function handleGlobalMouseMove(e: MouseEvent) {
    if (isResizing) {
      const deltaX = e.clientX - resizeStartX;
      
      // Ensure we maintain minimum width and don't allow resizing too small
      const minWidth = Math.max(columns[resizingColumnIndex].minWidth, 30); // At least 30px to prevent tiny columns
      const newWidth = Math.max(minWidth, resizeStartWidth + deltaX);
      
      // Update column width
      columns[resizingColumnIndex].width = newWidth;
      
      // Update total columns width and trigger canvas resize
      totalColumnsWidth = getTotalColumnsWidth();
      resizeCanvas();
    }
  }
  
  // Set up the component
  onMount(() => {
    if (canvasRef) {
      // Use optimized context settings
      ctx = canvasRef.getContext('2d', { 
        alpha: false,
        willReadFrequently: false,
        desynchronized: true // Hardware acceleration
      })!;
      
      // Initialize services
      columnManager = new TableColumnManager(columns, COLUMN_RIGHT_PADDING);
      
      const config: TableConfig = {
        rowHeight: ROW_HEIGHT,
        headerHeight: HEADER_HEIGHT,
        headerPaddingLeft: HEADER_PADDING_LEFT,
        rowPaddingLeft: ROW_PADDING_LEFT,
        resizeHandleWidth: RESIZE_HANDLE_WIDTH,
        resizeHandleOffset: RESIZE_HANDLE_OFFSET
      };
      
      // No longer needed - using selected flow directly
      
      renderer = new CanvasRenderer(
        canvasRef,
        ctx,
        requests,
        columns,
        config,
        {
          getSelectedId: () => selectedRequestId,
          getFocusedIndex: () => focusedRowIndex, 
          getHoveredIndex: () => hoveredRowIndex,
          getVisibleRange: () => ({ start: visibleStartIndex, end: visibleEndIndex }),
          getScrollTop: () => canvasContainerRef?.scrollTop || 0,
          getMethodColor: getMethodColorValue
        }
      );
      
      keyboardManager = new TableKeyboardManager({
        getRequests: () => requests,
        getFocusedIndex: () => focusedRowIndex,
        setFocusedIndex: (index) => {
          focusedRowIndex = index;
          ensureRowVisible(focusedRowIndex);
          if (selectedRequestId !== null) {
            dispatch('select', requests[focusedRowIndex]);
          }
          renderer.redraw();
        },
        hasSelectedItem: () => selectedRequestId !== null,
        closeDetails: () => dispatch('close'),
        selectItem: (index) => dispatch('select', requests[index])
      });
      
      // Initial setup
      totalColumnsWidth = getTotalColumnsWidth();
      resizeCanvas();
      // Calculate initial total content height
      totalContentHeight = requests.length * ROW_HEIGHT + ROW_HEIGHT/2;
      updateVisibleRange();
      
      // Ensure canvas has proper styling applied immediately (Svelte 5 fix)
      applyCanvasStyles();
      
      // Set up resize observer
      const resizeObserver = new ResizeObserver(() => {
        resizeCanvas();
      });
      resizeObserver.observe(canvasContainerRef);
      
      // Add window resize event
      window.addEventListener('resize', resizeCanvas);
      
      // Add window-level keyboard event listener instead of relying on tabindex
      document.addEventListener('keydown', handleKeyDown);
      
      // Add global mouse up handler for column resizing
      document.addEventListener('mouseup', handleGlobalMouseUp);
      document.addEventListener('mousemove', handleGlobalMouseMove);
      
      // Update selected index
      if (selectedRequestId) {
        const selectedIndex = requests.findIndex(req => req.id === selectedRequestId);
        if (selectedIndex >= 0) {
          focusedRowIndex = selectedIndex;
          ensureRowVisible(focusedRowIndex);
        }
      }
      
      return () => {
        resizeObserver.disconnect();
        document.removeEventListener('keydown', handleKeyDown);
        document.removeEventListener('mouseup', handleGlobalMouseUp);
        document.removeEventListener('mousemove', handleGlobalMouseMove);
        window.removeEventListener('resize', resizeCanvas);
        if (scrollRAF) {
          cancelAnimationFrame(scrollRAF);
        }
        renderer?.destroy();
        keyboardManager?.destroy();
      };
    }
  });
  
  // React to prop changes
  $: if (requests && renderer) {
    totalContentHeight = requests.length * ROW_HEIGHT + ROW_HEIGHT/2;
    updateVisibleRange();
    renderer.updateData(requests);
    renderer.redraw();
    // Ensure canvas styling is applied after data changes (Svelte 5 fix)
    applyCanvasStyles();
  }
  
  $: if (selectedRequestId) {
    if (renderer) {
      renderer.redraw();
    }
  }
  
  // Clean up on component unmount
  onDestroy(() => {
    if (ctx) {
      ctx = null as any;
    }
    renderer?.destroy();
    keyboardManager?.destroy();
  });
  
  // Handle context menu
  function handleContextMenuClose() {
    showContextMenu = false;
    contextMenuRequest = null;
  }
  
  // Copy request to clipboard for pasting in request flow tab
  async function copyRequestToClipboard() {
    if (!contextMenuRequest) return;
    
    try {
      // Call backend to copy request to clipboard
      await CopyRequestToClipboard(contextMenuRequest.id);
      
      // Show notification
      notificationMessage = "Request copied to clipboard";
      showCopiedNotification = true;
      setTimeout(() => {
        showCopiedNotification = false;
      }, 3000);
      
      // Close context menu
      handleContextMenuClose();
    } catch (error) {
      console.error('Failed to copy request:', error);
    }
  }
  
  // Copy request directly to the selected flow
  async function copyRequestToSelectedFlow() {
    if (!contextMenuRequest) return;
    
    try {
      // Call backend to copy request to the currently selected flow
      await CopyRequestToSelectedFlow(contextMenuRequest.id);
      
      // Show notification
      notificationMessage = "Request added to selected flow";
      showCopiedNotification = true;
      setTimeout(() => {
        showCopiedNotification = false;
      }, 3000);
      
      // Close context menu
      handleContextMenuClose();
    } catch (error) {
      console.error('Failed to copy request to selected flow:', error);
    }
  }
  
  // Get context menu items
  function getContextMenuItems() {
    const items = [
      { label: 'Copy Request', onClick: copyRequestToClipboard },
      { label: 'Copy to Current Flow', onClick: copyRequestToSelectedFlow }
    ];
    
    return items;
  }
  
  // Handle column sorting
  function handleSort(columnId: string) {
    let newSortDirection = 'asc';

    if (sortColumn === columnId) {
      // Toggle between asc and desc
      if (sortDirection === 'asc') {
        newSortDirection = 'desc';
      } else {
        newSortDirection = 'asc';
      }
    } else {
      // New column, start with asc
      newSortDirection = 'asc';
    }

    // Emit sorting event to parent component
    dispatch('sort', { column: columnId, direction: newSortDirection });
  }
</script>

<div class="flex flex-col h-full overflow-hidden">
  <!-- Main container with header and scrollable content -->
  <div class="h-full flex flex-col relative">
    <!-- Fixed header at the top - with horizontal scrolling that matches main content -->
    <div class="sticky top-0 left-0 shrink-0 h-6 z-10 overflow-hidden header-container">
      <div class="header-scroll-container overflow-x-auto w-full h-full" style="overflow-y: hidden;">
        <div style="width: {effectiveContainerWidth}px;" class="flex h-full">
          {#each columns as column, index}
            <ColumnHeader 
              {column}
              isLast={index === columns.length - 1}
              onResize={(width) => {
                columnManager.setColumnWidth(index, width);
                columns = [...columns]; // Ensure Svelte detects the change for the #each block
                totalColumnsWidth = columnManager.getTotalWidth();
                resizeCanvas();
              }}
              onSort={(columnId) => handleSort(columnId)}
              sortColumn={sortColumn}
              sortDirection={sortDirection}
            />
          {/each}
        </div>
      </div>
    </div>

    <!-- Main scrollable container supporting both horizontal and vertical scrolling -->
    <div 
      bind:this={canvasContainerRef} 
      class="flex-1 overflow-auto overscroll-none"
      style="overscroll-behavior: none; will-change: scroll-position;"
      on:scroll={handleScroll}
    >
      <div class="relative" style="width: {effectiveContainerWidth}px; height: {totalContentHeight}px;">
        <!-- Canvas element positioned at the top of the scrollable area -->
        <canvas 
          bind:this={canvasRef} 
          class=""
          on:click={handleCanvasClick}
          on:contextmenu|preventDefault={handleCanvasClick}
          on:mousemove={handleCanvasMouseMove}
          on:mouseleave={handleCanvasMouseLeave}
        ></canvas>
      </div>
    </div>
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

<!-- Copy notification -->
{#if showCopiedNotification}
  <Notification 
    message={notificationMessage}
    type="success"
    duration={3000}
  />
{/if}

<style>
  /* Ensure canvas receives pointer events */
  canvas {
    pointer-events: auto !important; 
  }
  
  /* Fixed header container with explicit background */
  .header-container {
    background-color: var(--color-header-bg);
    user-select: none;
    position: relative;
  }
  
  /* Hide scrollbar in header but keep functionality */
  .header-scroll-container::-webkit-scrollbar {
    display: none;
  }
  
  .header-scroll-container {
    -ms-overflow-style: none;
    scrollbar-width: none;
  }
  
  /* Apply to canvas container specifically */
  :global(.overflow-auto) {
    outline: none !important;
  }
  
  /* Apply to parent element */
  :global(.flex-col) {
    outline: none !important;
  }
  
  /* Override browser-specific focus styles */
  *:focus {
    outline: none !important;
  }
</style> 