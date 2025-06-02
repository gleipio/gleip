import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import sveltePreprocess from 'svelte-preprocess'

// https://vitejs.dev/config/
export default defineConfig(({ command, mode }) => {
  const isProduction = mode === 'production';
  console.log(`Running in ${mode} mode - compilerOptions.dev=${!isProduction}`);
  
  return {
    plugins: [
      svelte({
        compilerOptions: {
          dev: !isProduction,
        },
        preprocess: sveltePreprocess({
          typescript: true
        })
      })
    ],
    build: {
      // When running 'wails dev', don't minify to make debugging easier
      minify: isProduction ? 'esbuild' : false,
      sourcemap: !isProduction
    },
    optimizeDeps: {
      // Monaco is now handled specially in our setup
      include: []
    }
  };
}); 