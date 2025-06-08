<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  
  export let position: 'left' | 'right' = 'right';
  export let stepPosition: number;
  export let stepType: 'request' | 'chef' | 'script' = 'request';
  
  const dispatch = createEventDispatcher();
  
  function handleClick() {
    dispatch('addStep', { position: stepPosition, stepType });
  }
</script>

<div class="absolute top-[3rem] {position === 'left' ? '-left-[1.5rem]' : '-right-[1.5rem]'} transform -translate-y-1/2 z-10">
  <button
    class="w-9 h-9 bg-[var(--color-table-row-hover)] hover:bg-[var(--color-table-row-hover)]/80 text-[var(--color-button-text)] rounded-full flex items-center justify-center transition-all duration-200 shadow-lg hover:shadow-xl border border-gray-600/50 hover:border-gray-500 opacity-60 hover:opacity-100"
    on:click={handleClick}
    title="Add {stepType} step here"
  >
    <span class="text-4xl text-gray-300 pb-1 pl-[0.1rem] font-light">+</span>
  </button>
</div> 