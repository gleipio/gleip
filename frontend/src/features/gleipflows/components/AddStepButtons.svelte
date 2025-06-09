<script lang="ts">
  import { createEventDispatcher } from "svelte";

  export let position: "left" | "right" = "right";
  export let stepPosition: number;

  const dispatch = createEventDispatcher();

  function handleClick(stepType: "request" | "chef" | "script") {
    dispatch("addStep", { position: stepPosition, stepType });
  }

  function handlePaste() {
    dispatch("paste", { position: stepPosition });
  }
  
</script>

<div
  class="absolute top-[10rem] {position === 'left'
    ? '-left-[1.5rem]'
    : '-right-[1.5rem]'} transform -translate-y-1/2 z-10"
>
  <button
    class="my-2 w-9 h-9 bg-[var(--color-table-row-hover)] hover:bg-[var(--color-table-row-hover)]/80 text-[var(--color-button-text)] rounded-full flex items-center justify-center transition-all duration-200 shadow-lg hover:shadow-xl border border-gray-600/50 hover:border-gray-500 opacity-60 hover:opacity-100"
    on:click={() => handleClick("request")}
    title="Add request step here"
  >
    <span class="text-xl text-blue-500 pb-1 pl-[0.1rem] font-light">+R</span>
  </button>
  <button
    class="my-2 w-9 h-9 bg-[var(--color-table-row-hover)] hover:bg-[var(--color-table-row-hover)]/80 text-[var(--color-button-text)] rounded-full flex items-center justify-center transition-all duration-200 shadow-lg hover:shadow-xl border border-gray-600/50 hover:border-gray-500 opacity-60 hover:opacity-100"
    on:click={() => handleClick("chef")}
    title="Add chef step here"
  >
    <span class="text-xl text-purple-500 pb-1 pl-[0.1rem] font-light">+C</span>
  </button>

  <button
    class="my-2 w-9 h-9 bg-[var(--color-table-row-hover)] hover:bg-[var(--color-table-row-hover)]/80 text-[var(--color-button-text)] rounded-full flex items-center justify-center transition-all duration-200 shadow-lg hover:shadow-xl border border-gray-600/50 hover:border-gray-500 opacity-60 hover:opacity-100"
    on:click={handlePaste}
    title="Paste request here"
  >
    <span class="text-4xl text-blue-500 pb-1 pl-[0.1rem] font-light"
      >&#x2398;</span
    >
  </button>
</div>
