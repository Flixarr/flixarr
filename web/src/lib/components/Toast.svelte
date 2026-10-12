<script lang="ts">
	import Icon from './Icon.svelte';
	import { toasts, dismiss } from '#lib/toast.svelte.ts';
</script>

<div class="pointer-events-none fixed inset-x-0 bottom-24 z-[60] flex flex-col items-center gap-2 px-4 tablet:bottom-8" role="status" aria-live="polite">
	{#each toasts as t (t.id)}
		<button class="toast-in pointer-events-auto flex items-center gap-2 rounded-xl border border-gray-700/60 bg-gray-900 px-4 py-3 text-left text-sm shadow-2xl transition-opacity duration-300 {t.leaving ? 'opacity-0' : 'opacity-100'}"
			onclick={() => dismiss(t.id)} aria-label="{t.text} (dismiss)">
			<span class={t.tone === 'green' ? 'text-green-400' : 'text-primary-light'}><Icon name={t.tone === 'green' ? 'check' : 'info'} class="size-4" /></span>{t.text}
		</button>
	{/each}
</div>
