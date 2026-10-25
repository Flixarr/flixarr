<script lang="ts">
	// Where a title's files are: the folder's own name big, the path above it small, the size and quality on
	// the right, and (for people who manage requests) the file name.
	import { fmtBytes } from '#lib/api.ts';
	import Icon from './Icon.svelte';
	let { path = '', size, quality = '', release = '', sub = '' }: { path?: string; size: number; quality?: string; release?: string; sub?: string } = $props();
	const cut = $derived(path.replace(/\/$/, '').lastIndexOf('/'));
	const name = $derived(cut >= 0 ? path.replace(/\/$/, '').slice(cut + 1) : path);
	const parent = $derived(cut >= 0 ? path.slice(0, cut + 1) : '');
</script>

<div class="p-3">
	<div class="flex items-end justify-between gap-3">
		<span class="flex min-w-0 items-center gap-2">
			<span class="grid size-7 shrink-0 place-items-center rounded-md bg-gray-900/70 text-gray-400 ring-1 ring-gray-700/50"><Icon name="folder" class="size-3.5" /></span>
			<span class="min-w-0 leading-tight"><span class="block truncate text-xs font-medium text-gray-200">{name || 'Files'}</span>{#if parent}<span class="block truncate font-mono text-[10px] text-gray-500">{parent}</span>{:else if sub}<span class="block truncate text-[10px] text-gray-500">{sub}</span>{/if}</span>
		</span>
		<span class="shrink-0 text-right leading-tight"><span class="block text-xs font-semibold tabular-nums text-gray-100">{fmtBytes(size)}</span>{#if quality}<span class="block text-[10px] text-gray-500">{quality}</span>{/if}</span>
	</div>
	{#if release}<p class="mt-2 break-all font-mono text-[10px] text-gray-500">{release}</p>{/if}
</div>
