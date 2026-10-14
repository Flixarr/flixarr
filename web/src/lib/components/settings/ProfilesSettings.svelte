<script lang="ts">
	// Settings → Quality profiles: what each Flixarr profile does, and where it's been added.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { ServiceKind, ServiceView } from '#lib/types.ts';

	type Detail = { basedOn: string; basedOnUrl: string; qualities: string[]; cutoff: string; formats: number; preferred: string[]; blocked: string[] };
	type Entry = { id: string; name: string; label: string; description: string; arrName: string; rev: string; apps: ServiceKind[]; details: Record<string, Detail> };
	type Info = { version: string; source: { name: string; url: string; commit: string; license: string }; profiles: Entry[] };

	const APPS: { kind: ServiceKind; name: string }[] = [{ kind: 'radarr', name: 'Radarr' }, { kind: 'sonarr', name: 'Sonarr' }];

	let info = $state<Info | null>(null);
	let servers = $state<Record<ServiceKind, ServiceView | null> | null>(null);
	let error = $state('');
	let syncing = $state(false);
	let open = $state<Record<string, boolean>>({});

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
	const stateText: Record<string, string> = { ok: 'Up to date', update: 'Updates on the next sync', customized: 'Edited there', missing: 'Deleted there', '': 'Not added' };
	const stateTone: Record<string, string> = { ok: 'bg-green-500/10 text-green-300 ring-green-500/30', update: 'bg-gray-800 text-gray-300 ring-gray-700/60', customized: 'bg-yellow-500/10 text-yellow-300 ring-yellow-500/30', missing: 'bg-gray-800 text-gray-400 ring-gray-700/60', '': 'bg-gray-800/60 text-gray-500 ring-gray-700/40' };
</script>

{#if error}
	<div class="card p-4 text-sm text-red-300">{error}</div>
{:else if info && servers}
	<div class="card flex flex-wrap items-center gap-4 p-4">
		<span class="grid size-10 shrink-0 place-items-center rounded-xl bg-primary/15 text-lg text-primary-light ring-1 ring-primary/40">◆</span>
		<div class="min-w-0 flex-1 text-sm">
			<div class="font-medium">Flixarr quality profiles</div>
			<div class="text-xs text-gray-400">Version {when(info.version)} · built on the <a href={info.source.url} target="_blank" rel="noopener" class="text-primary-light hover:underline">{info.source.name}</a>. Updates arrive on their own; profiles you edit in Radarr or Sonarr are left alone.</div>
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
{/if}
