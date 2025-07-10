<script lang="ts">
  import { onMount } from 'svelte';

  // Props
  export let message: string;
  export let type: 'success' | 'error' | 'info' | 'warning' = 'info';
  export let duration: number = 3000;
  export let onClose: () => void = () => {}; 

  // State
  let visible = true;

  // Define styling based on type
  const typeStyles = {
    success: {
      bg: 'bg-green-900/80',
      text: 'text-green-200',
    },
    error: {
      bg: 'bg-red-900/80',
      text: 'text-red-200',
    },
    warning: {
      bg: 'bg-yellow-800/80',
      text: 'text-yellow-200',
    },
    info: {
      bg: 'bg-blue-900/80',
      text: 'text-blue-200',
    },
  };

  // Auto-close after duration
  onMount(() => {
    if (duration > 0) {
      const timer = setTimeout(() => {
        visible = false;
        onClose();
      }, duration);
      
      return () => clearTimeout(timer);
    }
  });

  // Handle manual close
  function close() {
    visible = false;
    onClose();
  }
</script>

{#if visible}
  <div class="fixed bottom-4 right-4 z-50 {typeStyles[type].bg} {typeStyles[type].text} text-sm py-2 px-3 rounded-md shadow-md flex items-center">
    <span>{message}</span>
    <button 
      class="ml-3 opacity-70 hover:opacity-100" 
      on:click={close}
      aria-label="Close notification"
    >
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M18 6L6 18M6 6l12 12" />
      </svg>
    </button>
  </div>
{/if} 