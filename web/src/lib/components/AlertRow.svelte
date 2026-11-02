<script lang="ts">
	// One alert in the bell: its picture, title, detail and when; red dot while it's new. A to-do has a coloured
	// edge. Grouped alerts (#185) use it for each title inside the group.
	import type { Alert } from '#lib/alerts.svelte.ts';
	import { ago, img } from '#lib/api.ts';
	import Icon from './Icon.svelte';
	let { a, onopen, inner = false }: { a: Alert; onopen: () => void; inner?: boolean } = $props();
</script>

<button class="flex w-full items-start gap-3 py-3 text-left hover:bg-white/5 {inner ? 'pl-8 pr-4' : 'px-4'} {a.kind === 'todo' ? 'border-l-2 border-l-primary' : ''}" onclick={onopen}>
	{#if a.poster}<img src={img(a.poster, 'w92')} alt="" class="h-12 w-8 shrink-0 rounded object-cover {a.tapped ? 'opacity-50' : ''}" />
	{:else if a.key.startsWith('arr:')}<img src="/logos/{a.key.split(':')[1]}.svg" alt="" class="mt-0.5 size-8 shrink-0 {a.tapped ? 'opacity-50' : ''}" />
	{:else}<span class="grid h-12 w-8 shrink-0 place-items-center rounded bg-gray-800 text-gray-400"><Icon name="bell" class="size-4" /></span>{/if}
	<span class="min-w-0 flex-1">
		<span class="block truncate text-sm {a.tapped ? 'text-gray-400' : 'font-semibold text-white'}">{inner ? a.detail.split(' · ')[0] : a.title}</span>
		<span class="block truncate text-xs {a.tapped ? 'text-gray-500' : 'text-gray-300'}">{inner ? a.detail.split(' · ').slice(1).join(' · ') : a.detail}</span>
		<span class="mt-0.5 block text-[11px] text-gray-500">{ago(a.at)}</span>
	</span>
	{#if a.new}<span class="mt-1.5 size-2 shrink-0 rounded-full bg-red-500"></span>{/if}
</button>
