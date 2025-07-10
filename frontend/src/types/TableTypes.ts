/**
 * Column definition for table components
 */
export interface Column {
  /** Unique column identifier */
  id: string;
  /** Display name for the column */
  name: string;
  /** Column width in pixels */
  width: number;
  /** Whether the column can be resized */
  resizable: boolean;
  /** Minimum width in pixels */
  minWidth: number;
}

/**
 * Configuration for table rendering
 */
export interface TableConfig {
  /** Height of each row in pixels */
  rowHeight: number;
  /** Height of the header in pixels */
  headerHeight: number;
  /** Left padding for the header cells */
  headerPaddingLeft: number;
  /** Left padding for the row cells */
  rowPaddingLeft: number;
  /** Width of the resize handle in pixels */
  resizeHandleWidth: number;
  /** Offset for the resize handle */
  resizeHandleOffset: number;
}

/**
 * Context data for rendering decisions
 */
export interface RenderContext<T> {
  /** Gets the currently selected item ID */
  getSelectedId: () => string | null;
  /** Gets the index of the focused row */
  getFocusedIndex: () => number;
  /** Gets the index of the hovered row */
  getHoveredIndex: () => number;
  /** Gets the visible range of rows */
  getVisibleRange: () => { start: number; end: number };
  /** Gets the current scroll top position */
  getScrollTop: () => number;
  /** Gets the color for an HTTP method */
  getMethodColor: (method: string) => string;
}

/**
 * Interface for keyboard navigation handlers
 */
export interface KeyboardHandlers<T> {
  /** Gets the current list of items */
  getRequests: () => T[];
  /** Gets the index of the focused row */
  getFocusedIndex: () => number;
  /** Sets the index of the focused row */
  setFocusedIndex: (index: number) => void;
  /** Checks if an item is selected */
  hasSelectedItem: () => boolean;
  /** Closes the details panel */
  closeDetails: () => void;
  /** Selects an item at the given index */
  selectItem: (index: number) => void;
} 