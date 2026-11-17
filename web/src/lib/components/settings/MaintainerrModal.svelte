<script lang="ts">
	// Settings → Services → Maintainerr's Add / Edit window, drawn like Radarr's and Sonarr's: the connection on
	// a colored band, then (once connected) its rules, Keep new requests and its instant updates.
	// The connection and Keep new requests are saved with Save; Add to Maintainerr acts right away.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Select from '#lib/components/Select.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import { maintainerrOn, type MaintainerrInfo } from './services.ts';

	let { info = $bindable(), onchange }: { info: MaintainerrInfo | null; onchange: () => void } = $props();

	const RGB = '229 160 13';
	const ABOUT = 'Removes titles from Plex by rules you set in Maintainerr.';
	const PROTECT = [0, 7, 14, 30, 60, 90].map((d) => ({ value: d, label: d ? `${d} days` : 'Off', hint: d === 30 ? 'default' : '' }));
	const input = 'mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60';

	let open = $state(false);
	let confirmRemove = $state(false);
	let changing = $state(false);
	let url = $state('');
	let apiKey = $state('');
	let tested = $state('');
	let error = $state('');
	let busy = $state('');
	let found = $state<string[] | null>(null);
	let keepDays = $state(30);
	let base = $state('');

	const isNew = $derived(!maintainerrOn(info));
	const connected = $derived(!!info?.connected && !changing);

	export function edit() {
		url = info?.url ?? '';
		apiKey = '';
		tested = '';
		error = '';
		found = null;
		changing = isNew || !info?.connected;
		keepDays = info?.protectDays ?? 30;
		if (!base) api<{ base: string }>('/webhook').then((d) => (base = d.base || location.origin)).catch(() => {});
		open = true;
	}
	export function askRemove() {
		confirmRemove = true;
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
	const dirty = $derived((changing && !!tested) || (!changing && keepDays !== info?.protectDays));
	async function save() {
		busy = 'save';
		try {
			if (changing) {
				info = await api<MaintainerrInfo>('/maintainerr', { method: 'PUT', body: JSON.stringify({ url: url.trim(), apiKey: apiKey.trim() }) });
				notify(`Connected to Maintainerr ${info.version ?? ''}. Reading its rules now.`);
				setTimeout(onchange, 3000);
			}
			if (keepDays !== info?.protectDays) {
				info = await api<MaintainerrInfo>('/maintainerr', { method: 'PUT', body: JSON.stringify({ protectDays: keepDays }) });
				if (!changing) notify(keepDays ? `New requests stay out of Maintainerr's rules for ${keepDays} days after they arrive.` : 'New requests are no longer kept out of the rules.');
			}
			open = false;
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Unable to save', 'orange');
		} finally {
			busy = '';
		}
	}
	async function hook() {
		busy = 'hook';
		try {
			await api('/maintainerr/webhook', { method: 'POST', body: JSON.stringify({ base: base.trim() }) });
			info = { ...info!, webhook: true };
			notify('Maintainerr now tells Flixarr the moment its rules change.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Unable to add the webhook', 'orange');
		} finally {
			busy = '';
		}
	}
	async function remove() {
		confirmRemove = false;
		try {
			await api('/maintainerr', { method: 'DELETE' });
			onchange();
			notify('Maintainerr is disconnected. Titles Flixarr kept stay kept in Maintainerr.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Unable to remove Maintainerr', 'orange');
		}
	}
</script>

<Modal bind:open title={isNew ? 'Add Maintainerr' : 'Edit Maintainerr'}>
	{#snippet top()}
		<div class="px-5 pb-5 pt-5" style="background:linear-gradient(160deg, rgb({RGB} / 0.3), rgb({RGB} / 0.04))">
			<div class="mb-4 flex items-center gap-3 pr-8">
				<img src="/logos/maintainerr.svg" alt="" class="size-10 shrink-0" />
				<div><div class="text-lg font-semibold">{isNew ? 'Add Maintainerr' : 'Edit Maintainerr'}</div><div class="text-xs text-gray-300">{ABOUT}</div></div>
			</div>
			{#if connected}
				<div class="flex items-center gap-3 rounded-xl border border-green-500/40 bg-gray-900/90 p-4 shadow-xl">
					<span class="size-2 shrink-0 rounded-full bg-green-500"></span>
					<div class="min-w-0 flex-1"><div class="text-sm font-medium text-green-400">Connected{info?.version ? ` · v${info.version.split('.').slice(0, 2).join('.')}` : ''}</div><div class="truncate text-xs text-gray-500">{info?.url}</div></div>
					<button class="text-xs text-primary-light hover:underline" onclick={() => (changing = true)}>Change</button>
				</div>
			{:else}
				<div class="space-y-3 rounded-xl border border-gray-700/60 bg-gray-900/90 p-4 shadow-xl">
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
						<input id="mt-key" class="{input} font-mono" type="password" bind:value={apiKey} oninput={() => (tested = '')} placeholder={info?.keySet ? 'Saved' : ''} autocomplete="off" />
					</div>
					<div class="flex gap-2">
						<button class="btn-ghost flex-1 py-2 text-sm" disabled={!!busy || !url.trim()} onclick={test}>{busy === 'test' ? 'Testing…' : 'Test'}</button>
						{#if !isNew && info?.connected}<button class="btn-ghost px-4 py-2 text-sm" onclick={() => ((changing = false), (tested = ''))}>Cancel</button>{/if}
					</div>
					{#if tested}<p class="flex items-center gap-2 text-xs text-green-400"><span class="size-2 rounded-full bg-green-500"></span>Connected · Maintainerr {tested}</p>{/if}
					{#if error || (!changing && info?.error)}<p class="flex items-center gap-2 text-xs text-red-400"><span class="size-2 shrink-0 rounded-full bg-red-500"></span>{error || info?.error}</p>{/if}
				</div>
			{/if}
		</div>
	{/snippet}

	{#if isNew}
		<div class="px-5 py-4 text-sm text-gray-300">Connecting it adds:
			<ul class="mt-2 list-disc space-y-1 pl-5 text-xs text-gray-400">
				<li>a Leaving Soon row on Discover</li>
				<li>leaving dates on titles, with Keep and Ask to keep buttons</li>
				<li>a heads up a week before something someone requested leaves</li>
				<li>new requests kept safe from its rules for a while</li>
			</ul>
		</div>
	{:else if info?.connected}
		<div class="space-y-5 px-5 py-5">
			<div>
				<div class="text-sm">Its rules</div>
				<p class="text-xs text-gray-500">Set up in Maintainerr. Shown here so you can see what's leaving and when.</p>
				{#if info.stats.length}
					<div class="mt-2 divide-y divide-gray-700/40 rounded-lg bg-gray-800/40 ring-1 ring-gray-700/40">
						{#each info.stats as s (s.id)}
							<div class="flex items-center justify-between gap-3 px-3 py-2 text-sm">
								<span class="min-w-0 truncate">{s.title}{#if !s.active}<span class="ml-2 text-xs text-gray-500">paused</span>{/if}</span>
								<span class="shrink-0 text-xs text-gray-400">{s.type === 'movie' || s.type === 'show' ? `${s.leaving} leaving · ` : ''}removed after {s.deleteAfterDays} days</span>
							</div>
						{/each}
					</div>
				{:else}<p class="mt-2 text-xs text-gray-500">No rules set up in Maintainerr yet.</p>{/if}
			</div>
			<div>
				<label for="mt-keep" class="block text-sm">Keep new requests</label>
				<Select id="mt-keep" bind:value={keepDays} options={PROTECT} />
				<p class="mt-1 text-xs text-gray-500">After a request arrives in Plex, its title stays out of Maintainerr's rules this long, so it isn't removed before it's watched.</p>
			</div>
			<div class="space-y-2">
				<div class="flex items-center justify-between gap-3">
					<span><span class="block text-sm">Instant updates</span><span class="block text-xs text-gray-500">Maintainerr tells Flixarr when titles join, leave or are removed. Without it, Flixarr checks every 15 minutes.</span></span>
					{#if info.webhook}<span class="flex shrink-0 items-center gap-3"><span class="text-sm text-green-400">Added</span><button class="btn-ghost px-3 py-1 text-xs" disabled={!!busy} onclick={hook}>{busy === 'hook' ? 'Updating…' : 'Update'}</button></span>
					{:else}<button class="btn shrink-0 px-3 py-1 text-xs" disabled={!!busy || !base.trim()} onclick={hook}>{busy === 'hook' ? 'Adding…' : 'Add to Maintainerr'}</button>{/if}
				</div>
				<label for="mt-base" class="block text-xs text-gray-400">Flixarr's address, as Maintainerr sees it</label>
				<input id="mt-base" class="{input} mt-0" bind:value={base} placeholder="http://192.168.1.10:7575" autocomplete="off" spellcheck="false" />
			</div>
		</div>
	{:else}
		<p class="px-5 py-4 text-xs text-gray-500">Its settings show up here again once Maintainerr answers.</p>
	{/if}

	{#snippet footer()}
		{#if !isNew}<button class="btn-ghost mr-auto text-red-300" onclick={() => ((open = false), (confirmRemove = true))}>Remove</button>{/if}
		<button class="btn-ghost" onclick={() => (open = false)}>Cancel</button>
		<button class="btn px-6" disabled={!!busy || !dirty} onclick={save}>{busy === 'save' ? 'Saving…' : 'Save'}</button>
	{/snippet}
</Modal>

<Modal bind:open={confirmRemove} title="Remove Maintainerr?" sub="The connection and its settings are deleted from Flixarr.">
	<p class="p-5 text-sm text-gray-300">Leaving Soon, leaving dates and Keep buttons go away. Nothing changes inside Maintainerr.</p>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (confirmRemove = false)}>Cancel</button>
		<button class="btn px-6" onclick={remove}>Remove</button>
	{/snippet}
</Modal>
