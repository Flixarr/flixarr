<script lang="ts">
	// Fix for a download stuck in Radarr's or Sonarr's queue (#129): a different release (suggested), try the
	// import again, or remove it. Shown to people who manage requests and people with Fix and redownload.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Modal from './Modal.svelte';

	let { kind, queueId, title, ondone }: { kind: 'radarr' | 'sonarr'; queueId: number; title: string; ondone?: () => void } = $props();

	const app = $derived(kind === 'radarr' ? 'Radarr' : 'Sonarr');
	const CHOICES = $derived([
		{ action: 'different', label: 'Get a different release', about: `Removes this one from ${app} and the download client, blocks it so it isn't grabbed again, and searches for another.`, suggested: true },
		{ action: 'retry', label: 'Try the import again', about: `${app} looks at the downloaded files again. For when they weren't ready yet.` },
		{ action: 'remove', label: 'Remove it', about: `Removes it from ${app} and the download client without searching again.` }
	]);

	let open = $state(false);
	let busy = $state('');

	async function fix(action: string) {
		busy = action;
		try {
			const r = await api<{ done: string }>(`/queue/${kind}/${queueId}/fix`, { method: 'POST', body: JSON.stringify({ action }) });
			notify(`${r.done}.`, 'green');
			open = false;
			ondone?.();
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not fix it.', 'orange');
		} finally {
			busy = '';
		}
	}
</script>

<button class="btn-ghost mt-2 px-3 py-1 text-xs" onclick={() => (open = true)}>Fix</button>

<Modal bind:open title="Fix this download" sub={title}>
	<div class="space-y-2 p-5">
		{#each CHOICES as c (c.action)}
			<button class="w-full rounded-xl px-4 py-3 text-left ring-1 transition disabled:opacity-60 {c.suggested ? 'bg-primary/10 ring-primary/50 hover:bg-primary/15' : 'ring-gray-700/60 hover:bg-white/5'}" disabled={!!busy} onclick={() => fix(c.action)}>
				<span class="flex items-center justify-between gap-2 text-sm font-medium">{c.label}{#if c.suggested}<span class="text-[11px] font-normal text-primary-light">Suggested</span>{/if}{#if busy === c.action}<span class="text-[11px] font-normal text-gray-400">Working…</span>{/if}</span>
				<span class="mt-0.5 block text-xs text-gray-400">{c.about}</span>
			</button>
		{/each}
	</div>
</Modal>
