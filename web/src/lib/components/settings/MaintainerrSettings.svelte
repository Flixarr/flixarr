<script lang="ts">
	// Settings → Services → Maintainerr: the connection, what its rules are about to remove, keeping new
	// requests out of the rules for a while, and its webhook so changes reach Flixarr at once.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Select from '#lib/components/Select.svelte';
	import Icon from '#lib/components/Icon.svelte';

	type Stat = { id: number; title: string; type: string; active: boolean; deleteAfterDays: number; leaving: number; handled: number; handledBytes: number };
	type Info = { connected: boolean; url?: string; keySet?: boolean; version?: string; error?: string; webhook?: boolean; protectDays: number; stats: Stat[]; leaving?: number; keeps?: number };
	let info = $state<Info | null>(null);
	let url = $state('');
	let apiKey = $state('');
	let base = $state('');
	let busy = $state('');
	let found = $state<string[] | null>(null);
	const input = 'mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60';

	async function load() {
		info = await api<Info>('/maintainerr');
		url = info.url ?? '';
	}
	$effect(() => {
		load().catch(() => {});
		api<{ base: string }>('/webhook').then((d) => (base = d.base || location.origin)).catch(() => {});
	});

	async function connect() {
		busy = 'connect';
		try {
			info = await api<Info>('/maintainerr', { method: 'PUT', body: JSON.stringify({ url: url.trim(), apiKey: apiKey.trim() }) });
			apiKey = '';
			notify(`Connected to Maintainerr ${info.version ?? ''}. Reading its collections now.`);
			setTimeout(() => load().catch(() => {}), 3000);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not connect.', 'orange');
		} finally {
			busy = '';
		}
	}
	async function find() {
		busy = 'find';
		try {
			found = (await api<{ found: string[] }>('/maintainerr/find', { method: 'POST' })).found;
			if (found.length) url = found[0];
			else notify("Didn't find it. Type its address (it usually runs on port 6246).", 'orange');
		} finally {
			busy = '';
		}
	}
	async function protect(days: number) {
		try {
			info = await api<Info>('/maintainerr', { method: 'PUT', body: JSON.stringify({ protectDays: days }) });
			notify(days ? `New requests stay out of Maintainerr's rules for ${days} days after they arrive.` : 'New requests are no longer kept out of the rules.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}
	async function hook() {
		busy = 'hook';
		try {
			await api('/maintainerr/webhook', { method: 'POST', body: JSON.stringify({ base: base.trim() }) });
			info = { ...info!, webhook: true };
			notify('Maintainerr now tells Flixarr the moment its collections change.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not add it.', 'orange');
		} finally {
			busy = '';
		}
	}
	async function disconnect() {
		await api('/maintainerr', { method: 'DELETE' });
		await load();
		notify('Maintainerr is disconnected. Titles Flixarr kept stay kept in Maintainerr.');
	}
	const size = (b: number) => (b >= 1e12 ? `${(b / 1e12).toFixed(1)} TB` : `${Math.round(b / 1e9)} GB`);
	const freed = $derived(info?.stats.reduce((n, s) => n + s.handledBytes, 0) ?? 0);
	const handled = $derived(info?.stats.reduce((n, s) => n + s.handled, 0) ?? 0);
	const PROTECT = [0, 7, 14, 30, 60, 90].map((d) => ({ value: d, label: d ? `${d} days` : 'Off', hint: d === 30 ? 'default' : '' }));
</script>

{#if info}
	<div>
		<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Maintainerr</div>
		<p class="-mt-1 mb-3 text-xs text-gray-500">Optional. If Maintainerr removes titles by rules, Flixarr shows what's leaving and when, lets people ask to keep a title, and keeps new requests safe for a while.</p>
		{#if !info.connected}
			<div class="card space-y-3 p-4">
				<div>
					<label for="mt-url" class="block text-sm">Address</label>
					<div class="flex gap-2"><input id="mt-url" class={input} bind:value={url} placeholder="http://192.168.1.10:6246" autocomplete="off" spellcheck="false" />
						<button class="btn-ghost mt-1 shrink-0 text-sm" disabled={!!busy} onclick={find}>{busy === 'find' ? 'Looking…' : 'Find automatically'}</button></div>
					{#if found && found.length > 1}<p class="mt-1 text-xs text-gray-500">Also found: {found.slice(1).join(', ')}</p>{/if}
				</div>
				<div>
					<label for="mt-key" class="block text-sm">API key <span class="text-gray-500">(only if your Maintainerr asks for one)</span></label>
					<input id="mt-key" class={input} type="password" bind:value={apiKey} autocomplete="off" />
				</div>
				<div class="flex justify-end"><button class="btn px-6" disabled={!!busy || !url.trim()} onclick={connect}>{busy === 'connect' ? 'Checking…' : 'Connect'}</button></div>
			</div>
		{:else}
			<div class="card divide-y divide-gray-700/40">
				<div class="flex items-center gap-3 p-4">
					<span class="grid size-8 shrink-0 place-items-center rounded-lg bg-gray-800 text-gray-300"><Icon name="clock" class="size-4" /></span>
					<span class="min-w-0 flex-1">
						<span class="block font-semibold">Maintainerr</span>
						<span class="block truncate text-[11px] text-gray-500">{info.url}{info.version ? ` · ${info.version}` : ''}</span>
					</span>
					{#if info.error}<span class="flex items-center gap-1.5 text-xs text-red-400"><span class="size-2 rounded-full bg-red-500"></span>Not connected</span>
					{:else}<span class="flex items-center gap-1.5 text-xs text-green-400"><span class="size-2 rounded-full bg-green-500"></span>Connected</span>{/if}
				</div>
				{#if info.error}<p class="px-4 py-3 text-sm text-red-300">{info.error}</p>{/if}
				<div class="grid grid-cols-3 gap-2 px-4 py-3 text-center">
					<a href="/leaving" class="rounded-lg bg-gray-800/50 px-2 py-2 hover:bg-gray-800"><div class="text-lg font-bold">{info.leaving ?? 0}</div><div class="text-[11px] text-gray-400">leaving soon</div></a>
					<div class="rounded-lg bg-gray-800/50 px-2 py-2"><div class="text-lg font-bold">{info.keeps ?? 0}</div><div class="text-[11px] text-gray-400">kept by Flixarr</div></div>
					<div class="rounded-lg bg-gray-800/50 px-2 py-2"><div class="text-lg font-bold">{size(freed)}</div><div class="text-[11px] text-gray-400">freed ({handled} titles)</div></div>
				</div>
				{#each info.stats as s (s.id)}
					<div class="flex items-center justify-between gap-3 px-4 py-2 text-sm">
						<span class="min-w-0 truncate">{s.title}{#if !s.active}<span class="ml-2 text-xs text-gray-500">paused</span>{/if}</span>
						<span class="shrink-0 text-xs text-gray-400">{s.type === 'movie' || s.type === 'show' ? `${s.leaving} leaving · ` : ''}removed after {s.deleteAfterDays} days</span>
					</div>
				{/each}
				<div class="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
					<span><span class="block text-sm">Keep new requests</span><span class="block text-xs text-gray-500">After a request arrives in Plex, its title stays out of Maintainerr's rules this long, so it isn't removed before it's watched.</span></span>
					<Select value={info.protectDays} class="w-36" options={PROTECT} onchange={protect} />
				</div>
				<div class="space-y-2 px-4 py-3">
					<div class="flex items-center justify-between gap-3">
						<span><span class="block text-sm">Instant updates</span><span class="block text-xs text-gray-500">Maintainerr tells Flixarr when titles join, leave or are removed. Without it, Flixarr checks every 15 minutes.</span></span>
						{#if info.webhook}<span class="flex shrink-0 items-center gap-3"><span class="text-sm text-green-400">Added</span><button class="btn-ghost px-3 py-1 text-xs" disabled={!!busy} onclick={hook}>{busy === 'hook' ? 'Updating…' : 'Update'}</button></span>
						{:else}<button class="btn shrink-0 px-3 py-1 text-xs" disabled={!!busy || !base.trim()} onclick={hook}>{busy === 'hook' ? 'Adding…' : 'Add to Maintainerr'}</button>{/if}
					</div>
					<label for="mt-base" class="block text-xs text-gray-400">Flixarr's address, as Maintainerr sees it</label>
					<input id="mt-base" class="{input} mt-0" bind:value={base} placeholder="http://192.168.1.10:7575" autocomplete="off" spellcheck="false" />
				</div>
				<div class="flex justify-end px-4 py-3"><button class="btn-ghost text-sm text-red-300" onclick={disconnect}>Disconnect</button></div>
			</div>
		{/if}
	</div>
{/if}
