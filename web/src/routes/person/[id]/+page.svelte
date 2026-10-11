<script lang="ts">
	import { page } from '$app/state';
	import { meta, fmtDate } from '#lib/api.ts';
	import type { Person } from '#lib/types.ts';
	import Poster from '#lib/components/Poster.svelte';
	import Chips from '#lib/components/Chips.svelte';
	import Img from '#lib/components/Img.svelte';

	let p = $state<Person | null>(null);
	let error = $state('');
	let more = $state(false);
	let tab = $state<string | number>('all');
	const age = (b: string, d?: string | null) => Math.floor(((d ? new Date(d) : new Date()).getTime() - new Date(b).getTime()) / 3.15576e10);
	const list = $derived((p?.credits ?? []).filter((c) => tab === 'all' || c.type === tab));

	$effect(() => {
		const id = page.params.id;
		p = null;
		let alive = true;
		const show = (d: Person) => { if (!alive) return; p = d; document.title = 'Flixarr · ' + d.name; };
		meta<Person>('person/' + id, {}, show).then(show).catch((e) => (error = e.message));
		return () => (alive = false);
	});
</script>

<main class="mx-auto max-w-[1600px] space-y-9 pt-20 laptop:px-10 laptop:pt-24">
	{#if error}<p class="px-4 text-center text-gray-400">{error}</p>
	{:else if p}
		<section class="flex flex-col gap-6 px-4 tablet:flex-row laptop:px-0">
			<div class="mx-auto w-40 shrink-0 tablet:mx-0 tablet:w-52">
				<div class="aspect-[2/3] overflow-hidden rounded-xl border border-gray-700/40 bg-gray-800/40 shadow-2xl">
					{#if p.profile}<Img path={p.profile} kind="profile" sizes="(min-width: 640px) 208px, 160px" eager class="h-full w-full" />{:else}<div class="grid h-full place-items-center text-5xl text-gray-600">{p.name.charAt(0)}</div>{/if}
				</div>
			</div>
			<div class="min-w-0 space-y-3">
				<div class="text-xs text-gray-400">{p.dept ?? ''}</div>
				<h1 class="text-2xl font-bold laptop:text-3xl">{p.name}</h1>
				<div class="flex flex-wrap gap-x-4 gap-y-1 text-sm text-gray-300">
					{#if p.born}<span>Born {fmtDate(p.born, { month: 'long', day: 'numeric', year: 'numeric' })}{p.died ? '' : ` (age ${age(p.born)})`}</span>{/if}
					{#if p.died && p.born}<span>Died {fmtDate(p.died, { month: 'long', day: 'numeric', year: 'numeric' })} (age {age(p.born, p.died)})</span>{/if}
					{#if p.place}<span class="text-gray-400">{p.place}</span>{/if}
				</div>
				{#if p.bio}
					<p class="max-w-3xl whitespace-pre-line leading-relaxed text-gray-300 {more ? '' : 'line-clamp-5'}">{p.bio}</p>
					<button class="text-sm text-primary-light hover:text-white" onclick={() => (more = !more)}>{more ? 'Show less' : 'Read more'}</button>
				{/if}
			</div>
		</section>
		<section class="space-y-3">
			<div class="flex flex-wrap items-center justify-between gap-3 px-4 laptop:px-0"><h2 class="text-base font-semibold">Known for</h2></div>
			<Chips bind:value={tab} options={[{ key: 'all', label: 'All', count: p.credits.length }, { key: 'movie', label: 'Movies', count: p.credits.filter((c) => c.type === 'movie').length }, { key: 'series', label: 'Series', count: p.credits.filter((c) => c.type === 'series').length }]} />
			<div class="grid grid-cols-3 gap-3 px-4 tablet:grid-cols-4 laptop:grid-cols-6 laptop:px-0 desktop:grid-cols-8">
				{#each list as c (c.type + c.id)}<Poster item={c} class="w-full" sub={c.character ? 'as ' + c.character : ''} />{/each}
			</div>
		</section>
	{/if}
</main>
