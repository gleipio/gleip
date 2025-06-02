// Initialize Monaco Editor first, before other imports
import { initializeMonaco } from './components/monaco/monaco-setup';
initializeMonaco();

// Then load regular imports
import './app.css';
import App from './App.svelte';

// Set dark mode as default
document.documentElement.classList.add('dark');

const app = new App({
  target: document.getElementById('app')!,
});

export default app; 