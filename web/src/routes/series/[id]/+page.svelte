<script lang="ts">
	import { page } from '$app/state';
	import { meta, img, fmtDate } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { Series, Season } from '#lib/types.ts';
	import DetailHeader from '#lib/components/DetailHeader.svelte';
	import CastRow from '#lib/components/CastRow.svelte';
	import Facts from '#lib/components/Facts.svelte';
	import Row from '#lib/components/Row.svelte';
	import Poster from '#lib/components/Poster.svelte';
	import Trailer from '#lib/components/Trailer.svelte';
	import Icon from '#lib/components/Icon.svelte';

	let t = $state<Series | null>(null);
	let error = $state('');
	let trailer = $state(false);
	let episodes = $state<Record<number, Season | 'loading' | 'error'>>({});

	$effect(() => {
		const id = page.params.id;
		t = null;
		error = '';
		episodes = {};
		let alive = true;
		const show = (d: Series) => { if (!alive) return; t = d; document.title = 'Flixarr · ' + d.title; };
		meta<Series>('series/' + id, {}, show).then(show).catch((e) => (error = e.message));
		return () => (alive = false);
	});

	function openSeason(n: number, isOpen: boolean) {
		if (!isOpen || !t || episodes[n]) return;
		episodes[n] = 'loading';
		const show = (s: Season) => (episodes[n] = s);
		meta<Season>(`series/${t.id}/season/${n}`, {}, show).then(show).catch(() => (episodes[n] = 'error'));
	}
</script>

{#if error}
	<div class="px-4 pt-28 text-center text-gray-400">{error}</div>
{:else if t}
	<DetailHeader item={t}>
		{#snippet chip()}<span class="rounded-md bg-gray-900/70 px-1.5 py-0.5 text-gray-300 ring-1 ring-gray-700/60">Not in your library</span>{/snippet}
		{#snippet actions()}
			<button class="btn px-5" onclick={() => notify('Requests arrive in the next Flixarr update.', 'orange')}><Icon name="plus" class="size-4" />Request</button>
			{#if t?.trailer}<button class="btn-ghost px-4" onclick={() => (trailer = true)}><Icon name="play" class="size-4" />Trailer</button>{/if}
		{/snippet}
	</DetailHeader>

	<main class="relative z-10 mx-auto max-w-[1600px] space-y-9 pt-9 laptop:px-10">
		<div class="grid gap-9 laptop:grid-cols-[1fr_340px]">
			<div class="space-y-9">
				<section class="space-y-3 px-4 laptop:px-0">
					<h2 class="text-base font-semibold">Overview</h2>
					{#if t.tagline}<p class="italic text-gray-400">“{t.tagline}”</p>{/if}
					<p class="max-w-3xl leading-relaxed text-gray-300">{t.overview || 'No overview available.'}</p>
					<div class="grid max-w-3xl grid-cols-2 gap-4 pt-2 tablet:grid-cols-3">
						{#if t.creators.length}<div><div class="text-xs text-gray-500">Created by</div><div class="text-sm">{t.creators.join(', ')}</div></div>{/if}
						{#if t.networks.length}<div><div class="text-xs text-gray-500">Network</div><div class="text-sm">{t.networks.join(', ')}</div></div>{/if}
					</div>
				</section>
				<section class="space-y-3 px-4 laptop:px-0">
					<div class="flex items-center justify-between"><h2 class="text-base font-semibold">Seasons</h2>
						<span class="text-xs text-gray-400">{t.next ? 'Next episode ' + fmtDate(t.next) : t.status === 'Ended' ? 'Series ended' : ''}</span></div>
					<div class="space-y-2">
						{#each t.seasons as s (s.n)}
							<details class="group card overflow-hidden" ontoggle={(e) => openSeason(s.n, (e.currentTarget as HTMLDetailsElement).open)}>
								<summary class="flex cursor-pointer list-none items-center gap-3 p-3 hover:bg-white/5">
									{#if s.poster}<img src={img(s.poster, 'w92')} alt="" class="w-10 rounded-md ring-1 ring-gray-700/60" />
									{:else}<div class="grid h-15 w-10 place-items-center rounded-md bg-gray-800 text-xs text-gray-500">S{s.n}</div>{/if}
									<div class="min-w-0 flex-1"><div class="truncate text-sm font-medium">{s.name}</div><div class="text-xs text-gray-400">{s.episodes} episodes{s.air ? ' · ' + s.air.slice(0, 4) : ''}</div></div>
									<span class="text-gray-500 transition group-open:rotate-90"><Icon name="chevron" class="size-4" /></span>
								</summary>
								<div class="border-t border-gray-700/40">
									{#if episodes[s.n] === 'loading' || !episodes[s.n]}
										<div class="p-3 text-xs text-gray-500">Loading episodes…</div>
									{:else if episodes[s.n] === 'error'}
										<div class="p-3 text-xs text-red-400">Couldn't load episodes.</div>
									{:else}
										{#each (episodes[s.n] as Season).episodes as e (e.n)}
											<div class="flex gap-3 border-b border-gray-700/30 p-3 last:border-0">
												<div class="aspect-video w-28 shrink-0 overflow-hidden rounded-md bg-gray-800 tablet:w-36">{#if e.still}<img src={img(e.still, 'w300')} alt="" loading="lazy" class="h-full w-full object-cover" />{/if}</div>
												<div class="min-w-0 flex-1">
													<div class="flex items-center gap-2 text-sm"><span class="text-xs text-gray-500">E{e.n}</span><span class="truncate font-medium">{e.name}</span></div>
													<div class="text-xs text-gray-500">{fmtDate(e.air)}{e.runtime ? ' · ' + e.runtime + 'm' : ''}</div>
													<p class="mt-1 line-clamp-2 text-xs text-gray-400">{e.overview}</p>
												</div>
											</div>
										{/each}
									{/if}
								</div>
							</details>
						{/each}
					</div>
				</section>
			</div>
			<aside class="px-4 laptop:px-0">
				<Facts keywords={t.keywords} facts={[['Status', t.status], ['First aired', fmtDate(t.first, { month: 'long', day: 'numeric', year: 'numeric' })], ['Last aired', fmtDate(t.last, { month: 'long', day: 'numeric', year: 'numeric' })], ['Seasons', t.seasons.length], ['Episodes', t.seasons.reduce((a, s) => a + s.episodes, 0)], ['Episode length', t.runtime ? t.runtime + 'm' : ''], ['Rating', t.cert], ['Networks', t.networks.join(', ')], ['Studios', t.studios.join(', ')]]} />
			</aside>
		</div>
		<CastRow cast={t.cast} />
		<Row title="Recommended" items={t.recommendations}>{#snippet item(c)}<Poster item={c} />{/snippet}</Row>
		<Row title="Similar" items={t.similar}>{#snippet item(c)}<Poster item={c} />{/snippet}</Row>
	</main>
	{#if t.trailer}<Trailer key={t.trailer} bind:open={trailer} />{/if}
{:else}
	<div class="h-[52dvh] animate-pulse bg-gray-900/40"></div>
{/if}
