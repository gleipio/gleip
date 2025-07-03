/**
 * Lightweight Monaco Editor setup
 */

import * as monaco from 'monaco-editor';

// Prevent Monaco from making any network requests
let initialized = false;

// Initialize Monaco Editor environment
export function initializeMonaco() {
  // Only initialize once
  if (initialized) return;
  
  if (typeof window !== 'undefined') {
    try {
      // Mark as initialized early to prevent multiple attempts
      initialized = true;
      // Create a "dummy" worker that works in main thread
      const blob = new Blob(['self.onmessage=function(){}'], { type: 'application/javascript' });
      const blobUrl = URL.createObjectURL(blob);
      
      // Use the same lightweight worker for all languages
      window.MonacoEnvironment = {
        getWorker: function() {
          return new Worker(blobUrl);
        }
      };
      
      // Disable all advanced features for better performance
      monaco.languages.typescript.javascriptDefaults.setCompilerOptions({
        target: monaco.languages.typescript.ScriptTarget.ES2015,
        allowNonTsExtensions: true,
        moduleResolution: monaco.languages.typescript.ModuleResolutionKind.NodeJs,
        module: monaco.languages.typescript.ModuleKind.CommonJS,
        noEmit: true,
        typeRoots: ["node_modules/@types"]
      });
      
      // Register HTTP language
      monaco.languages.register({ id: 'http' });
      
      // Define HTTP tokens with minimal rules
      monaco.languages.setMonarchTokensProvider('http', {
        tokenizer: {
          root: [
            // Method line (simplified)
            [/(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)/, 'keyword'],
            [/HTTP\/[0-9.]+/, 'comment'],
            
            // Headers (simplified)
            [/^([^:]+)(:)/, ['type', 'delimiter']],
            
            // Body (simplified)
            [/"[^"]*"/, 'string'],
            [/\b\d+\b/, 'number'],
            [/[{}[\],:]/, 'delimiter'],
          ]
        }
      });
      
      // Register HTTP with Variables language
      monaco.languages.register({ id: 'httpWithVars' });
      
      // Define HTTP with Variables tokens
      monaco.languages.setMonarchTokensProvider('httpWithVars', {
        tokenizer: {
          root: [
            // Variables
            [/\{\{([^}]*)\}\}/, 'variable'],
            
            // Method line (simplified)
            [/(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)/, 'keyword'],
            [/HTTP\/[0-9.]+/, 'comment'],
            
            // Headers (simplified)
            [/^([^:]+)(:)/, ['type', 'delimiter']],
            
            // Body (simplified)
            [/"/, 'string', '@string'],
            [/\b\d+\b/, 'number'],
            [/[{}[\],:]/, 'delimiter'],
          ],
          
          string: [
            [/\{\{([^}]*)\}\}/, 'variable'],
            [/[^"{{]+/, 'string'],
            [/"/, 'string', '@pop'],
          ]
        }
      });

      // Register HTTP with Variables and Fuzz language
      monaco.languages.register({ id: 'httpWithVarsAndFuzz' });

      // Define HTTP with Variables tokens
      monaco.languages.setMonarchTokensProvider('httpWithVarsAndFuzz', {
        tokenizer: {
          root: [
            // Fuzz
            [/\{\{(fuzz)\}\}/, 'fuzz'],
            // Variables
            [/\{\{([^}]*)\}\}/, 'variable'],
            // Method line (simplified)
            [/(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)/, 'keyword'],
            [/HTTP\/[0-9.]+/, 'comment'],
            
            // Headers (simplified)
            [/^([^:]+)(:)/, ['type', 'delimiter']],
            
            // Body (simplified)
            [/"/, 'string', '@string'],
            [/\b\d+\b/, 'number'],
            [/[{}[\],:]/, 'delimiter'],
          ],
          
          string: [
            [/\{\{(fuzz)\}\}/, 'fuzz'],
            [/\{\{([^}]*)\}\}/, 'variable'],
            [/[^"{{]+/, 'string'],
            [/"/, 'string', '@pop'],
          ]
        }
      });
      
      // Define custom theme with variable highlighting
      monaco.editor.defineTheme('gleipDark', {
        base: 'vs-dark',
        inherit: true,
        rules: [
          { token: 'variable', foreground: 'FFFFFF', fontStyle: 'bold' },
          { token: 'fuzz', foreground: 'FF9D00', fontStyle: 'bold' }
        ],
        colors: {}
      });
      
      // Set as default theme
      monaco.editor.setTheme('gleipDark');
    } catch (err) {
      console.error('Monaco setup error:', err);
      // Reset initialized flag so it can be retried
      initialized = false;
      throw err;
    }
  } else {
    // Not in browser environment
    initialized = true;
  }
}

// Function to check if Monaco is ready to use
export function isMonacoReady(): boolean {
  return initialized && typeof window !== 'undefined' && !!window.MonacoEnvironment;
}

// Export the monaco instance
export { monaco }; 