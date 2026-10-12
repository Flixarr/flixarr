<script lang="ts">
	// Settings → Discover: the order of the Discover rows (drag the handle, mouse or touch) and which show.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { ROW_INFO, type DiscoverRow } from '#lib/lists.ts';
	import Toggle from '#lib/components/Toggle.svelte';

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

	function toggle(key: string, on: boolean) {
		rows = rows!.map((r) => (r.key === key ? { ...r, on } : r));
		save(on ? `${ROW_INFO[key].title} is back on Discover.` : `${ROW_INFO[key].title} is hidden.`);
	}
	function reset() {
		rows = Object.keys(ROW_INFO).map((key) => ({ key, on: true }));
		save('Discover is back to the default rows.');
	}
	const isDefault = $derived(!!rows && rows.every((r, i) => r.on && r.key === Object.keys(ROW_INFO)[i]));
</script>

{#if rows}
	<div>
		<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Rows</div>
		<div bind:this={list} class="card divide-y divide-gray-700/40 px-2">
			{#each rows as r (r.key)}
				<div data-key={r.key} class="flex items-center gap-2 py-2.5 transition {dragging === r.key ? 'relative z-10 rounded-lg bg-gray-700/40 shadow-xl' : ''}">
					<button class="grid h-9 w-7 shrink-0 cursor-grab touch-none place-items-center rounded text-gray-500 hover:text-white active:cursor-grabbing" aria-label="Move {ROW_INFO[r.key]?.title}" onpointerdown={(e) => grab(e, r.key)}>
						<svg viewBox="0 0 24 24" class="size-4" fill="currentColor"><circle cx="9" cy="6" r="1.6" /><circle cx="15" cy="6" r="1.6" /><circle cx="9" cy="12" r="1.6" /><circle cx="15" cy="12" r="1.6" /><circle cx="9" cy="18" r="1.6" /><circle cx="15" cy="18" r="1.6" /></svg>
					</button>
					<div class="min-w-0 flex-1 pr-2">
						<Toggle checked={r.on} label={ROW_INFO[r.key]?.title ?? r.key} about={ROW_INFO[r.key]?.about} onchange={(on) => toggle(r.key, on)} />
					</div>
				</div>
			{/each}
		</div>
		<div class="mt-2 flex items-center justify-between gap-4">
			<p class="text-xs text-gray-500">Drag the handle to reorder. Changes apply to everyone's Discover page.</p>
			{#if !isDefault}<button class="shrink-0 text-xs text-gray-400 hover:text-white" onclick={reset}>Reset to default</button>{/if}
		</div>
	</div>
{/if}
