<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { LaunchFirefoxBrowser, CheckFirefoxInstallation, DownloadFirefox, GetFirefoxDownloadProgress, IsFirefoxDownloading, UninstallFirefox } from '../../../wailsjs/go/backend/App';
  import Button from './Button.svelte';

  // Interface definitions
  type DownloadProgress = {
    progress: number;
    status: string;
    error?: string;
  };

  // Props for customization
  export let onError: ((error: string) => void) | null = null;

  // State
  let isLaunching = false;
  let firefoxLaunched = false;
  let isFirefoxInstalled = false;
  let isDownloading = false;
  let isUninstalling = false;
  let downloadProgress: DownloadProgress = {
    progress: 0,
    status: ''
  };
  let progressCheckInterval: number | undefined;

  // Functions
  function handleError(err: unknown) {
    const errorMessage = err instanceof Error ? err.message : String(err);
    if (onError) {
      onError(errorMessage);
    } else {
      console.error('Firefox Manager Error:', errorMessage);
    }
  }

  async function checkFirefoxInstallation() {
    try {
      isFirefoxInstalled = await CheckFirefoxInstallation();
    } catch (err) {
      handleError(err);
    }
  }

  async function launchBrowser() {
    isLaunching = true;
    
    try {
      await LaunchFirefoxBrowser();
      firefoxLaunched = true;
      
      // Auto-hide the success message after 5 seconds
      setTimeout(() => {
        firefoxLaunched = false;
      }, 5000);
    } catch (err) {
      handleError(err);
    } finally {
      isLaunching = false;
    }
  }

  async function downloadFirefox() {
    try {
      isDownloading = true;
      await DownloadFirefox();
      
      // Check progress
      progressCheckInterval = window.setInterval(async () => {
        try {
          const isStillDownloading = await IsFirefoxDownloading();
          if (!isStillDownloading) {
            clearInterval(progressCheckInterval);
            isDownloading = false;
            await checkFirefoxInstallation();
          } else {
            downloadProgress = await GetFirefoxDownloadProgress();
          }
        } catch (err) {
          clearInterval(progressCheckInterval);
          isDownloading = false;
          handleError(err);
        }
      }, 500);
    } catch (err) {
      isDownloading = false;
      handleError(err);
    }
  }

  async function uninstallFirefox() {
    try {
      isUninstalling = true;
      await UninstallFirefox();
      await checkFirefoxInstallation();
    } catch (err) {
      handleError(err);
    } finally {
      isUninstalling = false;
    }
  }

  onMount(async () => {
    await checkFirefoxInstallation();
    
    // Check if still downloading
    const stillDownloading = await IsFirefoxDownloading();
    if (stillDownloading) {
      isDownloading = true;
      
      // Check progress
      progressCheckInterval = window.setInterval(async () => {
        try {
          const isStillDownloading = await IsFirefoxDownloading();
          if (!isStillDownloading) {
            clearInterval(progressCheckInterval);
            isDownloading = false;
            await checkFirefoxInstallation();
          } else {
            downloadProgress = await GetFirefoxDownloadProgress();
          }
        } catch (err) {
          clearInterval(progressCheckInterval);
          isDownloading = false;
          handleError(err);
        }
      }, 500);
    }
  });

  onDestroy(() => {
    if (progressCheckInterval) {
      clearInterval(progressCheckInterval);
    }
  });
</script>

{#if firefoxLaunched}
  <div class="fixed bottom-4 right-4 z-50 bg-green-900/80 text-green-200 text-sm py-2 px-3 rounded-md shadow-md">
    Browser Ready!
  </div>
{/if}

<div class="bg-gray-800/50 rounded-lg p-3 border border-gray-700/50">
  <div class="flex justify-between items-center mb-1">
    <h3 class="text-base font-semibold text-gray-50">Firefox Browser</h3>
    <p class="text-gray-500 text-xs">
      Status: <span class="text-gray-100">{isFirefoxInstalled ? 'Installed' : 'Not installed'}</span>
    </p>
  </div>
  
  <p class="text-gray-50 text-sm mb-4">
    Dedicated Firefox browser with proxy pre-configured.
  </p>
  
  <!-- Main Action Button (Mutually Exclusive) -->
  {#if isFirefoxInstalled}
    <Button 
      on:click={launchBrowser}
      disabled={isLaunching}
      fullWidth={true}
      loading={isLaunching}
      variant="primary"
      size="md"
    >
      {isLaunching ? 'Launching...' : 'Launch Browser'}
    </Button>
  {:else if isDownloading}
    <div>
      <p class="text-gray-50 text-xs mb-1">Downloading: {downloadProgress?.status}</p>
      <div class="w-full bg-gray-700 rounded-full h-1.5 mb-1">
        <div class="bg-[var(--color-midnight-accent)] h-1.5 rounded-full" style="width: {downloadProgress?.progress * 100}%"></div>
      </div>
      <p class="text-gray-500 text-xs text-right">{(downloadProgress?.progress * 100).toFixed(1)}%</p>
    </div>
  {:else}
    <Button 
      on:click={downloadFirefox}
      disabled={isDownloading}
      fullWidth={true}
      loading={isDownloading}
      variant="primary"
      size="md"
    >
      Download Firefox
    </Button>
  {/if}
  
  <!-- Secondary Action -->
  {#if isFirefoxInstalled}
    <div class="flex justify-end mt-3 text-xs">
      <Button 
        on:click={uninstallFirefox}
        disabled={isUninstalling}
        variant="link"
        size="xs"
        withGradientHover={false}
        class={isUninstalling ? 'opacity-50 cursor-not-allowed' : 'text-gray-500 hover:text-red-400'}
      >
        {isUninstalling ? 'Uninstalling...' : 'Uninstall'}
      </Button>
    </div>
  {/if}
</div> 