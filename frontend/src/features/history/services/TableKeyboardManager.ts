import type { KeyboardHandlers } from '../../../types/TableTypes';

/**
 * Service for handling keyboard navigation in tables
 */
export class TableKeyboardManager<T> {
  private handlers: KeyboardHandlers<T>;

  /**
   * Creates a new TableKeyboardManager
   * @param handlers Keyboard event handlers
   */
  constructor(handlers: KeyboardHandlers<T>) {
    this.handlers = handlers;
    this.handleKeyDown = this.handleKeyDown.bind(this);
  }

  /**
   * Handles keyboard events for navigation
   * @param e Keyboard event
   */
  handleKeyDown(e: KeyboardEvent): void {
    // Only handle if no input is focused
    if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) {
      return;
    }
    
    // If no row is currently focused but we have a selected item, start navigation from there
    if (this.handlers.getFocusedIndex() === -1 && this.handlers.hasSelectedItem()) {
      const requests = this.handlers.getRequests();
      if (requests.length > 0) {
        this.handlers.setFocusedIndex(0);
      }
    }
    
    // Update focused row
    const updateFocusedRow = (newIndex: number) => {
      const requests = this.handlers.getRequests();
      if (newIndex >= 0 && newIndex < requests.length) {
        this.handlers.setFocusedIndex(newIndex);
      }
    };
    
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        const requests = this.handlers.getRequests();
        const currentIndex = this.handlers.getFocusedIndex();
        
        if (currentIndex < requests.length - 1) {
          updateFocusedRow(currentIndex + 1);
        } else if (currentIndex === -1 && requests.length > 0) {
          updateFocusedRow(0);
        }
        break;
        
      case 'ArrowUp':
        e.preventDefault();
        const requestsList = this.handlers.getRequests();
        const focusedIndex = this.handlers.getFocusedIndex();
        
        if (focusedIndex > 0) {
          updateFocusedRow(focusedIndex - 1);
        } else if (focusedIndex === -1 && requestsList.length > 0) {
          updateFocusedRow(0);
        }
        break;
        
      case 'Enter':
        e.preventDefault();
        const reqList = this.handlers.getRequests();
        const curIndex = this.handlers.getFocusedIndex();
        
        if (curIndex >= 0 && curIndex < reqList.length) {
          this.handlers.selectItem(curIndex);
        }
        break;
        
      case 'Home':
        e.preventDefault();
        const allRequests = this.handlers.getRequests();
        if (allRequests.length > 0) {
          updateFocusedRow(0);
        }
        break;
        
      case 'End':
        e.preventDefault();
        const endRequests = this.handlers.getRequests();
        if (endRequests.length > 0) {
          updateFocusedRow(endRequests.length - 1);
        }
        break;
        
      case 'Escape':
        e.preventDefault();
        if (this.handlers.hasSelectedItem()) {
          this.handlers.closeDetails();
        }
        break;
        
      default:
        // Don't prevent default for unhandled keys
        break;
    }
  }

  /**
   * Clean up resources
   */
  destroy(): void {
    // No resources to clean up in this case
  }
} 