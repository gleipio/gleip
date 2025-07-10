// DOM-related utility functions

/**
 * Gets a CSS variable value from the document root
 * @param name CSS variable name
 * @param fallback Fallback value if variable is not found
 * @returns The value of the CSS variable or fallback
 */
export function getCssVar(name: string, fallback: string = ''): string {
  // Ensure name has the proper format for getPropertyValue
  const varName = name.startsWith('--') ? name : `--${name}`;
  const value = getComputedStyle(document.documentElement).getPropertyValue(varName).trim();
  
  // If empty, return the fallback
  if (!value) {
    console.log(`CSS variable ${varName} not found, using fallback: ${fallback}`);
    return fallback;
  }
  
  return value;
}

/**
 * Truncates text with ellipsis based on maximum width
 * @param text Text to truncate
 * @param maxWidth Maximum width in pixels
 * @param context Canvas rendering context for text measurement
 * @returns Truncated text with ellipsis if needed
 */
export function truncateWithEllipsis(text: string, maxWidth: number, context: CanvasRenderingContext2D): string {
  if (context.measureText(text).width <= maxWidth) return text;
  
  for (let i = 0; i < text.length; i++) {
    const testText = text.substring(0, i) + '...';
    if (context.measureText(testText).width > maxWidth) {
      return text.substring(0, i - 1) + '...';
    }
  }
  return text; // Fallback
} 