<script lang="ts">
	import { page } from '$app/state';
	import { meta, fmtDate } from '#lib/api.ts';
	import type { Person, PersonCredit, Role } from '#lib/types.ts';
	import Poster from '#lib/components/Poster.svelte';
	import Chips from '#lib/components/Chips.svelte';
	import Img from '#lib/components/Img.svelte';
	import { infinite } from '#lib/infinite.ts';

	let p = $state<Person | null>(null);
	let error = $state('');
	let more = $state(false);
	let clamped = $state(false);
	let bioEl = $state<HTMLParagraphElement>();
	let role = $state<string | number>('all');
	let type = $state<string | number>('all');
	const age = (b: string, d?: string | null) => Math.floor(((d ? new Date(d) : new Date()).getTime() - new Date(b).getTime()) / 3.15576e10);

	// Role tabs and type tabs filter together (#237); each tab counts what it would show with the other one applied.
	const ROLES: [Role, string][] = [['acting', 'Acting'], ['directing', 'Directing'], ['writing', 'Writing'], ['producing', 'Producing']];
	const rolesOf = (c: PersonCredit): Role[] => c.roles ?? ['acting'];
	const inRole = (c: PersonCredit, r: string | number) => r === 'all' || rolesOf(c).includes(r as Role);
	const inType = (c: PersonCredit, t: string | number) => t === 'all' || c.type === t;
	const credits = $derived(p?.credits ?? []);
	const roleTabs = $derived.by(() => {
		const tabs = ROLES.map(([key, label]) => ({ key, label, count: credits.filter((c) => inType(c, type) && rolesOf(c).includes(key)).length }))
			.filter((t) => credits.some((c) => rolesOf(c).includes(t.key)));
		return tabs.length > 1 ? [{ key: 'all', label: 'All', count: credits.filter((c) => inType(c, type)).length }, ...tabs] : [];
	});
	const typeTabs = $derived.by(() => {
		const r = credits.filter((c) => inRole(c, role));
		const movies = r.filter((c) => c.type === 'movie').length, series = r.length - movies;
		return movies && series ? [{ key: 'all', label: 'All', count: r.length }, { key: 'movie', label: 'Movies', count: movies }, { key: 'series', label: 'Series', count: series }] : [];
	});
	// A type picked under another role still filters only while its tabs show (e.g. Series, then Directing with no series).
	const shownType = $derived(typeTabs.length ? type : 'all');
	const list = $derived(credits.filter((c) => inRole(c, role) && inType(c, shownType)));
	// Under the poster: the character for acting, the jobs for crew. On All, crew jobs win when there are any.
	const CREW: Record<string, string[]> = { directing: ['Director', 'Co-Director'], writing: ['Writer'], producing: ['Producer', 'Executive Producer'] };
	const sub = (c: PersonCredit) => {
		const jobs = role === 'all' ? (c.jobs ?? []) : (c.jobs ?? []).filter((j) => CREW[role]?.includes(j));
		if (role !== 'acting' && jobs.length) return jobs.join(', ');
		return c.character ? 'as ' + c.character : '';
	};

	// Posters are drawn 48 at a time, more as you scroll (#236); a new tab starts over.
	const STEP = 48;
	let drawn = $state(STEP);
	$effect(() => {
		void [role, shownType];
		drawn = STEP;
	});
	const visible = $derived(list.slice(0, drawn));
	const next = async () => {
		if (drawn >= list.length) return false;
		drawn += STEP;
		return true;
	};

	$effect(() => {
		const id = page.params.id;
		p = null;
		error = '';
		more = false;
		role = 'all';
		type = 'all';
		let alive = true;
		const show = (d: Person) => { if (!alive) return; p = d; document.title = 'Flixarr · ' + d.name; };
		meta<Person>('person/' + id, {}, show).then(show).catch((e) => { if (alive) error = e.message; });
		return () => (alive = false);
	});
	// Read more only when the bio is actually cut off (#238).
	$effect(() => {
		void p?.bio;
		if (!bioEl || more) return;
		clamped = bioEl.scrollHeight > bioEl.clientHeight + 1;
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
					<p bind:this={bioEl} class="max-w-3xl whitespace-pre-line leading-relaxed text-gray-300 {more ? '' : 'line-clamp-5'}">{p.bio}</p>
					{#if clamped}<button class="text-sm text-primary-light hover:text-white" onclick={() => (more = !more)}>{more ? 'Show less' : 'Read more'}</button>{/if}
				{/if}
			</div>
		</section>
		<section class="space-y-3">
			<div class="flex flex-wrap items-center justify-between gap-3 px-4 laptop:px-0"><h2 class="text-base font-semibold">Known for</h2></div>
			{#if roleTabs.length}<Chips bind:value={role} options={roleTabs} />{/if}
			{#if typeTabs.length}<Chips bind:value={type} options={typeTabs} />{/if}
			<div class="grid grid-cols-3 gap-3 px-4 tablet:grid-cols-4 laptop:grid-cols-6 laptop:px-0 desktop:grid-cols-8">
				{#each visible as c (c.type + c.id)}<Poster item={c} class="w-full" sub={sub(c)} />{/each}
			</div>
			{#if drawn < list.length}<div use:infinite={next} class="py-6 text-center text-xs text-gray-500">Loading more…</div>{/if}
		</section>
	{/if}
</main>
