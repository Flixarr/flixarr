<script lang="ts">
	// Settings → Discover: the order of the Discover rows (drag the handle, mouse or touch), which show,
	// and custom rows (movies or series, a genre, an order).
	import { api, meta } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { ROW_INFO, SORTS, isCustom, rowAbout, rowTitle, type DiscoverRow } from '#lib/lists.ts';
	import Toggle from '#lib/components/Toggle.svelte';
	import Select from '#lib/components/Select.svelte';
	import Icon from '#lib/components/Icon.svelte';

	const OFF = ['now-playing', 'airing-today', 'top-rated-movies', 'top-rated-series']; // built-in rows that start off
	const DEFAULTS = Object.keys(ROW_INFO).map((key) => ({ key, on: !OFF.includes(key) }));

	let rows = $state<DiscoverRow[] | null>(null);
	let dragging = $state<string | null>(null);
	let list = $state<HTMLDivElement>();

	$effect(() => {
		api<DiscoverRow[]>('/discover/rows').then((r) => (rows = r));
	});

	async function save(msg: string) {
		try {
			rows = await api<DiscoverRow[]>('/discover/rows', { method: 'PUT', body: JSON.stringify(rows) });
			notify(msg);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}

	// Drag by the handle: the row follows the pointer, swapping places as it passes another row's middle.
	function grab(e: PointerEvent, key: string) {
		e.preventDefault();
		dragging = key;
		const before = JSON.stringify(rows);
		const move = (ev: PointerEvent) => {
			const els = [...list!.querySelectorAll<HTMLElement>('[data-key]')];
			const from = rows!.findIndex((r) => r.key === key);
			let to = els.findIndex((el) => ev.clientY < el.getBoundingClientRect().top + el.offsetHeight / 2);
			if (to === -1) to = els.length - 1;
			else if (to > from) to -= 1;
			if (to !== from) {
				const next = [...rows!];
				next.splice(to, 0, next.splice(from, 1)[0]);
				rows = next;
			}
		};
		const up = () => {
			removeEventListener('pointermove', move);
			removeEventListener('pointerup', up);
			removeEventListener('pointercancel', up);
			dragging = null;
			if (JSON.stringify(rows) !== before) save('Row order saved.');
		};
		addEventListener('pointermove', move);
		addEventListener('pointerup', up);
		addEventListener('pointercancel', up);
	}

	function toggle(r: DiscoverRow, on: boolean) {
		rows = rows!.map((x) => (x.key === r.key ? { ...x, on } : x));
		save(on ? `${rowTitle(r)} is on Discover.` : `${rowTitle(r)} is hidden.`);
	}
	// Back to the built-in rows in their order and on/off; custom rows stay, at the end.
	function reset() {
		rows = [...DEFAULTS, ...rows!.filter(isCustom)];
		save('Discover is back to the default rows.');
	}
	const isDefault = $derived(!!rows && DEFAULTS.every((d, i) => rows![i]?.key === d.key && rows![i].on === d.on));

	// A new custom row.
	let adding = $state(false);
	let draft = $state<{ title: string; kind: 'movie' | 'series'; genre: number; sort: 'popular' | 'rating' | 'newest' }>({ title: '', kind: 'movie', genre: 0, sort: 'popular' });
	let genres = $state<Record<string, { id: number; name: string }[]>>({});
	$effect(() => {
		const k = draft.kind;
		if (adding && !genres[k]) meta<{ genres: { id: number; name: string }[] }>('genres/' + k).then((d) => (genres = { ...genres, [k]: d.genres })).catch(() => {});
	});
	const genreOptions = $derived([{ value: 0, label: 'Any genre' }, ...(genres[draft.kind] ?? []).map((g) => ({ value: g.id, label: g.name }))]);
	function add() {
		const genreName = genres[draft.kind]?.find((g) => g.id === draft.genre)?.name ?? '';
		rows = [...rows!, { key: 'custom-new', on: true, title: draft.title.trim(), kind: draft.kind, genre: draft.genre || undefined, genreName: genreName || undefined, sort: draft.sort }];
		save(`${draft.title.trim()} is on Discover.`);
		adding = false;
		draft = { title: '', kind: 'movie', genre: 0, sort: 'popular' };
	}
	function remove(r: DiscoverRow) {
		rows = rows!.filter((x) => x.key !== r.key);
		save(`${rowTitle(r)} is removed.`);
	}
</script>

{#if rows}
	<div>
		<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Rows</div>
		<div bind:this={list} class="card divide-y divide-gray-700/40 px-2">
			{#each rows as r (r.key)}
				<div data-key={r.key} class="flex items-center gap-2 py-2.5 transition {dragging === r.key ? 'relative z-10 rounded-lg bg-gray-700/40 shadow-xl' : ''}">
					<button class="grid h-9 w-7 shrink-0 cursor-grab touch-none place-items-center rounded text-gray-500 hover:text-white active:cursor-grabbing" aria-label="Move {rowTitle(r)}" onpointerdown={(e) => grab(e, r.key)}>
						<svg viewBox="0 0 24 24" class="size-4" fill="currentColor"><circle cx="9" cy="6" r="1.6" /><circle cx="15" cy="6" r="1.6" /><circle cx="9" cy="12" r="1.6" /><circle cx="15" cy="12" r="1.6" /><circle cx="9" cy="18" r="1.6" /><circle cx="15" cy="18" r="1.6" /></svg>
					</button>
					<div class="min-w-0 flex-1 pr-2">
						<Toggle checked={r.on} label={rowTitle(r)} about={rowAbout(r)} onchange={(on) => toggle(r, on)} />
					</div>
					{#if isCustom(r)}<button class="grid size-8 shrink-0 place-items-center rounded text-gray-500 hover:text-red-300" aria-label="Remove {rowTitle(r)}" onclick={() => remove(r)}><Icon name="close" class="size-3.5" /></button>{/if}
				</div>
			{/each}
		</div>
		<div class="mt-2 flex items-center justify-between gap-4">
			<p class="text-xs text-gray-500">Drag the handle to reorder. Changes apply to everyone's Discover page.</p>
			{#if !isDefault}<button class="shrink-0 text-xs text-gray-400 hover:text-white" onclick={reset}>Reset to default</button>{/if}
		</div>
	</div>
	<div>
		<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Custom rows</div>
		{#if adding}
			<form class="card space-y-3 p-4 text-sm" onsubmit={(e) => (e.preventDefault(), add())}>
				<div><label for="cr-title" class="block text-sm">Name</label><input id="cr-title" maxlength="60" class="mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60" placeholder="Horror Movies" bind:value={draft.title} /></div>
				<div class="grid gap-3 tablet:grid-cols-3">
					<div><span class="mb-1 block text-sm">Shows</span><Select value={draft.kind} options={[{ value: 'movie', label: 'Movies' }, { value: 'series', label: 'Series' }]} onchange={(v) => (draft = { ...draft, kind: v as 'movie' | 'series', genre: 0 })} /></div>
					<div><span class="mb-1 block text-sm">Genre</span><Select value={draft.genre} options={genreOptions} onchange={(v) => (draft.genre = Number(v))} /></div>
					<div><span class="mb-1 block text-sm">Order</span><Select value={draft.sort} options={Object.entries(SORTS).map(([value, label]) => ({ value, label }))} onchange={(v) => (draft.sort = v as 'popular' | 'rating' | 'newest')} /></div>
				</div>
				<div class="flex justify-end gap-2"><button type="button" class="btn-ghost px-4 py-1.5 text-sm" onclick={() => (adding = false)}>Cancel</button><button class="btn px-5 py-1.5 text-sm" disabled={!draft.title.trim()}>Add row</button></div>
			</form>
		{:else}
			<button class="flex w-full items-center justify-center gap-2 rounded-xl border border-dashed border-gray-700 p-4 text-sm text-gray-400 transition hover:border-gray-500 hover:text-white" onclick={() => (adding = true)}><Icon name="plus" class="size-4" />Add a custom row</button>
		{/if}
		<p class="mt-2 text-xs text-gray-500">A row of movies or series, from one genre or all, in the order you pick. It shows on Discover where you drag it.</p>
	</div>
{/if}
