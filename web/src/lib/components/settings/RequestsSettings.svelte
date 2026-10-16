<script lang="ts">
	// Settings → Requests: whether requests wait for approval, and how series can be requested (whole
	// show, picked seasons, picked episodes) with the one the request window starts on.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Toggle from '#lib/components/Toggle.svelte';

	type Modes = { whole: boolean; seasons: boolean; episodes: boolean; default: 'whole' | 'seasons' | 'episodes' };
	type Info = { approvalRequired: boolean; tvModes: Modes };
	let info = $state<Info | null>(null);
	$effect(() => {
		api<Info>('/settings/requests').then((d) => (info = d));
	});

	async function save(change: Partial<Info>, msg: string) {
		try {
			info = await api<Info>('/settings/requests', { method: 'PUT', body: JSON.stringify({ ...info, ...change }) });
			notify(msg);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}
	const MODES: [keyof Omit<Modes, 'default'>, string, string][] = [
		['whole', 'Whole show', 'Every season, and new ones as they air.'],
		['seasons', 'Pick seasons', 'Choose which seasons. Seasons that air later are not added.'],
		['episodes', 'Pick episodes', 'Choose single episodes, from any seasons.']
	];
	function setMode(k: keyof Omit<Modes, 'default'>, on: boolean) {
		const m = { ...info!.tvModes, [k]: on };
		if (!m.whole && !m.seasons && !m.episodes) {
			notify('Keep at least one way to request series.', 'orange');
			// The switch already flipped on screen: flip it back by re-drawing it.
			const keep = info!;
			info = null;
			setTimeout(() => (info = keep));
			return;
		}
		save({ tvModes: m }, on ? `${MODES.find(([x]) => x === k)?.[1]} is on.` : `${MODES.find(([x]) => x === k)?.[1]} is off.`);
	}
</script>

{#snippet heading(t: string)}<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">{t}</div>{/snippet}

{#if info}
	<div>
		{@render heading('Approval')}
		<div class="card px-4">
			<div class="py-3">
				<Toggle checked={info.approvalRequired} label="Require approval" about="Off: requests go straight to Radarr and Sonarr. On: they wait until someone who manages requests approves them. People with Auto-approve (admins and Trusted users) never wait." onchange={(on) => save({ approvalRequired: on }, on ? 'Requests now wait for approval.' : 'Requests are sent straight away.')} />
			</div>
		</div>
	</div>
	<div>
		{@render heading('Series')}
		<div class="card divide-y divide-gray-700/40 px-4">
			{#each MODES as [k, label, about] (k)}
				<div class="py-3"><Toggle checked={info.tvModes[k]} {label} {about} onchange={(on) => setMode(k, on)} /></div>
			{/each}
			<div class="flex flex-wrap items-center justify-between gap-3 py-3">
				<span><span class="block text-sm">Start on</span><span class="block text-xs text-gray-500">What the request window has picked when it opens.</span></span>
				<div class="inline-flex gap-1 rounded-lg border border-gray-700/60 bg-gray-800/70 p-0.5 text-sm" role="radiogroup" aria-label="Start on">
					{#each MODES.filter(([k]) => info!.tvModes[k]) as [k, label] (k)}
						<button class="rounded-md px-3 py-1 {info.tvModes.default === k ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'}" role="radio" aria-checked={info.tvModes.default === k} onclick={() => save({ tvModes: { ...info!.tvModes, default: k } }, `The request window starts on ${label}.`)}>{label}</button>
					{/each}
				</div>
			</div>
		</div>
	</div>
{/if}
