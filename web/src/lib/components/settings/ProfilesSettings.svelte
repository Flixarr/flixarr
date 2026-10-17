<script lang="ts">
	// Settings → Quality profiles: what each Flixarr profile does, and where it's been added. Newer versions
	// wait here for an admin: what changes, Update (warns first if the profile was edited) or Skip.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Modal from '#lib/components/Modal.svelte';
	import type { ServiceKind, ServiceView } from '#lib/types.ts';

	type Changes = { edited: boolean; qualitiesAdded: string[]; qualitiesRemoved: string[]; cutoffFrom?: string; cutoffTo?: string; formatsAdded: string[]; formatsRemoved: string[]; scoresChanged: { name: string; from: number; to: number }[] };

	type Detail = { basedOn: string; basedOnUrl: string; qualities: string[]; cutoff: string; formats: number; preferred: string[]; blocked: string[] };
	type Entry = { id: string; name: string; label: string; description: string; arrName: string; rev: string; apps: ServiceKind[]; details: Record<string, Detail> };
	type Info = { version: string; source: { name: string; url: string; commit: string; license: string }; profiles: Entry[] };

	const APPS: { kind: ServiceKind; name: string }[] = [{ kind: 'radarr', name: 'Radarr' }, { kind: 'sonarr', name: 'Sonarr' }];

	let info = $state<Info | null>(null);
	let servers = $state<Record<ServiceKind, ServiceView | null> | null>(null);
	let error = $state('');
	let syncing = $state(false);
	let open = $state<Record<string, boolean>>({});
	let changes = $state<Record<string, Changes | 'loading' | string>>({}); // "<kind>:<key>"
	let busy = $state('');
	let confirm = $state<{ kind: ServiceKind; key: string; label: string; app: string } | null>(null);
	let confirmOpen = $state(false);

	async function showChanges(kind: ServiceKind, key: string) {
		const k = `${kind}:${key}`;
		if (changes[k] && changes[k] !== 'loading') return void delete changes[k];
		changes[k] = 'loading';
		try {
			changes[k] = await api<Changes>(`/services/${kind}/profiles/${key}/changes`);
		} catch (e) {
			changes[k] = e instanceof Error ? e.message : "Couldn't compare.";
		}
	}
	async function act(kind: ServiceKind, key: string, what: 'use-flixarr' | 'skip') {
		busy = `${kind}:${key}`;
		confirmOpen = false;
		try {
			servers![kind] = await api<ServiceView>(`/services/${kind}/profiles/${key}/${what}`, { method: 'POST' });
			delete changes[`${kind}:${key}`];
			notify(what === 'skip' ? 'Skipped. The next version will be offered when it comes.' : 'Updated.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not do that.', 'orange');
		} finally {
			busy = '';
		}
	}
	const scoreText = (n: number) => (n > 0 ? `+${n}` : `${n}`);

	async function load() {
		try {
			[info, servers] = await Promise.all([api<Info>('/profiles'), api<Record<ServiceKind, ServiceView | null>>('/services')]);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load the profiles.';
		}
	}
	$effect(() => {
		load();
	});

	async function syncNow() {
		syncing = true;
		try {
			await api('/jobs/profiles-sync/run', { method: 'POST' });
			// The job runs in the background; give it a moment, then show the result.
			await new Promise((r) => setTimeout(r, 4000));
			await load();
			notify('Quality profiles synced.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not sync.', 'orange');
		} finally {
			syncing = false;
		}
	}

	// "2026-10-12 04:44" from the version stamp 202610120444.
	const when = (v: string) => (v.length === 12 ? `${v.slice(0, 4)}-${v.slice(4, 6)}-${v.slice(6, 8)} ${v.slice(8, 10)}:${v.slice(10)}` : v);
	const stateText: Record<string, string> = { ok: 'Up to date', update: 'Update available', customized: 'Edited there', missing: 'Deleted there', '': 'Not added' };
	const stateTone: Record<string, string> = { ok: 'bg-green-500/10 text-green-300 ring-green-500/30', update: 'bg-primary/10 text-primary-light ring-primary/40', customized: 'bg-yellow-500/10 text-yellow-300 ring-yellow-500/30', missing: 'bg-gray-800 text-gray-400 ring-gray-700/60', '': 'bg-gray-800/60 text-gray-500 ring-gray-700/40' };
</script>

{#if error}
	<div class="card p-4 text-sm text-red-300">{error}</div>
{:else if info && servers}
	<div class="card flex flex-wrap items-center gap-4 p-4">
		<span class="grid size-10 shrink-0 place-items-center rounded-xl bg-primary/15 text-lg text-primary-light ring-1 ring-primary/40">◆</span>
		<div class="min-w-0 flex-1 text-sm">
			<div class="font-medium">Flixarr quality profiles</div>
			<div class="text-xs text-gray-400">Version {when(info.version)} · built on the <a href={info.source.url} target="_blank" rel="noopener" class="text-primary-light hover:underline">{info.source.name}</a>. New versions download on their own and wait here until you apply them.</div>
		</div>
		<button class="btn-ghost shrink-0 px-3 py-1.5 text-xs" disabled={syncing} onclick={syncNow}>{syncing ? 'Syncing…' : 'Sync now'}</button>
	</div>

	<!-- One card per profile: what it aims for and where it is at a glance; the long lists behind Details. -->
	<div class="grid gap-3 tablet:grid-cols-2">
		{#each info.profiles as p (p.id)}
			{@const apps = APPS.filter((a) => p.apps.includes(a.kind))}
			{@const first = p.details[apps[0]?.kind]}
			<div class="card flex flex-col p-4">
				<div class="flex items-baseline justify-between gap-3">
					<h3 class="text-base font-bold">{p.label}</h3>
					{#if first}<span class="shrink-0 rounded-md bg-primary/15 px-2 py-0.5 text-[11px] font-semibold text-primary-light ring-1 ring-primary/40">Up to {first.cutoff}</span>{/if}
				</div>
				<p class="mt-1.5 flex-1 text-sm text-gray-400">{p.description}</p>
				{#if first?.preferred.length}<p class="mt-2 text-xs text-gray-500">Prefers <span class="text-gray-300">{first.preferred.slice(0, 3).join(', ')}{first.preferred.length > 3 ? ` +${first.preferred.length - 3}` : ''}</span></p>{/if}
				<div class="mt-3 flex flex-wrap gap-2">
					{#each apps as a (a.kind)}
						{@const st = servers[a.kind]?.profiles.find((x) => x.key === p.id)}
						<span class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-[11px] font-medium ring-1 {servers[a.kind] ? stateTone[st?.state ?? ''] : stateTone['']}">
							<img src="/logos/{a.kind}.svg" alt="" class="size-3.5" />{servers[a.kind] ? `${a.name} · ${stateText[st?.state ?? ''].replace(' there', '')}` : `${a.name} isn't connected`}
						</span>
					{/each}
				</div>
				{#each apps as a (a.kind)}
					{@const st = servers[a.kind]?.profiles.find((x) => x.key === p.id)}
					{@const ck = `${a.kind}:${p.id}`}
					{@const ch = changes[ck]}
					{#if st?.installed && st.updateAvailable}
						<div class="mt-3 rounded-lg bg-primary/10 p-3 text-xs ring-1 ring-primary/30">
							<div class="flex flex-wrap items-center gap-2">
								<span class="min-w-0 flex-1 text-gray-200">A newer version is ready for {a.name}{st.state === 'customized' ? '. It was edited there; updating replaces those edits.' : '.'}</span>
								<button class="text-gray-400 hover:text-white" onclick={() => showChanges(a.kind, p.id)}>{ch && ch !== 'loading' ? 'Hide changes' : ch === 'loading' ? 'Comparing…' : 'What changes'}</button>
							</div>
							{#if ch && typeof ch === 'object'}
								{@const none = !ch.qualitiesAdded.length && !ch.qualitiesRemoved.length && !ch.cutoffTo && !ch.formatsAdded.length && !ch.formatsRemoved.length && !ch.scoresChanged.length}
								<ul class="mt-2 list-disc space-y-0.5 pl-4 text-gray-300">
									{#if ch.qualitiesAdded.length}<li>Allows {ch.qualitiesAdded.join(', ')}</li>{/if}
									{#if ch.qualitiesRemoved.length}<li>No longer allows {ch.qualitiesRemoved.join(', ')}</li>{/if}
									{#if ch.cutoffTo}<li>Stops upgrading at {ch.cutoffTo} instead of {ch.cutoffFrom}</li>{/if}
									{#if ch.formatsAdded.length}<li>New formats: {ch.formatsAdded.join(', ')}</li>{/if}
									{#if ch.formatsRemoved.length}<li>Formats dropped: {ch.formatsRemoved.join(', ')}</li>{/if}
									{#each ch.scoresChanged.slice(0, 8) as sc (sc.name)}<li>{sc.name}: {scoreText(sc.from)} to {scoreText(sc.to)}</li>{/each}
									{#if ch.scoresChanged.length > 8}<li>{ch.scoresChanged.length - 8} more scores changed</li>{/if}
									{#if none}<li>Only small changes to how formats are matched.</li>{/if}
								</ul>
							{:else if typeof ch === 'string' && ch !== 'loading'}<p class="mt-2 text-red-300">{ch}</p>{/if}
							<div class="mt-2 flex justify-end gap-2">
								<button class="btn-ghost px-3 py-1 text-xs" disabled={busy !== ''} onclick={() => act(a.kind, p.id, 'skip')}>Skip</button>
								<button class="btn px-3 py-1 text-xs" disabled={busy !== ''} onclick={() => (st.state === 'customized' ? ((confirm = { kind: a.kind, key: p.id, label: p.label, app: a.name }), (confirmOpen = true)) : act(a.kind, p.id, 'use-flixarr'))}>{busy === ck ? 'Updating…' : 'Update'}</button>
							</div>
						</div>
					{/if}
				{/each}
				<button class="mt-3 self-start text-xs text-gray-400 hover:text-white" aria-expanded={!!open[p.id]} onclick={() => (open[p.id] = !open[p.id])}>{open[p.id] ? 'Hide details' : 'Details'}</button>
				{#if open[p.id]}
					<div class="mt-2 space-y-3 border-t border-gray-700/40 pt-3 text-xs">
						{#each apps as a (a.kind)}
							{@const d = p.details[a.kind]}
							<div class="space-y-1">
								<div class="flex items-center gap-2 font-medium text-gray-200"><img src="/logos/{a.kind}.svg" alt="" class="size-4" />{a.name}</div>
								<div class="grid gap-x-4 gap-y-1 tablet:grid-cols-[6rem_1fr]">
									<span class="text-gray-500">Qualities</span><span class="text-gray-300">{d.qualities.join(' › ')} <span class="text-gray-500">(stops upgrading at {d.cutoff})</span></span>
									<span class="text-gray-500">Prefers</span><span class="text-gray-300">{d.preferred.join(', ') || 'nothing in particular'}</span>
									<span class="text-gray-500">Never grabs</span><span class="text-gray-300">{d.blocked.join(', ') || 'nothing blocked'}</span>
									<span class="text-gray-500">Based on</span><span class="text-gray-300"><a href={d.basedOnUrl} target="_blank" rel="noopener" class="hover:underline">TRaSH "{d.basedOn}"</a> · {d.formats} custom formats</span>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		{/each}
	</div>

	<p class="text-xs text-gray-500">Added to Radarr and Sonarr as "{info.profiles[0]?.arrName}" and so on. Custom formats already on a server are reused when they match exactly; otherwise Flixarr adds its own copy, marked "(Flixarr)", and never changes yours. Add or remove profiles from each server's connection window in Services.</p>

	<Modal bind:open={confirmOpen} title="Replace your edits?" sub="{confirm?.label} in {confirm?.app}">
		<p class="p-5 text-sm text-gray-300">This profile was changed in {confirm?.app} after Flixarr added it. Updating puts Flixarr's new version in place and those changes are lost.</p>
		{#snippet footer()}
			<button class="btn-ghost" onclick={() => (confirmOpen = false)}>Cancel</button>
			<button class="btn px-6" onclick={() => confirm && act(confirm.kind, confirm.key, 'use-flixarr')}>Update</button>
		{/snippet}
	</Modal>
{/if}
