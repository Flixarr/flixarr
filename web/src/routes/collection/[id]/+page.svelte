<script lang="ts">
	import { page } from '$app/state';
	import { api, meta } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { Collection } from '#lib/types.ts';
	import Poster from '#lib/components/Poster.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import Img, { BACKDROP_SIZES } from '#lib/components/Img.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import Select from '#lib/components/Select.svelte';
	import { statusOf, remember } from '#lib/availability.svelte.ts';
	import { can } from '#lib/session.svelte.ts';
	import type { Card, Request } from '#lib/types.ts';

	let c = $state<Collection | null>(null);
	let error = $state('');
	let open = $state(false);
	let picked = $state<number[]>([]);
	let sending = $state(false);
	type Options = { connected: boolean; server: string; approval: boolean; defaultProfileId?: number; profiles: { id: number; name: string }[]; usage: { limit: number; used: number } };
	let opts = $state<Options | null>(null);
	let profileId = $state(0);

	// Where each movie stands: in Plex, requested, or missing.
	const st = (p: Card) => statusOf(`movie:${p.id}`);
	const have = $derived(c ? c.parts.filter((p) => st(p)?.plex || st(p)?.status === 'available').length : 0);
	const missing = $derived(c ? c.parts.filter((p) => { const s = st(p); return s !== undefined && (!s || s.status === 'failed' || s.status === 'declined'); }) : []);

	async function openRequest() {
		picked = missing.filter((p) => p.year && p.year <= String(new Date().getFullYear())).map((p) => p.id);
		opts = null;
		open = true;
		try {
			opts = await api<Options>('/requests/options?type=movie');
			profileId = opts.defaultProfileId ?? 0;
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not load.', 'orange');
		}
	}
	async function send() {
		sending = true;
		let ok = 0, failed = 0, firstErr = '';
		for (const id of picked) {
			const p = c!.parts.find((x) => x.id === id)!;
			try {
				const r = await api<Request>('/requests', { method: 'POST', body: JSON.stringify({ type: 'movie', tmdbId: p.id, title: p.title, year: p.year, poster: p.poster ?? '', backdrop: p.backdrop ?? '', profileId, genres: [], language: '' }) });
				remember(`movie:${p.id}`, { id: r.id, status: r.status, progress: r.progress });
				if (r.status === 'failed') { failed++; firstErr ||= r.error; } else ok++;
			} catch (e) {
				failed++;
				firstErr ||= e instanceof Error ? e.message : 'Could not request it.';
			}
		}
		sending = false;
		open = false;
		if (failed) notify(`Requested ${ok} of ${ok + failed}. ${firstErr}`, 'orange');
		else notify(`Requested ${ok} ${ok === 1 ? 'movie' : 'movies'}.${opts?.approval ? ' They wait for an admin to approve them.' : ''}`);
	}
	$effect(() => {
		const id = page.params.id;
		c = null;
		let alive = true;
		const show = (d: Collection) => { if (!alive) return; c = d; document.title = 'Flixarr · ' + d.name; };
		meta<Collection>('collection/' + id, {}, show).then(show).catch((e) => (error = e.message));
		return () => (alive = false);
	});
</script>

{#if error}<p class="px-4 pt-28 text-center text-gray-400">{error}</p>
{:else if c}
	<section class="relative">
		<div class="absolute inset-x-0 top-0 h-[52dvh] min-h-[340px] [mask-image:linear-gradient(to_bottom,black_30%,transparent_100%)] laptop:h-[64dvh]">
			{#if c.backdrop}<Img path={c.backdrop} kind="backdrop" sizes={BACKDROP_SIZES} eager class="absolute inset-0 h-full w-full opacity-50 saturate-75" />{/if}
			<div class="absolute inset-0 bg-linear-to-r from-gray-950/90 via-gray-950/40 to-transparent"></div>
		</div>
		<div class="relative mx-auto flex max-w-[1600px] flex-row items-end gap-4 px-4 pt-[24dvh] tablet:gap-5 laptop:px-10 laptop:pt-[28dvh]">
			{#if c.poster}<Img path={c.poster} kind="poster" sizes="(min-width: 640px) 176px, 96px" eager class="aspect-[2/3] w-24 shrink-0 self-start rounded-xl border border-gray-700/40 bg-gray-800/40 shadow-2xl tablet:w-44 tablet:self-end" />{/if}
			<div class="min-w-0 space-y-3">
				<span class="rounded-md bg-primary/90 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide">Collection</span>
				<h1 class="text-2xl font-bold tablet:text-3xl laptop:text-4xl">{c.name}</h1>
				<div class="max-w-xs space-y-1.5">
					<div class="text-sm text-gray-300">{c.parts.length} movies · {have} in your library</div>
					<div class="h-1.5 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full bg-green-500 transition-all" style="width:{c.parts.length ? (have / c.parts.length) * 100 : 0}%"></div></div>
				</div>
				{#if missing.length && can('requestMovies')}
					<button class="btn px-5" onclick={openRequest}><Icon name="plus" class="size-4" />Request {missing.length} missing</button>
				{:else if c.parts.length && have === c.parts.length}
					<span class="inline-flex items-center gap-2 rounded-lg bg-green-500/15 px-4 py-2 text-sm font-medium text-green-300 ring-1 ring-green-500/40"><Icon name="check" class="size-4" />All in your library</span>
				{/if}
			</div>
		</div>
	</section>
	<main class="relative z-10 mx-auto max-w-[1600px] space-y-9 pt-9 laptop:px-10">
		{#if c.overview}<section class="space-y-3 px-4 laptop:px-0"><h2 class="text-base font-semibold">Overview</h2><p class="max-w-3xl leading-relaxed text-gray-300">{c.overview}</p></section>{/if}
		<section class="space-y-3">
			<h2 class="px-4 text-base font-semibold laptop:px-0">Movies</h2>
			<div class="grid grid-cols-3 gap-3 px-4 tablet:grid-cols-4 laptop:grid-cols-6 laptop:px-0 desktop:grid-cols-8">
				{#each c.parts as p (p.id)}<Poster item={p} class="w-full" />{/each}
			</div>
		</section>
	</main>
{/if}

<Modal bind:open title="Request {c?.name ?? ''}" sub="{missing.length} not in your library">
	<div class="space-y-4 p-5">
		{#if !opts}
			<p class="text-sm text-gray-400">Loading…</p>
		{:else if !opts.connected}
			<p class="rounded-lg bg-yellow-500/10 px-3 py-2 text-sm text-yellow-300 ring-1 ring-yellow-500/30">{opts.server} isn't connected yet. An admin can connect it in Settings → Services.</p>
		{:else}
			<p class="text-xs text-gray-400">Goes to <span class="text-gray-200">{opts.server}</span>{opts.approval ? ' after an admin approves it' : ''}.{#if opts.usage?.limit}{@const left = Math.max(0, opts.usage.limit - opts.usage.used)} You have {left} movie{left === 1 ? '' : 's'} left this week.{/if}</p>
			{#if opts.profiles.length}
				<div>
					<label for="col-quality" class="block text-sm">Quality</label>
					<Select id="col-quality" bind:value={profileId} options={opts.profiles.map((p) => ({ value: p.id, label: p.name, hint: p.id === opts!.defaultProfileId ? 'default' : '' }))} />
				</div>
			{/if}
			<div class="max-h-[45dvh] divide-y divide-gray-700/40 overflow-y-auto rounded-lg border border-gray-700/50 bg-gray-900/40">
				{#each missing as p (p.id)}
					<label class="flex cursor-pointer items-center gap-3 px-3 py-2 text-sm hover:bg-white/5">
						<input type="checkbox" class="accent-[var(--color-primary)]" checked={picked.includes(p.id)} onchange={(e) => (picked = (e.currentTarget as HTMLInputElement).checked ? [...picked, p.id] : picked.filter((x) => x !== p.id))} />
						<span class="min-w-0 flex-1 truncate">{p.title}</span><span class="text-xs text-gray-500">{p.year || 'Not out yet'}</span>
					</label>
				{/each}
			</div>
		{/if}
	</div>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (open = false)}>Cancel</button>
		<button class="btn px-6" disabled={!opts?.connected || !picked.length || sending} onclick={send}>{sending ? 'Sending…' : `Request ${picked.length}`}</button>
	{/snippet}
</Modal>
