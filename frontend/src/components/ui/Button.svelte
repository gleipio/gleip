<script lang="ts">
  /**
   * Customizable button component with gradient hover effect
   */
  
  // Props
  export let type: 'button' | 'submit' | 'reset' = 'button';
  export let variant: 'primary' | 'secondary' | 'dark' | 'link' = 'primary';
  export let size: 'xs' | 'sm' | 'md' | 'lg' = 'md';
  export let fullWidth = false;
  export let disabled = false;
  export let loading = false;
  export let withGradientHover = true;

  // Generate CSS classes based on variants
  $: variantClasses = {
    primary: 'bg-[var(--color-secondary-accent)] text-[var(--color-button-text)]',
    secondary: 'bg-[var(--color-midnight-accent)] text-[var(--color-midnight-darker)]',
    dark: 'bg-[var(--color-midnight-darker)] text-gray-100',
    link: 'bg-transparent text-[var(--color-midnight-accent)] hover:text-opacity-80'
  };

  // Generate CSS classes based on size
  $: sizeClasses = {
    xs: 'px-2 py-1 text-xs',
    sm: 'px-3 py-1 text-sm',
    md: 'px-4 py-2 text-sm',
    lg: 'px-5 py-2.5 text-base'
  };

  // Combine classes
  $: buttonClasses = `
    ${variantClasses[variant]} 
    ${sizeClasses[size]} 
    ${fullWidth ? 'w-full' : ''} 
    ${disabled || loading ? 'opacity-60 cursor-not-allowed' : ''} 
    ${withGradientHover && !disabled && !loading ? 'gradient-hover' : ''}
    rounded font-semibold transition-all focus:outline-none focus:ring-2 focus:ring-[var(--color-midnight-accent)] focus:ring-opacity-50
  `;
</script>

<style>
  /* Button hover gradient effects */
  .gradient-hover {
    position: relative;
    overflow: hidden;
    z-index: 1;
    transform: translateZ(0); /* Force hardware acceleration */
    backface-visibility: hidden; /* Prevent flickering */
  }

  .gradient-hover::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: var(--gradient-button-hover);
    opacity: 0;
    transition: opacity 0.3s ease;
    z-index: -1;
    pointer-events: none; /* Ensure hover doesn't affect text */
    will-change: opacity; /* Optimize for animation */
  }

  .gradient-hover:hover::before {
    opacity: 1;
  }
  
  /* Stabilize button content during hover */
  .gradient-hover > * {
    position: relative;
    z-index: 2;
  }
</style>

<button 
  type={type} 
  class={buttonClasses} 
  disabled={disabled || loading}
  on:click
  on:mouseover
  on:mouseenter
  on:mouseleave
  on:focus
  on:blur
  {...$$restProps}
>
  {#if loading}
    <span class="inline-block mr-2">
      <svg class="animate-spin h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
      </svg>
    </span>
  {/if}
  <slot></slot>
</button> 