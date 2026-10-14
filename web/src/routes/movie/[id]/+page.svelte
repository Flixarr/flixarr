<script lang="ts">
	import { page } from '$app/state';
	import { meta, fmtDate, runtime } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { Movie } from '#lib/types.ts';
	import DetailHeader from '#lib/components/DetailHeader.svelte';
	import CastRow from '#lib/components/CastRow.svelte';
	import Facts from '#lib/components/Facts.svelte';
	import Row from '#lib/components/Row.svelte';
	import Poster from '#lib/components/Poster.svelte';
	import Trailer from '#lib/components/Trailer.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import RequestButton from '#lib/components/RequestButton.svelte';
	import IssueModal from '#lib/components/IssueModal.svelte';
	import AdminPanel from '#lib/components/AdminPanel.svelte';
	import LeavingActions from '#lib/components/LeavingActions.svelte';
	import { can } from '#lib/session.svelte.ts';
	import type { RequestBrief } from '#lib/types.ts';
	import Img from '#lib/components/Img.svelte';

	let m = $state<Movie | null>(null);
	let error = $state('');
	let trailer = $state(false);
	const money = (n: number) => (n ? '$' + n.toLocaleString('en-US') : '');

	$effect(() => {
		const id = page.params.id;
		m = null;
		error = '';
		let alive = true;
		const show = (d: Movie) => { if (!alive) return; m = d; document.title = 'Flixarr · ' + d.title; };
		meta<Movie>('movie/' + id, {}, show).then(show).catch((e) => (error = e.message));
		return () => (alive = false);
	});
	let brief = $state<RequestBrief | null | undefined>(undefined);
	// Report issue: on what's in the library, or on a request that hasn't arrived ("Didn't download").
	const inLib = $derived(!!brief?.plex || brief?.status === 'available');
	const waiting = $derived(!!brief && ['approved', 'sent', 'grabbed', 'downloading', 'importing', 'failed'].includes(brief.status));
	let issueOpen = $state(false);
	let adminOpen = $state(false);
</script>

{#if error}
	<div class="px-4 pt-28 text-center text-gray-400">{error}</div>
{:else if m}
	<DetailHeader item={m}>
		{#snippet chip()}
			{#if brief?.plex}<span class="rounded-md bg-green-500/15 px-1.5 py-0.5 text-green-400 ring-1 ring-green-500/40">✓ In your library</span>
			{:else if brief !== undefined}<span class="rounded-md bg-gray-900/70 px-1.5 py-0.5 text-gray-300 ring-1 ring-gray-700/60">Not in your library</span>{/if}
		{/snippet}
		{#snippet actions()}
			{#if m}<RequestButton item={m} bind:req={brief} />{/if}
			{#if m && (inLib || waiting) && can('reportIssues')}<button class="btn-ghost act" onclick={() => (issueOpen = true)}><Icon name="issues" class="size-4" />Report issue</button>{/if}
			{#if m && can('manageRequests')}<button class="btn-ghost act" onclick={() => (adminOpen = true)}><Icon name="settings" class="size-4" />Admin</button>{/if}
			{#if m?.trailer}<button class="btn-ghost act" onclick={() => (trailer = true)}><Icon name="play" class="size-4" />Trailer</button>{/if}
			{#if m && brief?.plex}<LeavingActions type="movie" id={m.id} title={m.title} />{/if}
		{/snippet}
	</DetailHeader>
	<IssueModal bind:open={issueOpen} item={m} inLibrary={inLib} {waiting} />
	{#if m && can('manageRequests')}<AdminPanel bind:open={adminOpen} item={m} />{/if}

	<main class="relative z-10 mx-auto max-w-[1600px] space-y-9 pt-9 laptop:px-10">
		<div class="grid gap-9 laptop:grid-cols-[1fr_340px]">
			<section class="space-y-3 px-4 laptop:px-0">
				<h2 class="text-base font-semibold">Overview</h2>
				{#if m.tagline}<p class="italic text-gray-400">“{m.tagline}”</p>{/if}
				<p class="max-w-3xl leading-relaxed text-gray-300">{m.overview || 'No overview available.'}</p>
				<div class="grid max-w-3xl grid-cols-2 gap-4 pt-2 tablet:grid-cols-3">
					{#each [['Director', m.director], ['Writers', m.writers], ['Producers', m.producers]] as [k, v] (k)}
						{#if v.length}<div><div class="text-xs text-gray-500">{k}</div><div class="text-sm">{(v as string[]).join(', ')}</div></div>{/if}
					{/each}
				</div>
			</section>
			<aside class="px-4 laptop:px-0">
				<Facts keywords={m.keywords} facts={[['Status', m.status], ['Release date', fmtDate(m.release, { month: 'long', day: 'numeric', year: 'numeric' })], ['Runtime', runtime(m.runtime)], ['Rating', m.cert], ['Budget', money(m.budget)], ['Revenue', money(m.revenue)], ['Language', m.language], ['Studios', m.studios.join(', ')], ['Countries', m.countries.join(', ')]]} />
			</aside>
		</div>
		{#if m.collection}
			<a href="/collection/{m.collection.id}" class="relative mx-4 block overflow-hidden rounded-xl border border-gray-700/40 shadow-xl laptop:mx-0">
				{#if m.collection.backdrop}<Img path={m.collection.backdrop} kind="backdrop" sizes="(min-width: 1024px) 1000px, 100vw" class="absolute inset-0 h-full w-full opacity-40" />{/if}
				<div class="absolute inset-0 bg-linear-to-r from-gray-950/95 via-gray-950/70 to-gray-950/30"></div>
				<div class="relative flex items-center justify-between gap-4 p-5">
					<div><div class="text-xs text-gray-400">Part of a collection</div><div class="text-lg font-semibold">{m.collection.name}</div></div>
					<span class="btn-ghost shrink-0 px-4 text-sm">View collection <Icon name="chevron" class="size-4" /></span>
				</div>
			</a>
		{/if}
		<CastRow cast={m.cast} />
		<Row title="Recommended" items={m.recommendations}>{#snippet item(c)}<Poster item={c} />{/snippet}</Row>
		<Row title="Similar" items={m.similar}>{#snippet item(c)}<Poster item={c} />{/snippet}</Row>
	</main>
	{#if m.trailer}<Trailer key={m.trailer} bind:open={trailer} />{/if}
{:else}
	<div class="h-[52dvh] animate-pulse bg-gray-900/40"></div>
{/if}
