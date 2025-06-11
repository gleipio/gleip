import type { Column, TableConfig, RenderContext } from '../../../types/TableTypes';
import { getCssVar, truncateWithEllipsis } from '../../../shared/utils/domHelpers';

/**
 * Service for rendering canvas-based tables
 */
export class CanvasRenderer<T extends { id: string }> {
  private canvas: HTMLCanvasElement;
  private ctx: CanvasRenderingContext2D;
  private data: T[];
  private columns: Column[];
  private config: TableConfig;
  private renderContext: RenderContext<T>;
  private dpr: number = 1;
  private canvasWidth: number = 0;
  private canvasHeight: number = 0;

  /**
   * Creates a new CanvasRenderer
   * @param canvas Canvas element to render on
   * @param ctx Canvas rendering context
   * @param data Data to render
   * @param columns Column definitions
   * @param config Table configuration
   * @param renderContext Context for rendering decisions
   */
  constructor(
    canvas: HTMLCanvasElement,
    ctx: CanvasRenderingContext2D,
    data: T[],
    columns: Column[],
    config: TableConfig,
    renderContext: RenderContext<T>
  ) {
    this.canvas = canvas;
    this.ctx = ctx;
    this.data = [...data];
    this.columns = [...columns];
    this.config = { ...config };
    this.renderContext = renderContext;
  }

  /**
   * Updates the data to be rendered
   * @param data New data
   */
  updateData(data: T[]): void {
    this.data = [...data];
    
    // Force a redraw to ensure rows are rendered
    this.redraw();
  }

  /**
   * Resizes the canvas
   * @param width New width
   * @param height New height
   * @param dpr Device pixel ratio
   */
  resize(width: number, height: number, dpr: number): void {
    // console.log(`Resizing canvas to ${width}x${height}, DPR: ${dpr}`);
    this.canvasWidth = width;
    this.canvasHeight = height;
    this.dpr = dpr;

    // Set the canvas to its physical size (this affects resolution)
    this.canvas.width = Math.round(width * dpr);
    this.canvas.height = Math.round(height * dpr);
    
    // Preserve existing positioning styles while updating size
    const currentTransform = this.canvas.style.transform;
    const currentPosition = this.canvas.style.position;
    const currentTop = this.canvas.style.top;
    const currentLeft = this.canvas.style.left;
    
    // Set the display size via CSS (this affects layout)
    this.canvas.style.width = `${width}px`;
    this.canvas.style.height = `${height}px`;
    
    // Restore positioning styles if they existed (Svelte 5 fix)
    if (currentTransform) this.canvas.style.transform = currentTransform;
    if (currentPosition) this.canvas.style.position = currentPosition;
    if (currentTop) this.canvas.style.top = currentTop;
    if (currentLeft) this.canvas.style.left = currentLeft;
    
    // Reset the transformation matrix and apply DPR scaling
    this.ctx.setTransform(1, 0, 0, 1, 0, 0);
    this.ctx.scale(dpr, dpr);
    
    // Force a redraw after resize
    this.redraw();
  }

  /**
   * Redraws the canvas contents
   */
  redraw(): void {
    if (!this.ctx || !this.canvas) return;
    
    // If no data, draw empty state and return
    if (!this.data.length) {
      this.drawEmptyState();
      return;
    }
    
    // Clear entire canvas
    this.ctx.clearRect(0, 0, this.canvas.width / this.dpr, this.canvas.height / this.dpr);
    
    // Get scroll position from context
    const scrollTop = this.renderContext.getScrollTop();
    
    // Get visible range
    const { start: visibleStartIndex, end: visibleEndIndex } = this.renderContext.getVisibleRange();
    
    // Calculate how many rows to draw
    const rowsToDraw = Math.min(
      this.data.length - visibleStartIndex,
      Math.ceil(this.canvasHeight / this.config.rowHeight) + 20 // Add extra buffer
    );
    
    // CRITICAL: Log information for debugging
    // console.log(`Drawing ${rowsToDraw} rows, visible range: ${visibleStartIndex}-${visibleEndIndex}, total rows: ${this.data.length}`);
    
    // Iterate through visible rows
    for (let i = 0; i < rowsToDraw; i++) {
      const rowIndex = visibleStartIndex + i;
      if (rowIndex >= this.data.length) break;
      
      const item = this.data[rowIndex];
      
      // Calculate precise row position based on exact scroll offset
      const rowStartY = rowIndex * this.config.rowHeight;
      const rowY = rowStartY - scrollTop;
      
      // Skip rows that are completely outside the visible area with margin
      if (rowY < -this.config.rowHeight || rowY > this.canvasHeight) continue;
      
      const isSelected = item.id === this.renderContext.getSelectedId();
      const isFocused = rowIndex === this.renderContext.getFocusedIndex();
      const isHovered = rowIndex === this.renderContext.getHoveredIndex();
      
      // Log drawing information for this row
      if (i < 3 || rowIndex === this.renderContext.getFocusedIndex()) {
        // console.log(`Drawing row ${rowIndex} at Y=${rowY}, selected=${isSelected}, focused=${isFocused}`);
      }
      
      // Draw the row
      this.drawRow(item, rowY, isSelected, isFocused, isHovered);
    }
    
    // Draw end of list indicator
    if (visibleStartIndex + rowsToDraw >= this.data.length && this.data.length > 0) {
      this.drawEndIndicator(scrollTop);
    }
  }

  /**
   * Draws the empty state message
   */
  private drawEmptyState(): void {
    this.ctx.clearRect(0, 0, this.canvas.width / this.dpr, this.canvas.height / this.dpr);
    this.ctx.font = '20px system-ui, -apple-system, sans-serif';
    this.ctx.fillStyle = getCssVar('--color-gray-400');
    this.ctx.textAlign = 'center';
    this.ctx.textBaseline = 'middle';
    
    // Get the input element to check if there's a search query
    const searchInput = document.querySelector('input[placeholder="Search requests..."]') as HTMLInputElement;
    const hasSearchQuery = searchInput && searchInput.value.trim() !== '';
    
    if (hasSearchQuery) {
      // Show message for no search results
      this.ctx.fillText('No matching requests found', Math.round(this.canvasWidth / 2), Math.round(this.canvasHeight / 2));
    } else {
      // Show message for no requests at all
      this.ctx.fillText('No requests captured yet', Math.round(this.canvasWidth / 2), Math.round(this.canvasHeight / 2));
    }
  }

  /**
   * Draws the end of list indicator
   * @param scrollTop Current scroll position
   */
  private drawEndIndicator(scrollTop: number): void {
    // Position the end indicator exactly after the last row
    const endY = this.data.length * this.config.rowHeight - scrollTop;
    
    if (endY <= this.canvasHeight) {
      // Calculate total content height
      const totalContentHeight = this.data.length * this.config.rowHeight + this.config.rowHeight/2;
      
      // Ensure end indicator is always visible when scrolled to bottom
      const maxScroll = Math.max(0, totalContentHeight - this.canvasHeight);
      const isMaxScrolled = Math.abs(scrollTop - maxScroll) < 10; // With some tolerance
      
      const finalEndY = isMaxScrolled ? this.canvasHeight - this.config.rowHeight / 2 : endY;
      
      // Use secondary accent color with low opacity for the end indicator
      this.ctx.fillStyle = getCssVar('--color-secondary-accent');
      this.ctx.globalAlpha = 0.1;
      this.ctx.fillRect(0, finalEndY, this.canvasWidth, this.config.rowHeight / 2); // Half height indicator
      this.ctx.globalAlpha = 1.0; // Reset alpha
    }
  }

  /**
   * Draws a single row
   * @param item Data item to render
   * @param rowY Y-coordinate to draw at
   * @param isSelected Whether the row is selected
   * @param isFocused Whether the row is focused
   * @param isHovered Whether the row is hovered
   */
  private drawRow(item: any, rowY: number, isSelected: boolean, isFocused: boolean, isHovered: boolean): void {
    // Get the row's absolute index for alternating colors
    const rowIndex = this.data.indexOf(item);
    
    // Draw row background - this is what makes the rows visible!
    // Clear any existing color first to ensure we have a clean slate
    this.ctx.clearRect(0, rowY, this.canvasWidth, this.config.rowHeight);
    
    // Set the background color based on the row's state
    if (isSelected) {
      // Use midnight accent with opacity for selected rows
      this.ctx.fillStyle = getCssVar('--color-midnight-accent');
      this.ctx.globalAlpha = 0.2;
    } else if (isFocused) {
      // Use secondary accent with opacity for focused rows
      this.ctx.fillStyle = getCssVar('--color-secondary-accent');
      this.ctx.globalAlpha = 0.15;
    } else if (isHovered) {
      // Use dedicated hover color for better contrast
      this.ctx.fillStyle = getCssVar('--color-table-row-hover', '#2a2f45');
      this.ctx.globalAlpha = 1.0;
    } else {
      // Use absolute row index for consistent alternating colors
      // Ensure we always have a visible background color
      const evenRowColor = getCssVar('--color-table-row-even', '#1a1d2d');
      const oddRowColor = getCssVar('--color-table-row-odd', '#1e2235');
      this.ctx.fillStyle = rowIndex % 2 === 0 ? evenRowColor : oddRowColor;
      this.ctx.globalAlpha = 1.0;
    }
    
    // Fill the row background - CRITICAL for visibility
    this.ctx.fillRect(0, rowY, this.canvasWidth, this.config.rowHeight);
    this.ctx.globalAlpha = 1.0; // Reset alpha
    
    // Draw other row elements
    this.drawRowBorder(rowY);
    this.drawColumnDividers(rowY);
    this.drawSelectionIndicator(rowY, isSelected, isFocused);
    
    // Draw row content (text, icons, etc.)
    this.drawRowContent(item, rowY);
  }

  /**
   * Draws the row border
   * @param rowY Y-coordinate of the row
   */
  private drawRowBorder(rowY: number): void {
    // Draw row border - using the table border color for better contrast
    this.ctx.strokeStyle = getCssVar('--color-table-border', getCssVar('--color-midnight-darker'));
    this.ctx.globalAlpha = 0.6; // Increased from 0.5 for better visibility
    this.ctx.lineWidth = 1;
    const lineY = Math.floor(rowY + this.config.rowHeight) - 0.5;
    this.ctx.beginPath();
    this.ctx.moveTo(0, lineY);
    this.ctx.lineTo(this.canvasWidth, lineY);
    this.ctx.stroke();
    this.ctx.globalAlpha = 1.0; // Reset alpha
  }

  /**
   * Draws column dividers
   * @param rowY Y-coordinate of the row
   */
  private drawColumnDividers(rowY: number): void {
    // Get column boundaries for drawing indicators
    const columnBoundaries = this.getColumnBoundaries();
    
    // Draw column dividers for rows - using the table border color
    columnBoundaries.forEach((boundary, index) => {
      if (index < columnBoundaries.length - 1) {
        this.ctx.strokeStyle = getCssVar('--color-table-border', getCssVar('--color-midnight-darker'));
        this.ctx.globalAlpha = 0.5; // Increased from 0.4 for better visibility
        this.ctx.lineWidth = 1;
        this.ctx.beginPath();
        
        // Draw each column divider with its boundary position
        const exactLineX = Math.floor(boundary) + 0.5; // Add 0.5 for crisp lines
        this.ctx.moveTo(exactLineX, rowY);
        this.ctx.lineTo(exactLineX, rowY + this.config.rowHeight);
        this.ctx.stroke();
        this.ctx.globalAlpha = 1.0; // Reset alpha
      }
    });
  }

  /**
   * Draws selection/focus indicator
   * @param rowY Y-coordinate of the row
   * @param isSelected Whether the row is selected
   * @param isFocused Whether the row is focused
   */
  private drawSelectionIndicator(rowY: number, isSelected: boolean, isFocused: boolean): void {
    if (isSelected) {
      this.ctx.fillStyle = getCssVar('--color-midnight-accent');
      this.ctx.fillRect(0, rowY, 3, this.config.rowHeight);
    } else if (isFocused) {
      this.ctx.fillStyle = getCssVar('--color-secondary-accent');
      this.ctx.fillRect(0, rowY, 3, this.config.rowHeight);
    }
  }

  /**
   * Draws row content
   * @param req HTTP transaction data
   * @param rowY Y-coordinate of the row
   */
  private drawRowContent(req: any, rowY: number): void {
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
    this.ctx.font = '11px system-ui, -apple-system, sans-serif';
    this.ctx.fillStyle = getCssVar('--color-gray-300');
    this.ctx.textBaseline = 'middle';
    
    // Center text vertically
    const textY = Math.round(rowY + this.config.rowHeight / 2);
    
    // Draw all columns according to our column definitions
    let xOffset = this.config.rowPaddingLeft; // Initial padding
    
    // ID column - right-aligned
    this.ctx.fillStyle = getCssVar('--color-gray-200');
    this.ctx.font = 'bold 12px system-ui, -apple-system, sans-serif'; // Slightly larger and bold
    this.ctx.textAlign = 'right';
    this.ctx.fillText(req.seqNumber.toString(), Math.floor(xOffset + this.columns[0].width - this.config.rowPaddingLeft), textY);
    this.ctx.font = '11px system-ui, -apple-system, sans-serif'; // Reset font for other columns
    this.ctx.textAlign = 'left';
    xOffset += this.columns[0].width;
    
    // Host column
    this.ctx.fillStyle = getCssVar('--color-gray-300');
    const maxHostWidth = this.columns[1].width - 16; // Reduced from 24 to show more text
    this.ctx.fillText(truncateWithEllipsis(host, maxHostWidth, this.ctx), Math.floor(xOffset), textY);
    xOffset += this.columns[1].width;
    
    // Method column
    const methodColor = this.renderContext.getMethodColor(req.method);
    this.ctx.fillStyle = methodColor;
    this.ctx.font = '600 11px system-ui, -apple-system, sans-serif';
    this.ctx.fillText(req.method, Math.floor(xOffset), textY);
    this.ctx.font = '11px system-ui, -apple-system, sans-serif';
    xOffset += this.columns[2].width;
    
    // URL/Path column
    this.ctx.fillStyle = getCssVar('--color-gray-300');
    const maxUrlWidth = this.columns[3].width - 14; // Reduced from 16 to show more text
    this.ctx.fillText(truncateWithEllipsis(path, maxUrlWidth, this.ctx), Math.floor(xOffset), textY);
    xOffset += this.columns[3].width;
    
    // Params indicator - completely centered without any padding issues
    const paramsColumnIndex = 4; 
    const paramsColumnWidth = this.columns[paramsColumnIndex].width;
    
    // Calculate column boundaries for centering
    const columnBoundaries = this.getColumnBoundaries();
    
    // The start boundary is at the previous column's end
    const previousColumnEnd = columnBoundaries[paramsColumnIndex - 1];
    // The end boundary is this column's end
    const currentColumnEnd = columnBoundaries[paramsColumnIndex];
    
    // Calculate exact center of column based on boundaries
    const paramsColumnCenter = Math.floor((previousColumnEnd + currentColumnEnd) / 2);
    
    const hasPostParams = req.method === 'POST' || req.method === 'PUT' || req.method === 'PATCH';
    if (hasParams || hasPostParams) {
      // Draw the indicator dot exactly in the center of the column
      this.ctx.fillStyle = getCssVar('--color-success');
      this.ctx.beginPath();
      this.ctx.arc(paramsColumnCenter, textY, 4, 0, 2 * Math.PI);
      this.ctx.fill();
    }
    
    // Continue with normal xOffset tracking for other columns
    xOffset += paramsColumnWidth;
    
    // Length (Response size) - right-aligned
    this.ctx.fillStyle = getCssVar('--color-gray-300');
    let lengthText = "-";
    if (req.statusCode) { // If we have a response
      // Always display length in bytes with thousands separators
      const size = req.responseSize;
      lengthText = `${size.toLocaleString()}`;
    }
    this.ctx.textAlign = 'right';
    this.ctx.fillText(lengthText, Math.floor(xOffset + this.columns[5].width - 8), textY);
    this.ctx.textAlign = 'left';
    xOffset += this.columns[5].width;
    
    // Time column
    const time = new Date(req.timestamp).toLocaleTimeString();
    this.ctx.fillText(time, Math.floor(xOffset), textY);
  }

  /**
   * Gets column boundaries for visual indicators
   * @returns Array of column boundary positions
   */
  private getColumnBoundaries(): number[] {
    const boundaries: number[] = [];
    let xOffset = this.config.headerPaddingLeft;
    
    for (let i = 0; i < this.columns.length; i++) {
      xOffset += this.columns[i].width;
      boundaries.push(xOffset - 8); // Adjust for visual alignment
    }
    
    return boundaries;
  }

  /**
   * Cleans up resources
   */
  destroy(): void {
    // Release references to DOM objects
    this.canvas = null as any;
    this.ctx = null as any;
  }
} 