<script lang="ts">
	import { api, fmtBytes } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { ArrInfo, ServiceKind, ServiceView } from '#lib/types.ts';
	import Toggle from '#lib/components/Toggle.svelte';
	import Modal from '#lib/components/Modal.svelte';

	// One Radarr and one Sonarr. The server stores connections as a list, so more can be added later.
	const KINDS: { kind: ServiceKind; name: string; what: string; port: number }[] = [
		{ kind: 'radarr', name: 'Radarr', what: 'Movies', port: 7878 },
		{ kind: 'sonarr', name: 'Sonarr', what: 'Series', port: 8989 }
	];

	type Form = { url: string; apiKey: string; rootFolder: string; qualityProfileId: number; seasonFolders: boolean; searchOnAdd: boolean; enabled: boolean };
	type State = { saved: ServiceView | null; form: Form; info: ArrInfo | null; error: string; testing: boolean; saving: boolean };

	const blank = (): Form => ({ url: '', apiKey: '', rootFolder: '', qualityProfileId: 0, seasonFolders: true, searchOnAdd: true, enabled: true });
	const fromSaved = (v: ServiceView): Form => ({ url: v.url, apiKey: '', rootFolder: v.rootFolder, qualityProfileId: v.qualityProfileId, seasonFolders: v.seasonFolders, searchOnAdd: v.searchOnAdd, enabled: v.enabled });

	let st = $state<Record<ServiceKind, State> | null>(null);
	let removing = $state<ServiceKind | null>(null);
	let removeOpen = $state(false);

	$effect(() => {
		api<Record<ServiceKind, ServiceView | null>>('/services').then((d) => {
			st = {
				radarr: { saved: d.radarr, form: d.radarr ? fromSaved(d.radarr) : blank(), info: null, error: '', testing: false, saving: false },
				sonarr: { saved: d.sonarr, form: d.sonarr ? fromSaved(d.sonarr) : blank(), info: null, error: '', testing: false, saving: false }
			};
			// Saved connections are checked right away so their lists fill in.
			for (const { kind } of KINDS) if (d[kind]) test(kind, true);
		});
	});

	async function test(kind: ServiceKind, quiet = false) {
		const s = st![kind];
		s.testing = true;
		s.error = '';
		try {
			s.info = await api<ArrInfo>(`/services/${kind}/test`, { method: 'POST', body: JSON.stringify(s.form) });
			if (!s.form.rootFolder && s.info.folders.length) s.form.rootFolder = s.info.folders[0].path;
			if (!s.info.profiles.some((p) => p.id === s.form.qualityProfileId)) s.form.qualityProfileId = s.info.profiles[0]?.id ?? 0;
			if (!quiet) notify(`Connected to ${KINDS.find((k) => k.kind === kind)!.name} ${s.info.version}.`);
		} catch (e) {
			s.info = null;
			s.error = e instanceof Error ? e.message : 'Test failed.';
		} finally {
			s.testing = false;
		}
	}

	async function save(kind: ServiceKind) {
		const s = st![kind];
		s.saving = true;
		try {
			s.saved = await api<ServiceView>(`/services/${kind}`, { method: 'PUT', body: JSON.stringify(s.form) });
			s.form.apiKey = '';
			notify(`${s.saved.name} saved.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		} finally {
			s.saving = false;
		}
	}

	async function remove(kind: ServiceKind) {
		try {
			await api(`/services/${kind}`, { method: 'DELETE' });
			st![kind] = { saved: null, form: blank(), info: null, error: '', testing: false, saving: false };
			notify(`${KINDS.find((k) => k.kind === kind)!.name} removed.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not remove.', 'orange');
		} finally {
			removeOpen = false;
		}
	}

	// The connection details changed since the last good test, so the lists may not match anymore.
	function edited(kind: ServiceKind) {
		st![kind].info = null;
		st![kind].error = '';
	}
</script>

{#snippet field(label: string, id: string, hint = '')}
	<label for={id} class="block text-sm">{label}</label>
	{#if hint}<span class="block text-xs text-gray-500">{hint}</span>{/if}
{/snippet}

{#if st}
	{#each KINDS as k (k.kind)}
		{@const s = st[k.kind]}
		<div>
			<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">{k.name}</div>
			<div class="card divide-y divide-gray-700/40 px-4">
				<div class="flex items-center gap-3 py-3">
					<span class="size-2.5 shrink-0 rounded-full {s.info ? 'bg-green-500' : s.error ? 'bg-red-500' : 'bg-gray-600'}"></span>
					<span class="min-w-0 flex-1 text-sm">
						<span class="block">{k.what}</span>
						<span class="block text-xs {s.error ? 'text-red-400' : 'text-gray-500'}">
							{#if s.testing}Checking the connection…{:else if s.error}{s.error}{:else if s.info}Connected · {k.name} {s.info.version}{:else if s.saved}Saved, not checked yet{:else}Not connected{/if}
						</span>
					</span>
					<button class="btn-ghost px-3 py-1 text-xs" disabled={s.testing || !s.form.url} onclick={() => test(k.kind)}>{s.testing ? 'Testing…' : 'Test'}</button>
				</div>

				<div class="grid gap-3 py-3 tablet:grid-cols-2">
					<div>
						{@render field('Address', `${k.kind}-url`)}
						<input id="{k.kind}-url" bind:value={s.form.url} oninput={() => edited(k.kind)} placeholder="http://192.168.1.10:{k.port}" autocomplete="off" spellcheck="false"
							class="mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60" />
					</div>
					<div>
						{@render field('API key', `${k.kind}-key`)}
						<input id="{k.kind}-key" bind:value={s.form.apiKey} oninput={() => edited(k.kind)} autocomplete="off" spellcheck="false" type="password"
							placeholder={s.saved?.apiKeySet ? `Saved, ends in ${s.saved.apiKeyEnd}` : `${k.name} → Settings → General`}
							class="mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 font-mono text-sm outline-none focus:ring-2 focus:ring-primary/60" />
					</div>
				</div>

				{#if s.info}
					<div class="grid gap-3 py-3 tablet:grid-cols-2">
						<div>
							{@render field('Root folder', `${k.kind}-root`, `Where new ${k.what.toLowerCase()} are saved.`)}
							<select id="{k.kind}-root" bind:value={s.form.rootFolder} class="mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm">
								{#each s.info.folders as f (f.id)}<option value={f.path}>{f.path}{f.accessible ? ` (${fmtBytes(f.freeSpace)} free)` : ' (not reachable)'}</option>{/each}
							</select>
							{#if !s.info.folders.length}<span class="mt-1 block text-xs text-yellow-400">Add a root folder in {k.name} first (Settings → Media Management).</span>{/if}
						</div>
						<div>
							{@render field('Default quality', `${k.kind}-quality`, 'Used for requests unless another quality is picked.')}
							<select id="{k.kind}-quality" bind:value={s.form.qualityProfileId} class="mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm">
								{#each s.info.profiles as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
							</select>
						</div>
					</div>
					<div class="space-y-3 py-3">
						<Toggle checked={s.form.searchOnAdd} label="Search as soon as it's requested" about="{k.name} starts looking for a download right away instead of waiting for its next scheduled search." onchange={(on) => (s.form.searchOnAdd = on)} />
						{#if k.kind === 'sonarr'}
							<Toggle checked={s.form.seasonFolders} label="Season folders" about="Each season goes in its own folder inside the series folder." onchange={(on) => (s.form.seasonFolders = on)} />
						{/if}
						<Toggle checked={s.form.enabled} label="Send requests here" about="Turn off to stop sending new requests to {k.name} without removing the connection." onchange={(on) => (s.form.enabled = on)} />
					</div>
				{/if}

				<div class="flex flex-wrap items-center gap-2 py-3">
					<button class="btn px-5 py-1.5 text-sm" disabled={!s.info || s.saving || !s.form.rootFolder || !s.form.qualityProfileId} onclick={() => save(k.kind)}>{s.saving ? 'Saving…' : 'Save'}</button>
					{#if !s.info}<span class="text-xs text-gray-500">Test the connection to pick a folder and quality, then save.</span>{/if}
					{#if s.saved}<button class="btn-ghost ml-auto px-3 py-1.5 text-xs" onclick={() => { removing = k.kind; removeOpen = true; }}>Remove</button>{/if}
				</div>
			</div>
		</div>
	{/each}

	<Modal bind:open={removeOpen} title="Remove {KINDS.find((k) => k.kind === removing)?.name}?" sub="The connection and its settings are deleted from Flixarr.">
		<p class="p-5 text-sm text-gray-300">Nothing changes inside {KINDS.find((k) => k.kind === removing)?.name}. New requests can't be sent there until it's connected again.</p>
		{#snippet footer()}
			<button class="btn-ghost" onclick={() => (removeOpen = false)}>Cancel</button>
			<button class="btn px-6" onclick={() => removing && remove(removing)}>Remove</button>
		{/snippet}
	</Modal>
{/if}
