import type { Column } from '../../../types/TableTypes';

/**
 * Service for managing table column sizing and properties
 */
export class TableColumnManager {
  private columns: Column[];
  private rightPadding: number;

  /**
   * Creates a new TableColumnManager
   * @param columns Initial column definitions
   * @param rightPadding Extra right padding for the last column
   */
  constructor(columns: Column[], rightPadding: number = 0) {
    this.columns = [...columns]; // Create a copy to avoid reference issues
    this.rightPadding = rightPadding;
  }

  /**
   * Gets the total width of all columns
   * @returns Total width in pixels
   */
  getTotalWidth(): number {
    return this.columns.reduce((total, column) => total + column.width, 0) + this.rightPadding;
  }

  /**
   * Sets the width of a specific column
   * @param index Column index
   * @param width New width in pixels
   */
  setColumnWidth(index: number, width: number): void {
    if (index >= 0 && index < this.columns.length) {
      const minWidth = Math.max(this.columns[index].minWidth, 30);
      this.columns[index].width = Math.max(minWidth, width);
    }
  }

  /**
   * Gets column positions for boundaries
   * @param headerPaddingLeft Left padding for headers
   * @returns Array of column boundary positions
   */
  getColumnPositions(headerPaddingLeft: number): number[] {
    const positions: number[] = []; 
    let xOffset = headerPaddingLeft; 
    
    this.columns.forEach(column => {
      positions.push(xOffset); // Start of column
      xOffset += column.width;
      positions.push(xOffset); // End of column
    });
    
    return positions;
  }

  /**
   * Gets column boundaries for visual indicators
   * @param headerPaddingLeft Left padding for headers
   * @returns Array of column boundary positions
   */
  getColumnBoundaries(headerPaddingLeft: number): number[] {
    const boundaries: number[] = [];
    let xOffset = headerPaddingLeft;
    
    for (let i = 0; i < this.columns.length; i++) {
      xOffset += this.columns[i].width;
      boundaries.push(xOffset - 8); // Adjust for visual alignment
    }
    
    return boundaries;
  }

  /**
   * Gets all columns
   * @returns Array of column definitions
   */
  getColumns(): Column[] {
    return [...this.columns];
  }
} 