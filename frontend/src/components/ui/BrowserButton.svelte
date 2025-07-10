<script lang="ts">
  import { onMount } from 'svelte';
  import { LaunchFirefoxBrowser, IsFirefoxRunning, CheckFirefoxInstallation } from '../../../wailsjs/go/backend/App';
  import { EventsEmit } from '../../../wailsjs/runtime/runtime';
  import Button from './Button.svelte';

  // Props
  export let variant: 'primary' | 'secondary' = 'primary';
  export let size: 'sm' | 'md' | 'lg' = 'sm';
  export let style: string = 'white-space: nowrap;';

  // Firefox state
  let isLaunchingFirefox = false;
  let firefoxLaunched = false;
  let isFirefoxRunning = false;
  let isFirefoxInstalled = false;
  let checkInterval: number | undefined;

  // Check if Firefox is installed
  async function checkFirefoxInstallation() {
    try {
      const installed = await CheckFirefoxInstallation();
      isFirefoxInstalled = installed;
    } catch (error) {
      console.error('Failed to check Firefox installation status:', error);
      isFirefoxInstalled = false;
    }
  }

  // Check if Firefox is running
  async function checkFirefoxStatus() {
    try {
      const running = await IsFirefoxRunning();
      isFirefoxRunning = running;
      
      // If Firefox is running, we check more frequently to detect when it's closed
      if (running) {
        if (!checkInterval) {
          // Set up an interval to check Firefox status more frequently when it's running
          checkInterval = window.setInterval(async () => {
            const stillRunning = await IsFirefoxRunning();
            isFirefoxRunning = stillRunning;
            
            // Clear the interval when Firefox is no longer running
            if (!stillRunning && checkInterval) {
              window.clearInterval(checkInterval);
              checkInterval = undefined;
            }
          }, 1000); // Check every second when Firefox is running
        }
      } else if (!running && checkInterval) {
        // Firefox is not running, clear any active interval
        window.clearInterval(checkInterval);
        checkInterval = undefined;
      }
    } catch (error) {
      console.error('Failed to check Firefox status:', error);
    }
  }

  // Handle Firefox button click
  async function handleFirefoxButton() {
    // If Firefox is not installed, navigate to the settings page
    if (!isFirefoxInstalled) {
      // Instead of using a custom event, directly use EventsEmit to trigger navigation
      EventsEmit("navigate", "/settings");
      return;
    }
    
    if (isLaunchingFirefox) return;
    
    isLaunchingFirefox = true;
    firefoxLaunched = false;
    
    try {
      await LaunchFirefoxBrowser();
      firefoxLaunched = true;
      checkFirefoxStatus(); // Immediately check status
      
      // Auto-hide the success message after 5 seconds
      setTimeout(() => {
        firefoxLaunched = false;
      }, 5000);
    } catch (error) {
      console.error('Failed to interact with Firefox:', error);
    } finally {
      isLaunchingFirefox = false;
    }
  }

  onMount(() => {
    // Check Firefox installation and status
    checkFirefoxInstallation();
    checkFirefoxStatus();
    
    // Set up an interval to check Firefox status at a lower frequency
    const interval = setInterval(() => {
      if (!checkInterval) { // Only check if the high-frequency check isn't running
        checkFirefoxStatus();
      }
    }, 5000); // Check every 5 seconds as a background check
    
    return () => {
      // Clear intervals
      clearInterval(interval);
      if (checkInterval) {
        clearInterval(checkInterval);
      }
    };
  });
</script>

<div class="flex items-center gap-2">
  {#if firefoxLaunched}
    <div class="fixed bottom-4 right-4 z-50 bg-green-900/80 text-green-200 text-sm py-2 px-3 rounded-md shadow-md">
      Browser Ready!
    </div>
  {/if}
  <Button 
    on:click={handleFirefoxButton}
    disabled={isLaunchingFirefox}
    {variant}
    {size}
    {style}
  >
    {#if isLaunchingFirefox}
      <svg class="animate-spin h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
      </svg>
    {/if}
    {isLaunchingFirefox 
      ? 'Starting...' 
      : !isFirefoxInstalled
        ? 'Install Browser'
        : isFirefoxRunning 
          ? 'Show Browser' 
          : 'Launch Browser'
    }
  </Button>
</div> 