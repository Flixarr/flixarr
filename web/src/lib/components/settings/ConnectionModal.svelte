<script lang="ts">
	// Add or edit a Radarr or Sonarr connection, and the window that confirms removing one.
	import { api, fmtBytes } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { ArrInfo, FlixarrProfile, ServiceKind, ServiceView } from '#lib/types.ts';
	import Toggle from '#lib/components/Toggle.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import Select from '#lib/components/Select.svelte';
	import { KINDS, type Services } from './services.ts';

	let { saved = $bindable() }: { saved: Services } = $props();
	const nameOf = (kind: ServiceKind) => KINDS.find((k) => k.kind === kind)!.name;

	type Form = { name: string; url: string; webUrl: string; apiKey: string; rootFolder: string; qualityProfileId: number; seasonFolders: boolean; searchOnAdd: boolean; enabled: boolean; addProfiles: string[]; defaultProfile: string };

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
	let flixarrProfiles = $state<FlixarrProfile[]>([]);
	let profileBusy = $state('');
	let webFromArr = $state(false); // the web address was filled in from the Application URL
	let changing = $state(false); // editing: the connection form is open ("Change")
	let initial = $state(''); // the form as it opened, so Save only lights up after a change
	const dirty = $derived(JSON.stringify(form) !== initial);

	export function askRemove(k: ServiceKind) {
		kind = k;
		confirmRemove = true;
	}

	function blank(k: ServiceKind): Form {
		return { addProfiles: [], defaultProfile: '', name: nameOf(k), url: '', webUrl: '', apiKey: '', rootFolder: '', qualityProfileId: 0, seasonFolders: true, searchOnAdd: true, enabled: true };
	}

	export function edit(k: ServiceKind) {
		const v = saved![k];
		kind = k;
		form = v ? { addProfiles: [], defaultProfile: '', name: v.name, url: v.url, webUrl: v.webUrl, apiKey: '', rootFolder: v.rootFolder, qualityProfileId: v.qualityProfileId, seasonFolders: v.seasonFolders, searchOnAdd: v.searchOnAdd, enabled: v.enabled } : blank(k);
		info = null;
		error = '';
		showKey = false;
		found = null;
		changing = false;
		initial = JSON.stringify(form);
		open = true;
		webFromArr = false;
		// Flixarr's own profiles: offered (all ticked) when adding a server, and the default quality.
		if (!v) api<{ profiles: FlixarrProfile[] }>('/profiles').then((d) => {
			flixarrProfiles = d.profiles.filter((p) => p.apps.includes(k));
			form.addProfiles = flixarrProfiles.filter((p) => p.defaultOn).map((p) => p.id);
			form.defaultProfile = form.addProfiles.includes('standard') ? 'standard' : (form.addProfiles[0] ?? '');
			initial = JSON.stringify(form);
		}).catch(() => (flixarrProfiles = []));
		if (v) test(true); // fills the folder and quality lists
	}

	// auto = the quiet check when an existing connection is opened; it doesn't fill in anything.
	async function test(auto = false) {
		testing = true;
		error = '';
		try {
			info = await api<ArrInfo>(`/services/${kind}/test`, { method: 'POST', body: JSON.stringify(form) });
			if (!info.folders.some((f) => f.path === form.rootFolder)) form.rootFolder = info.folders[0]?.path ?? '';
			if (!info.profiles.some((p) => p.id === form.qualityProfileId)) form.qualityProfileId = info.profiles[0]?.id ?? 0;
			changing = false;
			// Use Radarr/Sonarr's own Application URL for Open, unless something is typed in already.
			if (!auto && !form.webUrl.trim() && info.applicationUrl) {
				form.webUrl = info.applicationUrl;
				webFromArr = true;
			}
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
			notify(e instanceof Error ? e.message : 'Unable to search', 'orange');
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

	// Existing server: add one Flixarr profile, or take Flixarr's version of one edited in Radarr/Sonarr.
	async function profileAction(key: string, action: 'add' | 'use-flixarr' | 'remove') {
		profileBusy = key;
		try {
			const v = await api<ServiceView>(action === 'add' ? `/services/${kind}/profiles` : `/services/${kind}/profiles/${key}/${action}`, { method: 'POST', body: JSON.stringify({ keys: [key] }) });
			saved[kind] = v;
			await test(true); // the new profile shows up in the default quality list
			notify(action === 'add' ? 'Profile added.' : action === 'remove' ? 'Profile removed.' : "Flixarr's version is back in place.");
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Unable to do that', 'orange');
		} finally {
			profileBusy = '';
		}
	}

	// The default quality picker: Flixarr profiles first (ones about to be added count, marked so), then the server's own.
	const qualityOptions = $derived.by(() => {
		if (!info) return [];
		const opts: { value: string; label: string; hint?: string }[] = [];
		for (const p of flixarrProfiles) if (form.addProfiles.includes(p.id)) opts.push({ value: `f:${p.id}`, label: p.arrName, hint: 'added on save' });
		const own = [...info.profiles].sort((a, b) => Number(b.name.startsWith('Flixarr · ')) - Number(a.name.startsWith('Flixarr · ')));
		for (const p of own) if (!opts.some((o) => o.label === p.name)) opts.push({ value: `p:${p.id}`, label: p.name });
		return opts;
	});
	const qualityValue = $derived(form.defaultProfile ? `f:${form.defaultProfile}` : `p:${form.qualityProfileId}`);
	function pickQuality(v: string) {
		if (v.startsWith('f:')) { form.defaultProfile = v.slice(2); form.qualityProfileId = 0; }
		else { form.defaultProfile = ''; form.qualityProfileId = +v.slice(2); }
	}
	function toggleAdd(key: string, on: boolean) {
		form.addProfiles = on ? [...form.addProfiles, key] : form.addProfiles.filter((k) => k !== key);
		if (!on && form.defaultProfile === key) { form.defaultProfile = ''; form.qualityProfileId = info?.profiles[0]?.id ?? 0; }
	}

	async function save() {
		saving = true;
		const wasNew = !saved[kind];
		try {
			const v = await api<ServiceView>(`/services/${kind}`, { method: 'PUT', body: JSON.stringify({ ...form, addProfiles: wasNew ? form.addProfiles : [] }) });
			saved[kind] = v;
			open = false;
			if (v.warning) notify(v.warning, 'orange');
			else notify(wasNew && form.addProfiles.length ? `${v.name} added, with Flixarr's quality profiles.` : `${v.name} saved.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Unable to save this server', 'orange');
		} finally {
			saving = false;
		}
	}

	async function remove() {
		try {
			await api(`/services/${kind}`, { method: 'DELETE' });
			const n = saved[kind]?.name ?? nameOf(kind);
			saved[kind] = null;
			confirmRemove = false;
			open = false;
			notify(`${n} removed.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Unable to remove this server', 'orange');
		}
	}

	const input = 'mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60';
	const k = $derived(KINDS.find((x) => x.kind === kind)!);
</script>

{#snippet flixarrBox()}
	{@const isNewServer = !saved[kind]}
	{@const list = saved[kind]?.profiles ?? []}
	{#if isNewServer ? flixarrProfiles.length : list.length}
		<div class="rounded-xl border border-primary/40 bg-primary/10 p-4">
			<div class="flex items-center gap-2 text-sm font-medium"><span class="text-primary-light">◆</span>Flixarr quality profiles</div>
			<p class="mt-0.5 text-xs text-gray-300">Tuned profiles Flixarr adds to {KINDS.find((x) => x.kind === kind)?.name} and keeps up to date. Your own profiles aren't changed.</p>
			<div class="mt-3 space-y-2">
				{#if isNewServer}
					{#each flixarrProfiles as p (p.id)}
						<label class="flex cursor-pointer items-start gap-3 rounded-lg bg-gray-900/50 px-3 py-2 ring-1 ring-gray-700/50">
							<input type="checkbox" class="mt-0.5 accent-[var(--color-primary)]" checked={form.addProfiles.includes(p.id)} onchange={(e) => toggleAdd(p.id, (e.currentTarget as HTMLInputElement).checked)} />
							<span class="min-w-0"><span class="block text-sm">{p.label}</span><span class="block text-xs text-gray-400">{p.description}</span></span>
						</label>
					{/each}
				{:else}
					{#each list as p (p.key)}
						<div class="flex items-center gap-3 rounded-lg bg-gray-900/50 px-3 py-2 ring-1 ring-gray-700/50">
							<span class="min-w-0 flex-1"><span class="block text-sm">{p.label}</span>
								<span class="block text-xs {p.state === 'ok' ? 'text-green-400' : p.state === 'customized' ? 'text-yellow-300' : 'text-gray-400'}">
									{#if p.state === 'ok'}Added · up to date{:else if p.state === 'update'}Added · a newer version is ready{:else if p.state === 'customized'}Edited in {KINDS.find((x) => x.kind === kind)?.name}{p.updateAvailable ? ' · a newer Flixarr version is available' : ''}{:else if p.state === 'missing'}Deleted in {KINDS.find((x) => x.kind === kind)?.name}{:else}Not added{/if}
								</span></span>
							{#if !p.installed}
								<button class="btn-ghost shrink-0 px-3 py-1 text-xs" disabled={profileBusy !== ''} onclick={() => profileAction(p.key, 'add')}>{profileBusy === p.key ? 'Adding…' : p.state === 'missing' ? 'Add again' : 'Add'}</button>
							{:else}
								{#if p.updateAvailable}<button class="btn-ghost shrink-0 px-3 py-1 text-xs" disabled={profileBusy !== ''} onclick={() => profileAction(p.key, 'use-flixarr')}>{profileBusy === p.key ? 'Updating…' : p.state === 'customized' ? "Use Flixarr's version" : 'Update'}</button>{/if}
								<button class="btn-ghost shrink-0 px-3 py-1 text-xs" disabled={profileBusy !== ''} onclick={() => profileAction(p.key, 'remove')}>Remove</button>
							{/if}
						</div>
					{/each}
				{/if}
			</div>
		</div>
	{/if}
{/snippet}

{#if saved}
	{@const isNew = !saved[kind]}
	{@const connected = !!info && !changing}
	<Modal bind:open title={isNew ? `Add ${k.name} server` : `Edit ${saved[kind]!.name}`}>
		{#snippet top()}
			<!-- The window's top is a band in the app's color, holding the connection (design E4.2). -->
			<div class="px-5 pb-5 pt-5" style="background:linear-gradient(160deg, rgb({k.rgb} / 0.3), rgb({k.rgb} / 0.04))">
				<div class="mb-4 flex items-center gap-3 pr-8">
					<img src="/logos/{kind}.svg" alt="" class="size-10 shrink-0" />
					<div><div class="text-lg font-semibold">{isNew ? `Add ${k.name} server` : `Edit ${saved[kind]!.name}`}</div><div class="text-xs text-gray-300">{k.about}</div></div>
				</div>
				{#if connected}
					<div class="swap-in flex items-center gap-3 rounded-xl border border-green-500/40 bg-gray-900/90 p-4 shadow-xl">
						<span class="size-2 shrink-0 rounded-full bg-green-500"></span>
						<div class="min-w-0 flex-1"><div class="text-sm font-medium text-green-400">Connected · v{info!.version.split('.').slice(0, 2).join('.')}</div><div class="truncate text-xs text-gray-500">{form.url}</div></div>
						<button class="text-xs text-primary-light hover:underline" onclick={() => (changing = true)}>Change</button>
					</div>
				{:else}
					<div class="space-y-3 rounded-xl border border-gray-700/60 bg-gray-900/90 p-4 shadow-xl">
						<div>
							<label for="svc-url" class="block text-sm">Address</label>
							<div class="relative">
								<input id="svc-url" bind:value={form.url} oninput={addressChanged} placeholder="http://192.168.1.10:{k.port}" autocomplete="off" spellcheck="false" class="{input} pr-20" />
								<button class="absolute right-2 top-1/2 mt-0.5 -translate-y-1/2 px-2 text-xs text-gray-400 hover:text-white disabled:opacity-50" disabled={finding} onclick={find}>{finding ? 'Looking…' : 'Find it'}</button>
							</div>
							{#if found && found.length > 1}
								<div class="mt-2 flex flex-wrap gap-2">
									{#each found as f (f)}<button class="rounded-full border px-3 py-1 text-xs {form.url === f ? 'border-primary bg-primary/15 text-white' : 'border-gray-700 text-gray-300 hover:text-white'}" onclick={() => pick(f)}>{f}</button>{/each}
								</div>
							{:else if found && found.length === 0}
								<p class="mt-1 text-xs text-yellow-400">No {k.name} found nearby. Enter its address.</p>
							{/if}
						</div>
						<div>
							<label for="svc-key" class="block text-sm">API key</label>
							<div class="relative">
								<input id="svc-key" bind:value={form.apiKey} oninput={() => (error = '')} type={showKey ? 'text' : 'password'} autocomplete="off" spellcheck="false"
									placeholder={saved[kind]?.apiKeySet ? `Saved, ends in ${saved[kind]!.apiKeyEnd}` : `${k.name} → Settings → General → Security`} class="{input} pr-16 font-mono" />
								<button class="absolute right-2 top-1/2 mt-0.5 -translate-y-1/2 px-2 text-xs text-gray-400 hover:text-white" onclick={() => (showKey = !showKey)}>{showKey ? 'Hide' : 'Show'}</button>
							</div>
						</div>
						<div class="flex gap-2">
							<button class="btn-ghost flex-1 py-2 text-sm" disabled={testing || !form.url} onclick={() => test()}>{testing ? 'Testing…' : 'Test'}</button>
							{#if changing && info}<button class="btn-ghost px-4 py-2 text-sm" onclick={() => (changing = false)}>Cancel</button>{/if}
						</div>
						{#if error}<p class="flex items-center gap-2 text-xs text-red-400"><span class="size-2 shrink-0 rounded-full bg-red-500"></span>{error}</p>{/if}
					</div>
				{/if}
			</div>
		{/snippet}

		<!-- The settings stay hidden until the connection works. -->
		<div class="reveal px-5 {info ? 'open' : ''}" inert={!info}>
			<div>
				{#if info}
					<div class="space-y-4 py-5">
						<div>
							<label for="svc-root" class="block text-sm">Root folder</label>
							<Select id="svc-root" bind:value={form.rootFolder} placeholder="Pick a folder" search={info.folders.length > 8}
								options={info.folders.map((f) => ({ value: f.path, label: f.path, hint: f.accessible ? `${fmtBytes(f.freeSpace)} free` : 'not reachable' }))} />
							{#if !info.folders.length}<p class="mt-1 text-xs text-yellow-400">Add a root folder in {k.name} first (Settings → Media Management).</p>{/if}
						</div>
						{@render flixarrBox()}
						<div>
							<label for="svc-quality" class="block text-sm">Default quality</label>
							<Select id="svc-quality" value={qualityValue} placeholder="Pick a quality" search={qualityOptions.length > 8} options={qualityOptions} onchange={pickQuality} />
						</div>
						<Toggle checked={form.searchOnAdd} label="Search right away" about="Start looking for a download as soon as something is requested." onchange={(on) => (form.searchOnAdd = on)} />
						{#if kind === 'sonarr'}
							<Toggle checked={form.seasonFolders} label="Season folders" about="Each season gets its own folder." onchange={(on) => (form.seasonFolders = on)} />
						{/if}
						<div>
							<label for="svc-name" class="block text-sm">Name</label>
							<input id="svc-name" bind:value={form.name} maxlength="60" placeholder={k.name} class={input} />
						</div>
						<div>
							<label for="svc-web" class="block text-sm">Web address <span class="text-gray-500">(optional)</span></label>
							<input id="svc-web" bind:value={form.webUrl} oninput={() => (webFromArr = false)} placeholder={form.url || `https://${k.kind}.example.com`} autocomplete="off" spellcheck="false" class={input} />
							<p class="mt-1 text-xs text-gray-500">{webFromArr ? `From ${k.name}'s Application URL (Settings → General → Host). Change it if Flixarr should link somewhere else.` : `Used by Open ${k.name} when the address above only works from the server.`}</p>
						</div>
					</div>
				{/if}
			</div>
		</div>
		{#snippet footer()}
			{#if !isNew}<button class="btn-ghost mr-auto text-red-300" onclick={() => { open = false; confirmRemove = true; }}>Remove</button>{/if}
			<button class="btn-ghost" onclick={() => (open = false)}>Cancel</button>
			<button class="btn px-6" disabled={!info || changing || saving || !form.rootFolder || (!form.qualityProfileId && !form.defaultProfile) || (!isNew && !dirty)} onclick={save}>{saving ? 'Saving…' : isNew ? 'Add server' : 'Save'}</button>
		{/snippet}
	</Modal>

	<Modal bind:open={confirmRemove} title="Remove {saved[kind]?.name ?? k.name}?" sub="The connection and its settings are deleted from Flixarr.">
		<p class="p-5 text-sm text-gray-300">Nothing changes inside {k.name}. New requests can't be sent there until it's connected again.</p>
		{#snippet footer()}
			<button class="btn-ghost" onclick={() => { confirmRemove = false; edit(kind); }}>Cancel</button>
			<button class="btn px-6" onclick={remove}>Remove</button>
		{/snippet}
	</Modal>
{/if}
