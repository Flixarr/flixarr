import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				// Svelte 5 runes everywhere (project rule): legacy syntax fails the build.
				runes: ({ filename }) => (filename.split(/[/\\]/).includes('node_modules') ? undefined : true)
			},
			// Single-page app served by the Go binary: every non-file path gets 200.html.
			adapter: adapter({ pages: 'build', assets: 'build', fallback: '200.html', precompress: false })
		})
	],
	server: {
		// During development, the Go server runs on :7575 and handles /api.
		proxy: { '/api': 'http://localhost:7575' }
	}
});
