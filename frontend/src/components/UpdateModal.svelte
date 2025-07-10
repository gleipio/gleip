<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
  import Button from './ui/Button.svelte';

  type UpdateInfo = {
    currentVersion: string;
    latestVersion: string;
    releaseNotes: string;
    downloadUrl: string;
    isUpdateNeeded: boolean;
  };

  // Update state
  let showUpdateModal = false;
  let updateInfo: UpdateInfo | null = null;
  let isInstallingUpdate = false;
  let updateStatus = "";

  async function installUpdate() {
    try {
      isInstallingUpdate = true;
      showUpdateModal = false;
      
      if (updateInfo && updateInfo.downloadUrl) {
        // @ts-ignore - This will be available at runtime when Wails builds the app
        await window.go.backend.App.InstallUpdate(updateInfo.downloadUrl);
      } else {
        throw new Error("No update information available");
      }
    } catch (err) {
      if (err instanceof Error) {
        console.error(err.message);
      } else {
        console.error(String(err));
      }
      isInstallingUpdate = false;
    }
  }

  onMount(() => {
    // Set up event listeners for updates
    EventsOn("update:available", (info: UpdateInfo) => {
      updateInfo = info;
      showUpdateModal = true;
    });
    
    EventsOn("update:downloading", () => {
      updateStatus = "Downloading update...";
      isInstallingUpdate = true;
    });
    
    EventsOn("update:extracting", () => {
      updateStatus = "Extracting update...";
    });
    
    EventsOn("update:launching", () => {
      updateStatus = "Launching new version...";
    });
    
    EventsOn("update:complete", () => {
      updateStatus = "Update complete! Restarting application...";
    });
  });

  onDestroy(() => {
    // Clean up event listeners
    EventsOff("update:available");
    EventsOff("update:downloading");
    EventsOff("update:extracting");
    EventsOff("update:launching");
    EventsOff("update:complete");
  });
</script>

<!-- Update Modal -->
{#if showUpdateModal && updateInfo}
  <div class="fixed inset-0 bg-black/50 z-50 flex items-center justify-center">
    <div class="bg-gray-800 rounded-lg p-4 max-w-md border border-gray-700 shadow-lg">
      <h3 class="text-lg font-medium text-gray-50 mb-2">Update Available</h3>
      <p class="text-sm text-gray-100 mb-4">
        A new version of Gleip is available: {updateInfo.latestVersion}
      </p>
      {#if updateInfo.releaseNotes}
        <div class="bg-gray-900/50 rounded p-3 mb-4 max-h-60 overflow-y-auto">
          <h4 class="text-sm font-medium text-gray-100 mb-1">Release Notes:</h4>
          <div class="text-xs text-gray-50 whitespace-pre-line">
            {updateInfo.releaseNotes}
          </div>
        </div>
      {/if}
      <div class="flex justify-end space-x-2">
        <Button 
          on:click={() => showUpdateModal = false}
          variant="link"
          size="sm"
          withGradientHover={false}
          class="text-gray-50"
        >
          Later
        </Button>
        <Button 
          on:click={installUpdate}
          variant="primary"
          size="sm"
        >
          Update Now
        </Button>
      </div>
    </div>
  </div>
{/if}

<!-- Installation Progress Modal -->
{#if isInstallingUpdate && updateStatus}
  <div class="fixed inset-0 bg-black/50 z-50 flex items-center justify-center">
    <div class="bg-gray-800 rounded-lg p-4 max-w-md border border-gray-700 shadow-lg">
      <h3 class="text-lg font-medium text-gray-50 mb-2">Installing Update</h3>
      <p class="text-sm text-gray-100 mb-4">
        {updateStatus}
      </p>
      <div class="w-full bg-gray-700 rounded-full h-1.5 mb-4">
        <div class="bg-[var(--color-midnight-accent)] h-1.5 rounded-full animate-pulse w-full"></div>
      </div>
      <p class="text-xs text-gray-50 text-center">Please wait while the update is applied...</p>
    </div>
  </div>
{/if} 