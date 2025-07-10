<script lang="ts">
  import { onMount } from 'svelte';
  import { GetAppVersion } from '../../../wailsjs/go/backend/App';
  import { GetSettings, UpdateSettings } from '../../../wailsjs/go/backend/SettingsController';
  import Button from '../../components/ui/Button.svelte';
  import FirefoxManager from '../../components/ui/FirefoxManager.svelte';
  import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime';

  // Interface definitions
  type GleipSettings = {
    telemetryEnabled: boolean;
    userId?: string;
    runtimeInfo?: Record<string, string>;
  };
  
  type UpdateInfo = {
    currentVersion: string;
    latestVersion: string;
    releaseNotes: string;
    downloadUrl: string;
    isUpdateNeeded: boolean;
  };

  // State
  let error: string | null = null;
  let showCopiedNotification = false;
  let settings: GleipSettings = { 
    telemetryEnabled: true,
  };
  let isUpdatingSettings = false;
  
  // Update state
  let isCheckingForUpdates = false;
  let updateInfo: UpdateInfo | null = null;
  let updateStatus = "";
  let appVersion = "Unknown";

  // Functions
  async function loadSettings() {
    try {
      settings = await GetSettings();
    } catch (err) {
      if (err instanceof Error) {
        error = err.message;
      } else {
        error = String(err);
      }
    }
  }

  async function loadAppVersion() {
    try {
      const versionInfo = await GetAppVersion();
      appVersion = versionInfo.version || "Unknown";
    } catch (err) {
      console.error("Failed to load app version:", err);
      appVersion = "Unknown";
    }
  }

  async function updateSettings(newSettings: Partial<GleipSettings>) {
    try {
      isUpdatingSettings = true;
      error = null;
      const updatedSettings = { ...settings, ...newSettings };
      await UpdateSettings(updatedSettings);
      settings = updatedSettings;
    } catch (err) {
      if (err instanceof Error) {
        error = err.message;
      } else {
        error = String(err);
      }
    } finally {
      isUpdatingSettings = false;
    }
  }

  // Callback functions for FirefoxManager
  function handleFirefoxError(errorMessage: string) {
    error = errorMessage;
  }

  // Copy proxy address to clipboard
  function copyProxyAddressToClipboard() {
    const address = "http://127.0.0.1:9090";
    navigator.clipboard.writeText(address)
      .then(() => {
        showCopiedNotification = true;
        setTimeout(() => {
          showCopiedNotification = false;
        }, 3000);
      })
      .catch(err => {
        console.error("Failed to copy address: ", err);
      });
  }

  // Update functions
  async function checkForUpdates() {
    try {
      isCheckingForUpdates = true;
      error = null;
      
      // We need to directly call the Go functions
      // Since ManualCheckForUpdates isn't automatically bound
      // @ts-ignore - This will be available at runtime when Wails builds the app
      updateInfo = await window.go.backend.App.ManualCheckForUpdates();
      
      if (updateInfo) {
        if (!updateInfo.isUpdateNeeded) {
          // Show "up-to-date" notification
          updateStatus = "You are already on the latest version!";
          setTimeout(() => {
            updateStatus = "";
          }, 3000);
        }
      }
    } catch (err) {
      if (err instanceof Error) {
        error = err.message;
      } else {
        error = String(err);
      }
    } finally {
      isCheckingForUpdates = false;
    }
  }

  // Initialize
  onMount(async () => {
    await loadSettings();
    await loadAppVersion();
  });
</script>

<style>
  /* We can remove the gradient hover styles since they're now in the Button component */
</style>

<div class="flex flex-col h-full">
  <div class="h-12 border-b border-gray-700/50 bg-gray-800/30 px-4 grid items-center">
    <h2 class="text-lg font-medium text-gray-100">Settings</h2>
  </div>
  
  <div class="flex-1 overflow-auto p-4">
    <div class="max-w-4xl mx-auto">
      {#if error}
        <div class="mb-3 p-2 bg-red-900/20 border border-red-800 rounded text-red-300 text-sm">
          {error}
        </div>
      {/if}
      
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
        <!-- Firefox Browser -->
        <FirefoxManager 
          onError={handleFirefoxError}
        />
        
        <!-- Chromium Browser (Greyed Out) -->
        <div class="bg-gray-800/30 rounded-lg p-3 border border-gray-700/30 relative overflow-hidden opacity-70">
          <div class="absolute top-0 right-0 bg-[var(--color-midnight-accent)]/20 px-2 py-0.5 text-xs font-medium text-[var(--color-midnight-accent)] rounded-bl">
            Coming Soon
          </div>
          
          <div class="flex justify-between items-center mb-1">
            <h3 class="text-base font-semibold text-gray-50">Chromium Browser</h3>
            <p class="text-gray-600 text-xs">
              Status: <span class="text-gray-500">Not available</span>
            </p>
          </div>
          
          <p class="text-gray-500 text-sm mb-4">
            Support for Chromium-based browsers coming soon.
          </p>
          
          <!-- Main Action Button -->
          <button 
            disabled
            class="w-full px-3 py-2 rounded-lg text-sm font-medium bg-gray-700 text-gray-500 cursor-not-allowed"
          >
            Download Chromium
          </button>
          
          <!-- No uninstall button shown for Chromium since it's not available -->
        </div>
      </div>
      

      
      <!-- Manual Configuration -->
      <div class="bg-gray-800/50 rounded-lg p-3 border border-gray-700/50">
        <h3 class="text-base font-semibold text-gray-50 mb-1">Manual Browser Configuration</h3>
        <p class="text-gray-50 text-sm mb-2">
          For using your own browser with Gleip proxy:
        </p>
        
        <ol class="list-decimal list-inside space-y-1 text-gray-100 text-sm">
          <li class="text-gray-50">Set browser proxy to <span class="text-gray-50 font-mono">127.0.0.1:9090</span></li>
          <li class="text-gray-50 relative">
            Visit <button 
              on:click={copyProxyAddressToClipboard} 
              class="text-[var(--color-midnight-accent)] hover:text-opacity-80 focus:outline-none focus:ring-1 focus:ring-[var(--color-midnight-accent)] rounded px-1"
            >
              127.0.0.1:9090
            </button> to download the CA certificate
            {#if showCopiedNotification}
              <span class="absolute inset-0 flex items-center ml-11 mt-6">
                <span class="bg-green-900/80 text-green-200 text-xs py-0.5 px-1.5 rounded-md shadow-md">
                  Copied!
                </span>
              </span>
            {/if}
          </li>
          <li class="text-gray-50">Install the certificate as instructed</li>
          <li class="text-gray-50">Start browsing to capture traffic in History tab</li>
        </ol>
      </div>

      <!-- Application Settings -->
      <div class="bg-gray-800/50 rounded-lg p-3 border border-gray-700/50 mt-4">
        <h3 class="text-base font-semibold text-gray-50 mb-3">Application Settings</h3>
        
        <div class="space-y-4">
          <!-- Telemetry Switch -->
          <div class="flex items-center justify-between">
            <div>
              <h4 class="text-sm font-medium text-gray-50">Anonymous Usage Data</h4>
              <p class="text-xs text-gray-50 mt-0.5">
                Help improve Gleip by sending anonymous usage data.
              </p>
            </div>
            <label class="relative inline-flex items-center cursor-pointer">
              <input 
                type="checkbox" 
                class="sr-only peer" 
                checked={settings.telemetryEnabled}
                on:change={(e) => updateSettings({ telemetryEnabled: e.currentTarget.checked })}
                disabled={isUpdatingSettings}
              >
              <div class="w-11 h-6 bg-gray-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-[var(--color-midnight-accent)]"></div>
            </label>
          </div>
          
          <!-- Update Check Button -->
          <div class="flex items-center justify-between pt-2 border-t border-gray-700/50">
            <div>
              <h4 class="text-sm font-medium text-gray-50">Check for Updates</h4>
              <p class="text-xs text-gray-50 mt-0.5">
                Current version: {appVersion}
              </p>
              {#if updateStatus}
                <p class="text-xs text-green-400 mt-1">{updateStatus}</p>
              {/if}
            </div>
            <Button 
              on:click={checkForUpdates}
              disabled={isCheckingForUpdates}
              loading={isCheckingForUpdates}
              variant="secondary"
              size="sm"
            >
              {isCheckingForUpdates ? 'Checking...' : 'Check Now'}
            </Button>
          </div>
        </div>
      </div>
    </div>
  </div>
</div> 