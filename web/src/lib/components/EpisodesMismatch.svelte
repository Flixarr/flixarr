<script lang="ts">
	// Episodes mismatch (#174): Plex numbers a series' episodes unlike Sonarr. Fix makes Plex number them
	// the way Sonarr does (people who manage requests only).
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { can } from '#lib/session.svelte.ts';

	let { tmdbId, ondone }: { tmdbId: number; ondone?: () => void } = $props();
	let busy = $state(false);

	async function fix() {
		busy = true;
		try {
			await api(`/titles/series/${tmdbId}/plexorder`, { method: 'POST' });
			notify('Plex is renumbering the episodes. It takes a minute.');
			ondone?.();
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not fix it.', 'orange');
		} finally {
			busy = false;
		}
	}
</script>

<div class="flex items-start gap-3 rounded-lg bg-yellow-500/10 px-3 py-2.5 text-yellow-100 ring-1 ring-yellow-500/30">
	<div class="min-w-0 flex-1">
		<p class="font-medium">Episodes mismatch</p>
		<p class="mt-0.5 text-xs text-yellow-100/80">Plex and Sonarr number this show's episodes differently, so some episodes may be missing in Plex. Press Fix to make Plex number them the same way Sonarr does.</p>
	</div>
	{#if can('manageRequests')}<button class="btn shrink-0 px-3 py-1 text-xs" disabled={busy} onclick={fix}>{busy ? 'Fixing…' : 'Fix'}</button>{/if}
</div>
