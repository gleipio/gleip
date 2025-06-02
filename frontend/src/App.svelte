<script lang="ts">
  import { onMount } from 'svelte';
  import { WindowGetSize, EventsOn } from '../wailsjs/runtime/runtime';
  
  import ReqHistory from './features/history/ReqHistory.svelte';
  import * as GleipFlowModule from './features/gleipflows/GleipFlow.svelte';
  import Intercept from './features/intercept/Intercept.svelte';
  import Settings from './features/settings/Settings.svelte';
  import Import from './features/import/Import.svelte';
  import { theme, initTheme } from './shared/utils/theme';
  import UpdateModal from './components/UpdateModal.svelte';
  import { initializeInterceptStore, interceptEnabled, updateInterceptState } from './features/intercept/store/interceptStore';
  import { SetInterceptEnabled, NewProject, SaveProject, SaveProjectAs, LoadProject } from '../wailsjs/go/backend/App';
  
  // Import Lucide icons
  import { History, FunnelX, Settings as SettingsIcon, FileSearch, Database, Menu, X } from 'lucide-svelte';
  import { GiLinkedRings } from 'svelte-icons/gi';
  
  // Navigation items
  const navItems = [
    { path: '/', icon: History, label: 'History', component: ReqHistory },
    { path: '/intercept', icon: FunnelX, label: 'Intercept', component: Intercept },
    { path: '/gleipflows', icon: GiLinkedRings, label: 'GleipFlows', component: GleipFlowModule.default },
    { path: '/import', icon: Database, label: 'Import', component: Import },
    // { path: '/findings', icon: FileSearch, label: 'Findings', component: undefined },
    // TODO: Add interceptor back in
    { path: '/settings', icon: SettingsIcon, label: 'Settings', component: Settings }
  ];
  
  // State
  let windowSize = { w: 1000, h: 600 };
  let currentPath = '/';
  let viewKey = Date.now(); // Initialize with a unique value
  let hamburgerMenuOpen = false;
  
  // Handle navigation
  function navigateTo(path: string) {
    currentPath = path;
    // Optionally, change the viewKey on navigation as well if each navigation should be a full reload
    // viewKey = Date.now(); 
  }
  
  // Get current component - ensure it's always a valid component type or default
  let currentComponent: typeof ReqHistory | typeof GleipFlowModule.default | typeof Settings | typeof Intercept | typeof Import | undefined =
    navItems.find(item => item.path === currentPath)?.component || ReqHistory;

  // Function to refresh the current view by changing the key
  function forceReloadCurrentView() {
    viewKey = Date.now(); // Update the key to force re-render
    console.log('View key updated, forcing reload:', viewKey);
  }
  
  // Handle intercept toggle
  async function handleInterceptToggle(event: Event) {
    const checked = (event.target as HTMLInputElement).checked;
    try {
      await SetInterceptEnabled(checked);
      updateInterceptState(checked);
    } catch (error) {
      console.error('Failed to toggle interception:', error);
    }
  }
  
  // Handle switch container click
  function handleSwitchClick(event: Event) {
    // If intercept is currently enabled, prevent navigation when clicking to turn it off
    if ($interceptEnabled) {
      event.stopPropagation();
    }
  }
  
  // Handle hamburger menu toggle
  function toggleHamburgerMenu() {
    hamburgerMenuOpen = !hamburgerMenuOpen;
  }
  
  // Close hamburger menu when clicking outside
  function closeHamburgerMenu() {
    hamburgerMenuOpen = false;
  }
  
  // Handle file operations
  async function handleNewProject() {
    try {
      await NewProject();
      console.log('New project created successfully');
      closeHamburgerMenu();
    } catch (error) {
      console.error('Failed to create new project:', error);
    }
  }
  
  async function handleSaveProject() {
    try {
      await SaveProject();
      console.log('Project saved successfully');
      closeHamburgerMenu();
    } catch (error) {
      console.error('Failed to save project:', error);
    }
  }
  
  async function handleSaveProjectAs() {
    try {
      await SaveProjectAs();
      console.log('Project saved as successfully');
      closeHamburgerMenu();
    } catch (error) {
      console.error('Failed to save project as:', error);
    }
  }
  
  async function handleLoadProject() {
    try {
      await LoadProject();
      console.log('Project loaded successfully');
      closeHamburgerMenu();
    } catch (error) {
      console.error('Failed to load project:', error);
    }
  }
  
  // Handle keyboard shortcuts
  function handleKeydown(event: KeyboardEvent) {
    if (event.ctrlKey || event.metaKey) {
      switch (event.key) {
        case 'n':
          event.preventDefault();
          handleNewProject();
          break;
        case 's':
          if (event.shiftKey) {
            event.preventDefault();
            handleSaveProjectAs();
          } else {
            event.preventDefault();
            handleSaveProject();
          }
          break;
        case 'o':
          event.preventDefault();
          handleLoadProject();
          break;
      }
    }
  }
  
  onMount(() => {
    // Initialize theme variables
    initTheme();
    
    // Initialize intercept store early
    initializeInterceptStore();
    
    // Initialize with an async IIFE
    (async () => {
      // Initial size
      windowSize = await WindowGetSize();
    })();
    
    // Listen for window resize events
    window.addEventListener('resize', async () => {
      windowSize = await WindowGetSize();
    });
    
    // Listen for keyboard shortcuts
    window.addEventListener('keydown', handleKeydown);
    
    // Listen for navigation events from components
    const cleanupNavigate = EventsOn('navigate', (path: string) => {
      navigateTo(path);
    });

    // Listen for project loaded/created events from Go backend
    const cleanupProjectLoaded = EventsOn('project:loaded', (projectID: string) => {
      console.log(`Project loaded: ${projectID}, forcing view reload.`);
      forceReloadCurrentView();
    });

    const cleanupProjectCreated = EventsOn('project:created', (projectID: string) => {
      console.log(`Project created: ${projectID}, forcing view reload.`);
      forceReloadCurrentView();
    });
    
    return () => {
      cleanupNavigate();
      cleanupProjectLoaded();
      cleanupProjectCreated();
      window.removeEventListener('keydown', handleKeydown);
    };
  });

  // Reactive statement to update component when path changes
  $: {
    if (typeof currentPath === 'string') { // Ensure currentPath is initialized
        const newComponentCandidate = navItems.find(item => item.path === currentPath)?.component;
        currentComponent = newComponentCandidate || ReqHistory; // Default to ReqHistory
        // When the component type itself changes due to navigation, we also want to ensure a fresh instance.
        // If the component type is the same as before, forceReloadCurrentView via project events will handle it.
        // If the component type *changes*, Svelte usually creates a new one, but using a key is more explicit.
        viewKey = Date.now(); 
    }
  }
</script>

<div class="flex flex-col bg-midnight text-gray-100 overflow-hidden"
     style="width: {windowSize.w}px; height: {windowSize.h}px">
  
  <!-- Global backdrop for hamburger menu -->
  {#if hamburgerMenuOpen}
    <div 
      class="fixed inset-0 z-50 cursor-default" 
      on:click|stopPropagation={closeHamburgerMenu}
      on:keydown={(e) => (e.key === 'Escape' || e.key === 'Enter' || e.key === ' ') && closeHamburgerMenu()}
      role="button"
      tabindex="-1"
      aria-label="Close menu"
    ></div>
  {/if}
  
  <div class="flex flex-1 overflow-hidden">
    <!-- Sidebar -->
    <nav class="w-40 bg-midnight border-r border-midnight-darker flex-none flex flex-col">
      <!-- Header -->
      <div class="h-14 bg-midnight-light/70 backdrop-blur-xl border-b border-midnight-darker flex items-center justify-center px-4 z-50 flex-none relative">
        <!-- Hamburger menu button -->
        <button 
          on:click|stopPropagation={toggleHamburgerMenu}
          class="absolute left-2 p-1.5 rounded hover:bg-midnight-darker transition-colors z-20"
          aria-label="Menu"
        >
          <Menu size={20} class="text-gray-100 hover:text-white" />
        </button>
        
        <!-- Hamburger dropdown menu -->
        {#if hamburgerMenuOpen}
          <!-- Menu content -->
          <div 
            class="absolute left-4 top-14 bg-midnight-light border border-midnight-darker rounded-lg shadow-lg hamburger-menu py-2 min-w-56 z-[60]"
            on:click|stopPropagation
            on:keydown|stopPropagation
            role="menu"
            tabindex="-1"
          >
            <button 
              on:click={handleNewProject}
              class="w-full px-4 py-2 text-left text-sm text-gray-100 hover:bg-midnight-darker hover:text-white transition-colors flex items-center justify-between"
            >
              <span>New Project</span>
              <span class="text-xs text-gray-500">Ctrl+N</span>
            </button>
            
            <button 
              on:click={handleLoadProject}
              class="w-full px-4 py-2 text-left text-sm text-gray-100 hover:bg-midnight-darker hover:text-white transition-colors flex items-center justify-between"
            >
              <span>Open Project</span>
              <span class="text-xs text-gray-500">Ctrl+O</span>
            </button>
            
            <!-- <div class="border-t border-midnight-darker my-1"></div> -->
            
            <button 
              on:click={handleSaveProject}
              class="w-full px-4 py-2 text-left text-sm text-gray-100 hover:bg-midnight-darker hover:text-white transition-colors flex items-center justify-between"
            >
              <span>Save Project</span>
              <span class="text-xs text-gray-500">Ctrl+S</span>
            </button>
            
            <button 
              on:click={handleSaveProjectAs}
              class="w-full px-4 py-2 text-left text-sm text-gray-100 hover:bg-midnight-darker hover:text-white transition-colors flex items-center justify-between"
            >
              <span>Save Project As...</span>
              <span class="text-xs text-gray-500">Ctrl+Shift+S</span>
            </button>
          </div>
        {/if}
        
        <div class="flex items-center pl-2">
          <div class="logo-container relative mr-1">
            <div class="w-7 h-7 flex items-center justify-center">
              <!-- Single color icon without effects -->
              <div class="w-6 h-6 relative z-10">
                <div class="icon-solid">
                  <GiLinkedRings />
                </div>
              </div>
            </div>
          </div>
          <h1 class="text-xl relative logo-text">Gleip</h1>
        </div>
      </div>

      <!-- Navigation menu -->
      <div class="pt-0 pb-3 space-y-0">
        {#each navItems as { path, icon: Icon, label }, index}
          <button 
            on:click={() => navigateTo(path)}
            class="nav-item {currentPath === path ? 'active' : ''} {index > 0 ? 'border-t' : ''}"
          >
            {#if currentPath === path}
              <div class="nav-active-bg"></div>
              <div class="nav-active-border"></div>
            {/if}
            <div class="nav-icon">
              {#if Icon === GiLinkedRings}
                <div class="gi-icon">
                  <svelte:component this={Icon} />
                </div>
              {:else}
                <svelte:component this={Icon} size={18} strokeWidth={currentPath === path ? 2.5 : 1.5} />
              {/if}
            </div>
            <div class="nav-content">
              <span class="nav-label">{label}</span>
              {#if path === '/intercept'}
                <div role="button" tabindex="0" on:click={handleSwitchClick} on:keydown={handleSwitchClick}>
                  <label class="mini-switch-container">
                    <input
                      type="checkbox"
                      class="sr-only"
                      bind:checked={$interceptEnabled}
                      on:change={handleInterceptToggle}
                    />
                    <div class="mini-switch-track" class:checked={$interceptEnabled}></div>
                  </label>
                </div>
              {/if}
            </div>
          </button>
        {/each}
      </div>
    </nav>

    <!-- Main content -->
    <main class="flex-1 overflow-hidden">
      {#key viewKey}
        {#if currentPath === '/'}
          <ReqHistory />
        {:else if currentPath === '/gleipflows'}
          <svelte:component this={GleipFlowModule.default} />
        {:else if currentPath === '/import'}
          <svelte:component this={Import} />
        {:else if currentPath === '/intercept'}
          <Intercept />
        {:else if currentPath === '/settings'}
          <Settings />
        {/if}
      {/key}
    </main>
  </div>
  
  <!-- Global update modal component -->
  <UpdateModal />
</div>

<style>
  /* Apply theme classes */
  .bg-midnight {
    background-color: var(--color-midnight);
  }
  
  .bg-midnight-light {
    background-color: var(--color-midnight-light);
  }
  
  /* Simplified Navigation styling */
  .nav-item {
    width: 100%;
    padding: 0.625rem 1rem;
    display: flex;
    align-items: center;
    gap: 0.625rem;
    transition: all 0.2s ease;
    position: relative;
    overflow: hidden;
    color: var(--color-nav-text);
    font-weight: 500;
  }
  
  .nav-item:hover {
    color: var(--color-nav-text-hover);
  }
  
  .nav-item.active {
    color: var(--color-nav-text-active);
  }
  
  .nav-item.border-t {
    border-top: 1px solid rgba(255, 255, 255, 0.07);
  }
  
  .nav-active-bg {
    position: absolute;
    inset: 0;
    z-index: -1;
    background: var(--gradient-nav-active-glow);
  }
  
  .nav-active-border {
    position: absolute;
    left: 0;
    top: 50%;
    transform: translateY(-50%);
    width: 0.25rem;
    height: 66.67%;
    background-color: var(--color-nav-border-active);
    border-radius: 0 0.25rem 0.25rem 0;
    z-index: 10;
  }
  
  .border-midnight-darker {
    border-color: var(--color-midnight-darker);
  }

  /* Hamburger menu styling */
  .hamburger-menu {
    box-shadow: 0 10px 25px rgba(0, 0, 0, 0.3);
  }
  
  /* Icon styling */
  .icon-solid :global(svg) {
    fill: var(--color-midnight-accent);
    width: 100%;
    height: 100%;
  }
  
  /* Simplified Icon styling */
  .nav-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    position: relative;
    z-index: 10;
    width: 18px;
    height: 18px;
  }
  
  /* Lucide icons (stroke-based) */
  .nav-icon > :global(svg) {
    width: 18px;
    height: 18px;
    stroke: currentColor;
    fill: none;
    transition: all 0.2s ease;
  }
  
  /* GiLinkedRings icon (fill-based) */
  .nav-icon .gi-icon :global(svg) {
    width: 18px;
    height: 18px;
    fill: currentColor;
    stroke: none;
    transition: all 0.2s ease;
  }
  
  .nav-content {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex: 1;
    min-width: 0;
  }
  
  .nav-label {
    font-weight: 500;
    position: relative;
    z-index: 10;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  /* Logo container */
  .logo-container {
    transform: translateY(-1px);
  }
  
  /* Logo text styling */
  .logo-text {
    font-family: 'Geist', sans-serif;
    font-weight: 500;
    color: white;
    display: flex;
    align-items: center;
    justify-content: center;
    height: 2.5rem;
  }
  
  @keyframes shimmer {
    0%, 100% { opacity: 0.05; color: var(--color-logo-shimmer-base); }
    15% { opacity: 0.05; color: var(--color-logo-shimmer-base); }
    20% { opacity: 0.1; color: var(--color-logo-shimmer-mid); }
    25% { opacity: 0.2; color: var(--color-logo-shimmer-high); }
    30% { opacity: 0.1; color: var(--color-logo-shimmer-mid); }
    35% { opacity: 0.05; color: var(--color-logo-shimmer-base); }
  }

  /* Mini switch styles */
  .mini-switch-container {
    position: relative;
    z-index: 20;
    cursor: pointer;
  }
  
  .mini-switch-track {
    width: 28px;
    height: 16px;
    background-color: #7f1d1d;
    border-radius: 8px;
    position: relative;
    transition: all 0.2s ease;
  }
  
  .mini-switch-track.checked {
    background-color: #16a34a;
  }
  
  .mini-switch-track::after {
    content: '';
    position: absolute;
    top: 1px;
    left: 1px;
    width: 12px;
    height: 14px;
    background-color: #d1d5db;
    border-radius: 10px;
    transition: all 0.2s ease;
  }
  
  .mini-switch-track.checked::after {
    transform: translateX(12px);
  }
</style>

<!-- SVG gradient definition -->
<svg width="0" height="0" class="hidden">
  <defs>
    <linearGradient id="logo-gradient" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="#f0f9ff" />
      <stop offset="50%" stop-color="{theme.colors.logoGradientMid}" />
      <stop offset="100%" stop-color="#62dafc" />
    </linearGradient>
  </defs>
</svg> 