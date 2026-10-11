<script lang="ts">
	import { api, fmtBytes } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { ArrInfo, ServiceKind, ServiceView } from '#lib/types.ts';
	import Toggle from '#lib/components/Toggle.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import Icon from '#lib/components/Icon.svelte';

	// Each kind is a list of connection cards. Connections are stored as a list; for now each kind
	// takes one, so "Add" shows only while a list is empty.
	const KINDS: { kind: ServiceKind; name: string; what: string; port: number; about: string }[] = [
		{ kind: 'radarr', name: 'Radarr', what: 'movies', port: 7878, about: 'Movie requests are sent here.' },
		{ kind: 'sonarr', name: 'Sonarr', what: 'series', port: 8989, about: 'Series requests are sent here.' }
	];
	const nameOf = (kind: ServiceKind) => KINDS.find((k) => k.kind === kind)!.name;

	type Form = { name: string; url: string; apiKey: string; rootFolder: string; qualityProfileId: number; seasonFolders: boolean; searchOnAdd: boolean; enabled: boolean };
	type Check = { info: ArrInfo | null; error: string; busy: boolean };

	let saved = $state<Record<ServiceKind, ServiceView | null> | null>(null);
	let checks = $state<Record<ServiceKind, Check>>({ radarr: { info: null, error: '', busy: false }, sonarr: { info: null, error: '', busy: false } });

	// The open modal.
	let open = $state(false);
	let kind = $state<ServiceKind>('radarr');
	let form = $state<Form>(blank('radarr'));
	let info = $state<ArrInfo | null>(null);
	let error = $state('');
	let testing = $state(false);
	let saving = $state(false);
	let showKey = $state(false);
	let finding = $state(false);
	let found = $state<string[] | null>(null);
	let confirmRemove = $state(false);

	function blank(k: ServiceKind): Form {
		return { name: nameOf(k), url: '', apiKey: '', rootFolder: '', qualityProfileId: 0, seasonFolders: true, searchOnAdd: true, enabled: true };
	}

	$effect(() => {
		api<Record<ServiceKind, ServiceView | null>>('/services').then((d) => {
			saved = d;
			for (const { kind: k } of KINDS) if (d[k]) check(k);
		});
	});

	// Background check of a saved connection, for the card's status dot.
	async function check(k: ServiceKind) {
		const v = saved![k]!;
		checks[k] = { info: null, error: '', busy: true };
		try {
			checks[k].info = await api<ArrInfo>(`/services/${k}/test`, { method: 'POST', body: JSON.stringify({ url: v.url, apiKey: '' }) });
		} catch (e) {
			checks[k].error = e instanceof Error ? e.message : 'Not reachable.';
		} finally {
			checks[k].busy = false;
		}
	}

	function edit(k: ServiceKind) {
		const v = saved![k];
		kind = k;
		form = v ? { name: v.name, url: v.url, apiKey: '', rootFolder: v.rootFolder, qualityProfileId: v.qualityProfileId, seasonFolders: v.seasonFolders, searchOnAdd: v.searchOnAdd, enabled: v.enabled } : blank(k);
		info = v ? checks[k].info : null;
		error = v ? checks[k].error : '';
		showKey = false;
		found = null;
		open = true;
	}

	async function test() {
		testing = true;
		error = '';
		try {
			info = await api<ArrInfo>(`/services/${kind}/test`, { method: 'POST', body: JSON.stringify(form) });
			if (!info.folders.some((f) => f.path === form.rootFolder)) form.rootFolder = info.folders[0]?.path ?? '';
			if (!info.profiles.some((p) => p.id === form.qualityProfileId)) form.qualityProfileId = info.profiles[0]?.id ?? 0;
		} catch (e) {
			info = null;
			error = e instanceof Error ? e.message : 'Test failed.';
		} finally {
			testing = false;
		}
	}

	async function find() {
		finding = true;
		found = null;
		try {
			found = (await api<{ found: { url: string }[] }>(`/services/${kind}/discover`, { method: 'POST' })).found.map((f) => f.url);
			if (found.length === 1) pick(found[0]);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Search failed.', 'orange');
		} finally {
			finding = false;
		}
	}

	function pick(url: string) {
		form.url = url;
		addressChanged();
	}

	// A new address may be a different server, so its folders and qualities have to be loaded again.
	function addressChanged() {
		info = null;
		error = '';
	}

	async function save() {
		saving = true;
		try {
			const v = await api<ServiceView>(`/services/${kind}`, { method: 'PUT', body: JSON.stringify(form) });
			saved![kind] = v;
			checks[kind] = { info, error: '', busy: false };
			open = false;
			notify(`${v.name} saved.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		} finally {
			saving = false;
		}
	}

	async function remove() {
		try {
			await api(`/services/${kind}`, { method: 'DELETE' });
			const n = saved![kind]?.name ?? nameOf(kind);
			saved![kind] = null;
			checks[kind] = { info: null, error: '', busy: false };
			confirmRemove = false;
			open = false;
			notify(`${n} removed.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not remove.', 'orange');
		}
	}

	const input = 'mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60';
	const k = $derived(KINDS.find((x) => x.kind === kind)!);
</script>

{#snippet label(text: string, id: string, hint = '')}
	<label for={id} class="block text-sm">{text}</label>
	{#if hint}<span class="block text-xs text-gray-500">{hint}</span>{/if}
{/snippet}

{#if saved}
	{#each KINDS as s (s.kind)}
		{@const v = saved[s.kind]}
		{@const c = checks[s.kind]}
		<div>
			<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">{s.name}</div>
			<p class="-mt-1 mb-3 text-xs text-gray-500">{s.about}</p>
			<div class="grid gap-3 tablet:grid-cols-2">
				{#if v}
					<button class="card flex items-start gap-3 p-4 text-left transition hover:border-gray-600" onclick={() => edit(s.kind)}>
						<span class="mt-1.5 size-2.5 shrink-0 rounded-full {c.busy ? 'animate-pulse bg-gray-500' : c.info ? 'bg-green-500' : 'bg-red-500'}"></span>
						<span class="min-w-0 flex-1 text-sm">
							<span class="block font-medium">{v.name}</span>
							<span class="block truncate text-xs text-gray-400">{v.url}</span>
							<span class="mt-1 block text-xs text-gray-500">{v.qualityProfileName} · {v.rootFolder}</span>
							{#if c.error}<span class="mt-1 block text-xs text-red-400">{c.error}</span>{:else if c.info}<span class="mt-1 block text-xs text-gray-500">{s.name} {c.info.version}</span>{/if}
						</span>
						<span class="text-gray-500"><Icon name="chevron" class="size-4" /></span>
					</button>
				{:else}
					<button class="flex items-center justify-center gap-2 rounded-xl border border-dashed border-gray-700 p-6 text-sm text-gray-400 transition hover:border-gray-500 hover:text-white" onclick={() => edit(s.kind)}>
						<Icon name="plus" class="size-4" />Add {s.name} server
					</button>
				{/if}
			</div>
		</div>
	{/each}

	<Modal bind:open title={saved[kind] ? `Edit ${saved[kind]!.name}` : `Add ${k.name} server`} sub="Sends {k.what} requests to {k.name}.">
		<div class="space-y-4 p-5">
			<div>
				{@render label('Name', 'svc-name', 'Shown on the card. Handy once there is more than one server.')}
				<input id="svc-name" bind:value={form.name} maxlength="60" class={input} />
			</div>

			<div>
				<div class="flex items-end justify-between gap-3">
					<span>{@render label('Address', 'svc-url', 'Include the port, and the URL base if one is set.')}</span>
					<button class="btn-ghost shrink-0 px-3 py-1 text-xs" disabled={finding} onclick={find}>{finding ? 'Looking…' : 'Find automatically'}</button>
				</div>
				<input id="svc-url" bind:value={form.url} oninput={addressChanged} placeholder="http://192.168.1.10:{k.port}" autocomplete="off" spellcheck="false" class={input} />
				{#if found}
					{#if found.length > 1}
						<div class="mt-2 flex flex-wrap gap-2">
							{#each found as f (f)}<button class="rounded-full border px-3 py-1 text-xs {form.url === f ? 'border-primary bg-primary/15 text-white' : 'border-gray-700 text-gray-300 hover:text-white'}" onclick={() => pick(f)}>{f}</button>{/each}
						</div>
					{:else if found.length === 0}
						<p class="mt-1 text-xs text-yellow-400">No {k.name} found nearby. Enter its address above.</p>
					{:else}
						<p class="mt-1 text-xs text-green-400">Found {k.name} at this address.</p>
					{/if}
				{/if}
			</div>

			<div>
				{@render label('API key', 'svc-key', `In ${k.name}: Settings → General → Security.`)}
				<div class="relative">
					<input id="svc-key" bind:value={form.apiKey} oninput={() => (error = '')} type={showKey ? 'text' : 'password'} autocomplete="off" spellcheck="false"
						placeholder={saved[kind]?.apiKeySet ? `Saved, ends in ${saved[kind]!.apiKeyEnd}` : ''} class="{input} pr-16 font-mono" />
					<button class="absolute right-2 top-1/2 mt-0.5 -translate-y-1/2 px-2 text-xs text-gray-400 hover:text-white" onclick={() => (showKey = !showKey)}>{showKey ? 'Hide' : 'Show'}</button>
				</div>
			</div>

			<div class="flex items-center gap-3">
				<button class="btn-ghost px-4 py-1.5 text-sm" disabled={testing || !form.url} onclick={test}>{testing ? 'Testing…' : 'Test'}</button>
				<span class="text-xs {error ? 'text-red-400' : info ? 'text-green-400' : 'text-gray-500'}">
					{#if error}{error}{:else if info}Connected · {k.name} {info.version}{:else}Test the connection to pick a folder and quality.{/if}
				</span>
			</div>

			{#if info}
				<div class="grid gap-4 tablet:grid-cols-2">
					<div>
						{@render label('Root folder', 'svc-root', `Where new ${k.what} are saved.`)}
						<select id="svc-root" bind:value={form.rootFolder} class={input}>
							{#each info.folders as f (f.id)}<option value={f.path}>{f.path}{f.accessible ? ` (${fmtBytes(f.freeSpace)} free)` : ' (not reachable)'}</option>{/each}
						</select>
						{#if !info.folders.length}<span class="mt-1 block text-xs text-yellow-400">Add a root folder in {k.name} first (Settings → Media Management).</span>{/if}
					</div>
					<div>
						{@render label('Default quality', 'svc-quality', 'Used for requests unless another quality is picked.')}
						<select id="svc-quality" bind:value={form.qualityProfileId} class={input}>
							{#each info.profiles as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
						</select>
					</div>
				</div>
				<div class="space-y-3">
					<Toggle checked={form.searchOnAdd} label="Search as soon as it's requested" about="{k.name} starts looking for a download right away instead of waiting for its next scheduled search." onchange={(on) => (form.searchOnAdd = on)} />
					{#if kind === 'sonarr'}
						<Toggle checked={form.seasonFolders} label="Season folders" about="Each season goes in its own folder inside the series folder." onchange={(on) => (form.seasonFolders = on)} />
					{/if}
				</div>
			{/if}
		</div>
		{#snippet footer()}
			{#if saved?.[kind]}<button class="btn-ghost mr-auto text-red-300" onclick={() => (confirmRemove = true)}>Remove</button>{/if}
			<button class="btn-ghost" onclick={() => (open = false)}>Cancel</button>
			<button class="btn px-6" disabled={!info || saving || !form.rootFolder || !form.qualityProfileId} onclick={save}>{saving ? 'Saving…' : 'Save'}</button>
		{/snippet}
	</Modal>

	<Modal bind:open={confirmRemove} title="Remove {saved[kind]?.name ?? k.name}?" sub="The connection and its settings are deleted from Flixarr.">
		<p class="p-5 text-sm text-gray-300">Nothing changes inside {k.name}. New requests can't be sent there until it's connected again.</p>
		{#snippet footer()}
			<button class="btn-ghost" onclick={() => (confirmRemove = false)}>Cancel</button>
			<button class="btn px-6" onclick={remove}>Remove</button>
		{/snippet}
	</Modal>
{/if}
