declare module 'highlight.js/lib/languages/javascript' {
  import { LanguageFn } from 'highlight.js';
  const javascript: LanguageFn;
  export default javascript;
}

// Fix for the highlightElement error
declare module 'highlight.js/lib/core' {
  export * from 'highlight.js';
  const hljs: import('highlight.js').HLJSApi;
  export default hljs;
}

// Fix for the main highlight.js module
declare module 'highlight.js' {
  export interface HLJSApi {
    highlightElement(element: HTMLElement): void;
    registerLanguage(name: string, language: LanguageFn): void;
  }
  
  export type LanguageFn = (hljs?: any) => any;
  
  const hljs: HLJSApi;
  export default hljs;
} 