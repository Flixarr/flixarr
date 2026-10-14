<script lang="ts">
	// Settings → Blocklist: titles nobody can request. Block them from a title's page.
	import { api, fmtDate } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { remember } from '#lib/availability.svelte.ts';
	import Img from '#lib/components/Img.svelte';
	import Toggle from '#lib/components/Toggle.svelte';

	type Blocked = { type: 'movie' | 'series'; tmdbId: number; title: string; year: string; poster: string; userName: string; createdAt: string };
	let list = $state<Blocked[] | null>(null);
	let hide = $state<boolean | null>(null);
	$effect(() => {
		api<Blocked[]>('/blocklist').then((d) => (list = d));
		api<{ hide: boolean }>('/settings/blocklist').then((d) => (hide = d.hide));
	});
	async function setHide(on: boolean) {
		try {
			hide = (await api<{ hide: boolean }>('/settings/blocklist', { method: 'PUT', body: JSON.stringify({ hide: on }) })).hide;
			notify(on ? 'Blocked titles are hidden from everyone who doesn\'t manage requests.' : "Blocked titles show again, as Can't be requested.");
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}
	async function unblock(b: Blocked) {
		try {
			await api(`/blocklist/${b.type}/${b.tmdbId}`, { method: 'DELETE' });
			list = list!.filter((x) => !(x.type === b.type && x.tmdbId === b.tmdbId));
			remember(`${b.type}:${b.tmdbId}`, null);
			notify(`${b.title} can be requested again.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not unblock it.', 'orange');
		}
	}
</script>

{#if hide !== null}
	<div class="card mb-4 px-4 py-3">
		<Toggle checked={hide} label="Hide blocked titles" about="Leaves them out of Discover, search, browse, collections and recommendations. People who manage requests still see them, so they can unblock." onchange={setHide} />
	</div>
{/if}
{#if list}
	{#if !list.length}
		<div class="card p-4 text-sm text-gray-400">Nothing is blocked. To block a title, open it and press Block next to Request.</div>
	{:else}
		<div class="card divide-y divide-gray-700/40 px-4">
			{#each list as b (b.type + b.tmdbId)}
				<div class="flex items-center gap-3 py-3">
					<a href="/{b.type}/{b.tmdbId}" class="w-10 shrink-0 overflow-hidden rounded ring-1 ring-gray-700/50">{#if b.poster}<Img path={b.poster} kind="poster" sizes="40px" class="aspect-[2/3] w-full" />{/if}</a>
					<div class="min-w-0 flex-1 text-sm">
						<a href="/{b.type}/{b.tmdbId}" class="font-medium hover:underline">{b.title}</a> <span class="text-gray-500">{b.year}</span>
						<div class="text-xs text-gray-500">{b.type === 'movie' ? 'Movie' : 'Series'} · blocked {fmtDate(b.createdAt, { month: 'short', day: 'numeric', year: 'numeric' })}{b.userName ? ` by ${b.userName}` : ''}</div>
					</div>
					<button class="btn-ghost shrink-0 px-3 py-1 text-xs" onclick={() => unblock(b)}>Unblock</button>
				</div>
			{/each}
		</div>
	{/if}
{/if}
