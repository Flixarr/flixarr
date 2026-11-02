<script lang="ts">
	// Settings → Issues (#194): the What's wrong reasons people pick when they report an issue, and the details
	// under each. Rename, add, remove and reorder them, then Save; Put defaults back restores Flixarr's list.
	// Didn't download stays (Flixarr uses it for requests that never arrived); its details can change. Issues
	// already reported keep the reason they were filed with.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Icon from '#lib/components/Icon.svelte';
	import Tooltip from '#lib/components/Tooltip.svelte';

	type Kind = { kind: string; subs: string[] };
	type Info = { kinds: Kind[]; defaults: Kind[]; custom: boolean };
	const DIDNT = "Didn't download";
	let info = $state<Info | null>(null);
	let list = $state<Kind[]>([]);
	let saving = $state(false);
	const copy = (k: Kind[]) => k.map((x) => ({ kind: x.kind, subs: [...x.subs] }));
	function got(d: Info) {
		info = d;
		list = copy(d.kinds);
	}
	$effect(() => {
		api<Info>('/settings/issues').then(got).catch(() => {});
	});
	const dirty = $derived(!!info && JSON.stringify(list) !== JSON.stringify(info.kinds));

	function move(i: number, by: number) {
		const j = i + by;
		if (j < 0 || j >= list.length) return;
		[list[i], list[j]] = [list[j], list[i]];
	}
	async function save(body: object, msg: string) {
		saving = true;
		try {
			got(await api<Info>('/settings/issues', { method: 'PUT', body: JSON.stringify(body) }));
			notify(msg);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		} finally {
			saving = false;
		}
	}
	const input = 'w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-1.5 text-sm outline-none focus:ring-2 focus:ring-primary/60';
</script>

{#if info}
	<div>
		<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">What's wrong</div>
		<p class="mb-3 text-sm text-gray-400">The reasons people pick when they report an issue, in this order, with the details under each. A reason with no details asks for a note. Issues already reported keep the reason they were filed with.</p>
		<div class="space-y-2">
			{#each list as k, i (i)}
				{@const fixed = k.kind === DIDNT && info.kinds.some((x) => x.kind === DIDNT)}
				<div class="card space-y-3 p-3">
					<div class="flex items-center gap-2">
						<input class="{input} font-medium" aria-label="Reason name" maxlength="40" bind:value={k.kind} readonly={fixed} />
						<Tooltip text="Move up"><button class="btn-ghost px-2 py-1.5" aria-label="Move {k.kind} up" disabled={i === 0} onclick={() => move(i, -1)}><Icon name="chevron" class="size-4 -rotate-90" /></button></Tooltip>
						<Tooltip text="Move down"><button class="btn-ghost px-2 py-1.5" aria-label="Move {k.kind} down" disabled={i === list.length - 1} onclick={() => move(i, 1)}><Icon name="chevron" class="size-4 rotate-90" /></button></Tooltip>
						<Tooltip text={fixed ? 'Flixarr uses this one for requests that never arrived' : 'Remove'}><button class="btn-ghost px-2 py-1.5 hover:text-red-300" aria-label="Remove {k.kind}" disabled={fixed} onclick={() => list.splice(i, 1)}><Icon name="trash" class="size-4" /></button></Tooltip>
					</div>
					{#if fixed}<p class="-mt-1 text-xs text-gray-500">For requests that never arrived, and the only reason allowed on titles that aren't in Plex yet. Its details can change.</p>{/if}
					<div class="flex flex-wrap items-center gap-1.5">
						{#each k.subs as _, j (j)}
							<span class="flex items-center gap-1 rounded-full border border-gray-700/60 bg-gray-800/50 py-0.5 pl-3 pr-1">
								<input class="w-28 bg-transparent text-xs outline-none" aria-label="Detail under {k.kind}" maxlength="40" bind:value={k.subs[j]} />
								<button class="grid size-5 place-items-center rounded-full text-gray-500 hover:bg-white/10 hover:text-red-300" aria-label="Remove {k.subs[j]}" onclick={() => k.subs.splice(j, 1)}><Icon name="close" class="size-3" /></button>
							</span>
						{/each}
						<button class="rounded-full border border-dashed border-gray-600 px-3 py-1 text-xs text-gray-400 hover:text-white" onclick={() => k.subs.push('')}>+ Detail</button>
						{#if !k.subs.length}<span class="text-xs text-gray-500">No details: people write a note.</span>{/if}
					</div>
				</div>
			{/each}
		</div>
		<button class="btn-ghost mt-2 text-sm" disabled={list.length >= 20} onclick={() => list.push({ kind: '', subs: [] })}><Icon name="plus" class="size-4" />Add a reason</button>
		<div class="mt-4 flex flex-wrap items-center gap-2">
			{#if info.custom}<button class="mr-auto text-xs text-gray-400 hover:text-white" disabled={saving} onclick={() => save({ reset: true }, "Flixarr's reasons are back.")}>Put defaults back</button>{:else}<span class="mr-auto"></span>{/if}
			<button class="btn-ghost text-sm" disabled={!dirty || saving} onclick={() => (list = copy(info!.kinds))}>Undo</button>
			<button class="btn px-6 text-sm" disabled={!dirty || saving} onclick={() => save({ kinds: list }, 'Saved. The report window uses it now.')}>{saving ? 'Saving…' : 'Save'}</button>
		</div>
	</div>
{/if}
