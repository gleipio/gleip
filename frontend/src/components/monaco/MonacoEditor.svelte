<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import { monaco, initializeMonaco, isMonacoReady } from './monaco-setup';
  
  // Props
  export let value: string = '';
  export let language: string = 'http';
  export let readOnly: boolean = false;
  export let automaticLayout: boolean = true;
  export let fontSize: number = 12;
  
  // Internal state
  let container: HTMLElement;
  let editor: monaco.editor.IStandaloneCodeEditor | null = null;
  let isUpdating = false;
  const dispatch = createEventDispatcher();
  
  // Function to manually trigger editor layout
  export function layout() {
    if (editor) {
      editor.layout();
    }
  }
  
  // Function to directly set editor value (bypassing reactivity)
  export function setValue(newValue: string) {
    if (editor) {
      try {
        editor.setValue(newValue);
      } catch (error) {
        console.error(`MONACO DIRECT setValue: Error:`, error);
      }
    }
  }
  
  // Function to get the Monaco editor instance
  export function getEditor() {
    return editor;
  }
  
  // Debug: Log when value prop changes
  $: console.log(`📝 MONACO VALUE PROP CHANGED: length ${value.length}, preview:`, value.substring(0, 50));
  
  // Create editor on mount with performance optimizations
  onMount(async () => {
    if (!container) return;
    
    try {
      // Ensure monaco is initialized
      initializeMonaco();
      
      // Wait for Monaco to be ready
      let attempts = 0;
      const maxAttempts = 20; // 1 second max wait
      while (!isMonacoReady() && attempts < maxAttempts) {
        await new Promise(resolve => setTimeout(resolve, 50));
        attempts++;
      }
      
      if (!isMonacoReady()) {
        throw new Error('Monaco Editor failed to initialize within timeout');
      }
      
      // Create editor with bare minimum settings
      editor = monaco.editor.create(container, {
      value,
      language,
      theme: 'gleipDark', // Use our custom theme with variable highlighting
      automaticLayout: false,
      readOnly,
      minimap: { enabled: false },
      // Remove contextmenu option due to type issues
      quickSuggestions: false,
      parameterHints: { enabled: false },
      suggestOnTriggerCharacters: false,
      snippetSuggestions: 'none',
      wordBasedSuggestions: 'off' as any,
      lineNumbers: readOnly ? 'off' : 'on',
      scrollBeyondLastLine: false,
      renderLineHighlight: 'none',
      hideCursorInOverviewRuler: true,
      overviewRulerBorder: false,
      overviewRulerLanes: 0,
      folding: false,
      glyphMargin: false,
      wordWrap: 'on',
      fontSize,
      stickyScroll: { enabled: false },
      scrollbar: {
        // These settings let the parent scroll when editor scrollbar reaches the end
        alwaysConsumeMouseWheel: false,
        handleMouseWheel: true,
      }
    });
    
    // Force model language when it's HTTP
    if ((language === 'http' || language === 'httpWithVars') && editor.getModel()) {
      monaco.editor.setModelLanguage(editor.getModel()!, language);
    }
    
    // Add change handler
    editor.onDidChangeModelContent(() => {
      if (editor) {
        dispatch('change', { value: editor.getValue() });
      }
    });
    
    // Dispatch mount event with the editor instance
    dispatch('mount', { editor });
    
    // Apply layout immediately to ensure proper initial sizing
    setTimeout(() => layout(), 10);
    
    // Add resize observer for more responsive resizing
    if (automaticLayout && typeof ResizeObserver !== 'undefined') {
      const resizeObserver = new ResizeObserver(() => {
        if (editor && container.offsetWidth > 0 && container.offsetHeight > 0) {
          editor.layout();
        }
      });
      
      resizeObserver.observe(container);
      
      // Store observer for cleanup
      (container as any)._resizeObserver = resizeObserver;
    } 
    // Fallback to interval-based layout updates for older browsers
    else if (automaticLayout) {
      const layoutInterval = setInterval(() => {
        if (editor && container.offsetWidth > 0 && container.offsetHeight > 0) {
          editor.layout();
        }
      }, 200); // Check more frequently for better responsiveness
      
      // Store interval for cleanup
      (container as any)._layoutInterval = layoutInterval;
    }
    } catch (error) {
      console.error('Failed to initialize Monaco Editor:', error);
      // Fallback: show error message in container
      if (container) {
        container.innerHTML = `<div style="padding: 10px; color: #ff6b6b; font-family: monospace;">
          Monaco Editor failed to load. Error: ${error instanceof Error ? error.message : 'Unknown error'}
        </div>`;
      }
    }
  });
  
  // Clean up on destroy
  onDestroy(() => {
    // Clean up resize observer if it exists
    if (container && (container as any)._resizeObserver) {
      (container as any)._resizeObserver.disconnect();
    }
    
    // Clean up layout interval if it exists
    if (container && (container as any)._layoutInterval) {
      clearInterval((container as any)._layoutInterval);
    }
    
    // Dispose editor and model
    if (editor) {
      const model = editor.getModel();
      editor.dispose();
      if (model) model.dispose();
      editor = null;
    }
  });
  
  // Update the editor when value changes with proper model management
  $: if (editor && editor.getValue() !== value && !isUpdating) {
    console.log(`🔄 MONACO REACTIVE triggered: editor exists: ${!!editor}, current value length: ${editor.getValue().length}, new value length: ${value.length}, isUpdating: ${isUpdating}`);
    updateEditorValue(value);
  }
  
  async function updateEditorValue(newValue: string) {
    if (!editor || isUpdating) {
      console.log(`❌ MONACO updateEditorValue early return: editor exists: ${!!editor}, isUpdating: ${isUpdating}`);
      return;
    }
    
    console.log(`🔄 MONACO updateEditorValue: Setting new value, length: ${newValue.length}, preview:`, newValue.substring(0, 50));
    
    try {
      isUpdating = true;
      
      // Get current model
      const currentModel = editor.getModel();
      
      // If we have a value and it's different, update it
      if (newValue && editor.getValue() !== newValue) {
        // Use simple setValue - executeEdits was failing silently
        console.log(`🔄 MONACO using setValue to update content`);
        editor.setValue(newValue);
        console.log(`✅ MONACO content updated, new editor value length: ${editor.getValue().length}`);
      } else if (!newValue) {
        // Clear the editor if no value
        console.log(`🔄 MONACO clearing editor (no value)`);
        editor.setValue('');
      } else {
        console.log(`❓ MONACO no update needed, values are same`);
      }
      
      // Force layout after content change
      setTimeout(() => {
        if (editor) {
          editor.layout();
        }
      }, 10);
      
    } catch (error) {
      console.error('Error updating Monaco editor value:', error);
      // Fallback to simple setValue
      try {
        editor.setValue(newValue || '');
      } catch (fallbackError) {
        console.error('Fallback setValue also failed:', fallbackError);
      }
    } finally {
      isUpdating = false;
    }
  }
  
  // Update language when it changes
  $: if (editor && editor.getModel() && !isUpdating) {
    updateEditorLanguage(language);
  }
  
  function updateEditorLanguage(newLanguage: string) {
    if (!editor || isUpdating) return;
    
    try {
      const model = editor.getModel();
      if (model && model.getLanguageId() !== newLanguage) {
        monaco.editor.setModelLanguage(model, newLanguage);
      }
    } catch (error) {
      console.error('Error updating Monaco editor language:', error);
    }
  }

  // Update read-only state when it changes
  $: if (editor) {
    editor.updateOptions({ readOnly });
  }
</script>

<div bind:this={container} class="monaco-editor-container h-full w-full"></div>

<style>
  .monaco-editor-container {
    min-height: 6rem;
  }
  
  /* Styling for variables (keywords) */
  :global(.monaco-editor .mtk25, .monaco-editor .mtk24) {
    color: var(--color-midnight-accent) !important;
    font-weight: bold !important;
    background-color: rgba(74, 171, 198, 0.3) !important;
    border-radius: 3px !important;
    padding: 0 2px !important;
  }

  :global(.monaco-editor .mtk11) {
    color: var(--color-warning) !important;
    font-weight: bold !important;
    background-color: rgba(245, 158, 11, 0.3) !important;
    border-radius: 3px !important;
    padding: 0 2px !important;
  }
</style> 