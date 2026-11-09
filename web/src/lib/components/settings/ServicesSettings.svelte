<script lang="ts">
	// Settings → Services: how the connected servers are doing, then the optional extras.
	import { api } from '#lib/api.ts';
	import type { ServiceKind } from '#lib/types.ts';
	import Icon from '#lib/components/Icon.svelte';
	import { on } from '#lib/live.svelte.ts';
	import ServiceCard from './ServiceCard.svelte';
	import ConnectionModal from './ConnectionModal.svelte';
	import WebhooksCard from './WebhooksCard.svelte';
	import MaintainerrSettings from './MaintainerrSettings.svelte';
	import { KINDS, health, type Services } from './services.ts';

	let saved = $state<Services | null>(null);
	let now = $state(Date.now());
	let modal = $state<ConnectionModal>();

	// The cards show what Radarr/Sonarr were last read as. They reload when that changes (pushed over the
	// live connection after every read, including webhook-triggered ones) or after a gap in the connection.
	$effect(() => {
		const load = () => api<Services>('/services').then((d) => (saved = d)).catch(() => {});
		load();
		const tick = setInterval(() => (now = Date.now()), 5000); // "Next update in" only; no requests
		const offs = [on('services', load), on('resync', load)];
		return () => {
			clearInterval(tick);
			offs.forEach((off) => off());
		};
	});

	const connected = $derived(saved ? KINDS.filter((k) => saved![k.kind]) : []);
	const trouble = $derived(saved ? connected.filter((k) => ['down', 'issues'].includes(health(saved![k.kind]!))) : []);
	const queued = $derived(saved ? connected.reduce((n, k) => n + (saved![k.kind]!.status?.queue.length ?? 0), 0) : 0);
	const summary = $derived.by(() => {
		if (!connected.length) return { text: 'Nothing connected yet', sub: 'Connect Radarr and Sonarr so requests have somewhere to go.', dot: 'bg-gray-500' };
		if (trouble.length) return { text: `${trouble.map((k) => saved![k.kind]!.name).join(' and ')} ${trouble.length === 1 ? 'needs' : 'need'} attention`, sub: 'Details are on the card below.', dot: 'bg-yellow-400' };
		return { text: 'Everything is running', sub: queued ? `${queued} item${queued === 1 ? '' : 's'} in the download queues.` : 'Nothing is downloading right now.', dot: 'bg-green-500' };
	});
	const set = (k: ServiceKind, v: Services[ServiceKind]) => (saved![k] = v);
</script>

{#if saved}
	<div class="flex items-center gap-3 px-1">
		<span class="size-2.5 shrink-0 rounded-full {summary.dot}"></span>
		<div class="min-w-0"><div class="text-base font-semibold">{summary.text}</div><div class="text-xs text-gray-400">{summary.sub}</div></div>
	</div>

	<div class="grid items-start gap-4 laptop:grid-cols-2">
		{#each KINDS as s (s.kind)}
			{@const v = saved[s.kind]}
			{#if v}
				<ServiceCard {s} {v} {now} onedit={() => modal?.edit(s.kind)} onremove={() => modal?.askRemove(s.kind)} onchange={(n) => set(s.kind, n)} />
			{:else}
				<button class="flex h-full min-h-40 flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-gray-700 p-6 text-sm text-gray-400 transition hover:border-gray-500 hover:text-white" onclick={() => modal?.edit(s.kind)}>
					<img src="/logos/{s.kind}.svg" alt="" class="size-9 opacity-80" />
					<span class="inline-flex items-center gap-1.5 font-medium"><Icon name="plus" class="size-4" />Add {s.name}</span>
					<span class="text-xs text-gray-500">{s.about}</span>
				</button>
			{/if}
		{/each}
	</div>

	<div class="grid items-start gap-4 laptop:grid-cols-2">
		<WebhooksCard kinds={connected} />
		<MaintainerrSettings />
	</div>
	<ConnectionModal bind:this={modal} bind:saved={saved} />
{/if}
