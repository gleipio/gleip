<script lang="ts">
  // Props
  export let x: number;
  export let y: number;
  export let onClose: () => void;
  export let items: { label: string; onClick: () => void }[] = [];

  // Click outside to close
  function handleClickOutside() {
    onClose();
  }

  // Handle keyboard events
  function handleKeyDown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      onClose();
    }
  }
</script>

<svelte:window on:keydown={handleKeyDown} />

<div 
  class="fixed z-50 bg-[var(--color-midnight-light)] rounded shadow-lg shadow-black/30 border border-[var(--color-midnight-darker)] w-48 py-1 text-sm text-gray-100"
  style="top: {y}px; left: {x}px;"
  role="menu"
  tabindex="-1"
>
  {#each items as item}
    <button 
      class="w-full text-left px-4 py-2 hover:bg-[var(--color-midnight-accent)]/20 flex items-center gap-2 transition-colors duration-150"
      on:click={() => {
        item.onClick();
        onClose();
      }}
      role="menuitem"
    >
      <span>{item.label}</span>
    </button>
  {/each}
</div>

<!-- Click outside to close context menu -->
<div 
  class="fixed inset-0 z-40" 
  on:click={handleClickOutside}
  on:contextmenu|preventDefault={handleClickOutside}
  role="presentation"
></div> 