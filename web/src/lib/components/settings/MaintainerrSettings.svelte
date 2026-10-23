<script lang="ts">
	// Settings → Services → Maintainerr: the connection, what its rules are about to remove, keeping new
	// requests out of the rules for a while, and its webhook so changes reach Flixarr at once.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Select from '#lib/components/Select.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import Modal from '#lib/components/Modal.svelte';

	type Stat = { id: number; title: string; type: string; active: boolean; deleteAfterDays: number; leaving: number; handled: number; handledBytes: number };
	type Info = { connected: boolean; url?: string; keySet?: boolean; version?: string; error?: string; webhook?: boolean; protectDays: number; stats: Stat[]; leaving?: number; keeps?: number };
	let info = $state<Info | null>(null);
	let url = $state('');
	let apiKey = $state('');
	let base = $state('');
	let busy = $state('');
	let found = $state<string[] | null>(null);
	// The Add / Edit window, like Radarr and Sonarr's: nothing is saved until Save.
	let open = $state(false);
	let tested = $state('');
	let error = $state('');
	let confirmRemove = $state(false);
	function edit() {
		url = info?.url ?? '';
		apiKey = '';
		tested = '';
		error = '';
		found = null;
		open = true;
	}
	async function test() {
		busy = 'test';
		error = '';
		try {
			tested = (await api<{ version: string }>('/maintainerr/test', { method: 'POST', body: JSON.stringify({ url: url.trim(), apiKey: apiKey.trim() }) })).version;
		} catch (e) {
			tested = '';
			error = e instanceof Error ? e.message : 'Test failed.';
		} finally {
			busy = '';
		}
	}
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
			open = false;
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
			if (found.length) {
				url = found[0];
				test();
			}
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
		confirmRemove = false;
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
		{#if !info.connected}
			<!-- Not connected: what it would add, so admins know it's there. Nothing else in Flixarr mentions it until then. -->
			<div class="card space-y-3 p-4 text-sm">
				<p class="text-gray-300">Optional. <a href="https://maintainerr.info" target="_blank" rel="noopener" class="text-primary-light hover:underline">Maintainerr</a> removes titles from Plex by rules you set. Connect it and Flixarr adds:</p>
				<ul class="list-disc space-y-1 pl-5 text-gray-400">
					<li>a Leaving Soon row on Discover</li>
					<li>leaving dates on titles, with Keep and Ask to keep buttons</li>
					<li>a heads up a week before something someone requested leaves</li>
					<li>new requests kept safe from its rules for a while</li>
				</ul>
				<button class="btn-ghost w-full justify-center py-2.5" onclick={edit}><Icon name="plus" class="size-4" />Add Maintainerr</button>
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
				<button class="flex w-full items-center justify-center gap-1.5 py-2.5 text-xs text-gray-300 hover:bg-white/5 hover:text-white" onclick={edit}><Icon name="settings" class="size-3.5" />Edit connection</button>
			</div>
		{/if}
	</div>
{/if}

<Modal bind:open title={info?.connected ? 'Edit Maintainerr' : 'Add Maintainerr'} sub="Removes titles from Plex by rules you set in Maintainerr.">
	<div class="space-y-4 p-5">
		<div>
			<label for="mt-url" class="block text-sm">Address</label>
			<div class="relative">
				<input id="mt-url" class="{input} pr-20" bind:value={url} oninput={() => ((tested = ''), (error = ''))} placeholder="http://192.168.1.10:6246" autocomplete="off" spellcheck="false" />
				<button class="absolute right-2 top-1/2 mt-0.5 -translate-y-1/2 px-2 text-xs text-gray-400 hover:text-white disabled:opacity-50" disabled={!!busy} onclick={find}>{busy === 'find' ? 'Looking…' : 'Find it'}</button>
			</div>
			{#if found && found.length > 1}
				<div class="mt-2 flex flex-wrap gap-2">{#each found as f (f)}<button class="rounded-full border px-3 py-1 text-xs {url === f ? 'border-primary bg-primary/15 text-white' : 'border-gray-700 text-gray-300 hover:text-white'}" onclick={() => ((url = f), test())}>{f}</button>{/each}</div>
			{:else if found && !found.length}<p class="mt-1 text-xs text-yellow-400">No Maintainerr found nearby. Enter its address (it usually runs on port 6246).</p>{/if}
		</div>
		<div>
			<label for="mt-key" class="block text-sm">API key <span class="text-gray-500">(optional, only if your Maintainerr asks for one)</span></label>
			<input id="mt-key" class={input} type="password" bind:value={apiKey} oninput={() => (tested = '')} placeholder={info?.keySet ? 'Saved' : ''} autocomplete="off" />
		</div>
		<button class="btn-ghost w-full py-2 text-sm" disabled={!!busy || !url.trim()} onclick={test}>{busy === 'test' ? 'Testing…' : 'Test'}</button>
		{#if tested}<p class="flex items-center gap-2 text-sm text-green-400"><span class="size-2 rounded-full bg-green-500"></span>Connected · Maintainerr {tested}</p>{/if}
		{#if error}<p class="text-sm text-red-300">{error}</p>{/if}
	</div>
	{#snippet footer()}
		{#if info?.connected}<button class="btn-ghost mr-auto text-red-300" onclick={() => ((open = false), (confirmRemove = true))}>Remove</button>{/if}
		<button class="btn-ghost" onclick={() => (open = false)}>Cancel</button>
		<button class="btn px-6" disabled={!!busy || !tested} onclick={connect}>{busy === 'connect' ? 'Saving…' : 'Save'}</button>
	{/snippet}
</Modal>

<Modal bind:open={confirmRemove} title="Remove Maintainerr?" sub="The connection is deleted from Flixarr.">
	<p class="p-5 text-sm text-gray-300">Nothing changes inside Maintainerr, and titles Flixarr kept stay kept there. Flixarr stops showing what's leaving.</p>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => ((confirmRemove = false), edit())}>Cancel</button>
		<button class="btn px-6" onclick={disconnect}>Remove</button>
	{/snippet}
</Modal>
