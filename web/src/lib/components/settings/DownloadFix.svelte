<script lang="ts">
	// Fix on a stuck download: get a different copy (remove it, block that release, search again), or just
	// remove it. A download Sonarr matched by ID can only be imported by hand, so it also links to Sonarr's queue.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { QueueRow, ServiceKind } from '#lib/types.ts';
	import Icon from '#lib/components/Icon.svelte';
	import { needsManualImport } from './services.ts';

	let { q, kind, name, web, onfixed }: { q: QueueRow; kind: ServiceKind; name: string; web: string; onfixed: (id: number, action: 'different' | 'remove') => void } = $props();

	let open = $state(false);
	let busy = $state(false);
	let el = $state<HTMLElement>();

	$effect(() => {
		if (!open) return;
		const away = (e: PointerEvent) => !el?.contains(e.target as Node) && (open = false);
		document.addEventListener('pointerdown', away);
		return () => document.removeEventListener('pointerdown', away);
	});

	async function fix(action: 'different' | 'remove') {
		open = false;
		busy = true;
		try {
			await api(`/queue/${kind}/${q.id}/fix`, { method: 'POST', body: JSON.stringify({ action }) });
			onfixed(q.id, action);
			notify(action === 'different' ? `Looking for a different copy of ${q.title}.` : `Removed ${q.title} from the queue.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Unable to fix this download', 'orange');
		} finally {
			busy = false;
		}
	}
	const item = 'block w-full px-3 py-2 text-left hover:bg-white/5';
</script>

<span bind:this={el} class="relative inline-block">
	<button class="inline-flex items-center gap-1 rounded-md bg-yellow-500/15 px-2.5 py-1 text-[11px] font-medium text-yellow-200 ring-1 ring-yellow-500/40 hover:bg-yellow-500/25 disabled:opacity-50"
		disabled={busy} aria-expanded={open} onclick={() => (open = !open)}>{busy ? 'Fixing…' : 'Fix'}<Icon name="chevron" class="size-3 {open ? '-rotate-90' : 'rotate-90'}" /></button>
	{#if open}
		<div class="absolute right-0 top-full z-20 mt-1 w-64 max-w-[calc(100vw-2rem)] overflow-hidden rounded-lg border border-gray-700/60 bg-gray-900 py-1 shadow-[0_16px_40px_-4px_rgb(0_0_0/0.75),0_4px_12px_rgb(0_0_0/0.5)]">
			<button class={item} onclick={() => fix('different')}><span class="block text-xs font-medium text-gray-100">Get a different copy</span><span class="block text-[11px] text-gray-500">Removes this one, blocks it, and searches for another.</span></button>
			<button class={item} onclick={() => fix('remove')}><span class="block text-xs font-medium text-gray-100">Remove it</span><span class="block text-[11px] text-gray-500">Takes it out of the queue without searching again.</span></button>
			{#if needsManualImport(q)}
				<a class="{item} border-t border-gray-700/40" href="{web.replace(/\/$/, '')}/activity/queue" target="_blank" rel="noopener" onclick={() => (open = false)}>
					<span class="flex items-center gap-1.5 text-xs font-medium text-gray-100">Open in {name}<Icon name="external" class="size-3" /></span><span class="block text-[11px] text-gray-500">For a manual import when the name doesn't match.</span></a>
			{/if}
		</div>
	{/if}
</span>
