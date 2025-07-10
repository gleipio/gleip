import { writable } from 'svelte/store';
import { GetInterceptEnabled } from '../../../../wailsjs/go/backend/App';

// Create writable store for intercept state
export const interceptEnabled = writable(false);

// Initialize the store with the actual backend state
export async function initializeInterceptStore() {
  try {
    const interceptState = await GetInterceptEnabled();
    interceptEnabled.set(interceptState);
    console.log('Intercept enabled state initialized:', interceptState);
  } catch (error) {
    console.error('Failed to initialize intercept store:', error);
    // Keep default values
  }
}

// Helper function to update the intercept enabled state
export function updateInterceptState(enabled: boolean) {
  interceptEnabled.set(enabled);
} 