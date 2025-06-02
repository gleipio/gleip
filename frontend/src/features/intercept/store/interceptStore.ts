import { writable } from 'svelte/store';
import { GetInterceptEnabled } from '../../../../wailsjs/go/backend/App';

// Create a writable store for intercept state
export const interceptEnabled = writable(false);

// Initialize the store with the actual backend state
export async function initializeInterceptStore() {
  try {
    const state = await GetInterceptEnabled();
    interceptEnabled.set(state);
    console.log('Intercept store initialized with state:', state);
  } catch (error) {
    console.error('Failed to initialize intercept store:', error);
    // Keep default false value
  }
}

// Helper function to update the store when intercept state changes
export function updateInterceptState(enabled: boolean) {
  interceptEnabled.set(enabled);
} 