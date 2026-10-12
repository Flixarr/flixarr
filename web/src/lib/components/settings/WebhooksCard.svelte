<script lang="ts">
	// Settings → Services → Instant updates: a Webhook connection in Radarr and Sonarr, so grabs, imports
	// and health problems reach Flixarr at once. Add adds it for you (Radarr/Sonarr tests it first).
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { ServiceKind } from '#lib/types.ts';

	let { kinds }: { kinds: { kind: ServiceKind; name: string }[] } = $props();

	type Info = { path: string; base: string; last: string };
	let info = $state<Info | null>(null);
	let base = $state('');
	let added = $state<Partial<Record<ServiceKind, boolean>>>({});
	let busy = $state<ServiceKind | ''>('');
	const input = 'mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60';

	$effect(() => {
		api<Info>('/webhook').then((d) => {
			info = d;
			base = d.base || location.origin;
		});
		for (const k of kinds) api<{ added: boolean }>(`/services/${k.kind}/webhook`).then((d) => (added = { ...added, [k.kind]: d.added })).catch(() => {});
	});

	async function add(k: { kind: ServiceKind; name: string }) {
		busy = k.kind;
		try {
			await api(`/services/${k.kind}/webhook`, { method: 'POST', body: JSON.stringify({ base: base.trim() }) });
			added = { ...added, [k.kind]: true };
			notify(`${k.name} now tells Flixarr about grabs, imports and problems right away.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not add it.', 'orange');
		} finally {
			busy = '';
		}
	}
	async function copy() {
		const url = base.trim().replace(/\/$/, '') + info!.path;
		try {
			await navigator.clipboard.writeText(url);
			notify('Webhook URL copied.');
		} catch {
			notify(url, 'orange');
		}
	}
</script>

{#if info && kinds.length}
	<div>
		<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Instant updates</div>
		<div class="card divide-y divide-gray-700/40 px-4">
			<p class="py-3 text-sm text-gray-400">Without this, Flixarr checks Radarr and Sonarr every few seconds while something is on its way. With it, they tell Flixarr the moment a download is grabbed or imported, or when something goes wrong.</p>
			<div class="py-3">
				<label for="wh-base" class="block text-sm">Flixarr's address, as Radarr and Sonarr see it</label>
				<input id="wh-base" class={input} bind:value={base} autocomplete="off" spellcheck="false" placeholder="http://192.168.1.10:7575" />
				<p class="mt-1 text-xs text-gray-500">Usually the local IP and port. Not an address behind a login page (like Cloudflare Access); Radarr and Sonarr can't sign in.</p>
			</div>
			{#each kinds as k (k.kind)}
				<div class="flex items-center justify-between gap-3 py-3 text-sm">
					<span class="flex items-center gap-2"><img src="/logos/{k.kind}.svg" alt="" class="size-5" />{k.name}</span>
					{#if added[k.kind]}
						<span class="flex items-center gap-3"><span class="text-green-400">Added</span><button class="btn-ghost px-3 py-1 text-xs" disabled={!!busy} onclick={() => add(k)}>{busy === k.kind ? 'Updating…' : 'Update'}</button></span>
					{:else}
						<button class="btn px-3 py-1 text-xs" disabled={!!busy || !base.trim()} onclick={() => add(k)}>{busy === k.kind ? 'Adding…' : `Add to ${k.name}`}</button>
					{/if}
				</div>
			{/each}
			<div class="flex flex-wrap items-center justify-between gap-2 py-3 text-xs text-gray-500">
				<span>Prefer to do it yourself? Settings → Connect → + → Webhook, method POST, with this URL.</span>
				<button class="text-primary-light hover:underline" onclick={copy}>Copy the URL</button>
			</div>
			{#if info.last}<p class="py-3 text-xs text-gray-500">Last heard: {info.last}</p>{/if}
		</div>
	</div>
{/if}
