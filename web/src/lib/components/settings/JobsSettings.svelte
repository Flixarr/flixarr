<script lang="ts">
	import { on } from '#lib/live.svelte.ts';
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import * as cron from '#lib/cron.ts';
	import type { Job } from '#lib/types.ts';
	import Toggle from '#lib/components/Toggle.svelte';
	import Modal from '#lib/components/Modal.svelte';

	const QUICK = [['Every 5 min', '*/5 * * * *'], ['Every 15 min', '*/15 * * * *'], ['Hourly', '0 * * * *'], ['Every 6 hours', '0 */6 * * *'], ['Daily 3 AM', '0 3 * * *'], ['Weekly Sun 5 AM', '0 5 * * 0']];
	const PARTS = [['Minute', '0–59'], ['Hour', '0–23'], ['Day', '1–31'], ['Month', '1–12'], ['Weekday', '0–6, Sun=0']];

	let list = $state<Job[]>([]);
	let editing = $state<Job | null>(null);
	let open = $state(false);
	let parts = $state<string[]>([]);
	let enabled = $state(true);

	const expr = $derived(parts.map((p) => p.trim() || '*').join(' '));
	const preview = $derived.by(() => {
		try {
			return { ok: true as const, text: cron.describe(expr), runs: cron.next(expr, 4) };
		} catch (e) {
			return { ok: false as const, text: e instanceof Error ? e.message : 'Invalid schedule' };
		}
	});

	const load = () => api<Job[]>('/jobs').then((d) => (list = d));
	$effect(() => {
		load();
		// "Last" and "Next" change when a job starts or finishes: reload then (pushed live), not on a timer.
		const offs = [on('job', load), on('resync', load)];
		return () => offs.forEach((off) => off());
	});

	function edit(j: Job) {
		editing = j;
		parts = j.schedule.split(' ');
		enabled = j.enabled;
		open = true;
	}

	async function save() {
		if (!editing || !preview.ok) return;
		try {
			list = await api<Job[]>('/jobs/' + editing.id, { method: 'PUT', body: JSON.stringify({ schedule: expr, enabled }) });
			notify(enabled ? `${editing.name}: ${preview.text.toLowerCase()}.` : `${editing.name} is paused.`);
			open = false;
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}

	async function run(j: Job) {
		try {
			list = await api<Job[]>(`/jobs/${j.id}/run`, { method: 'POST' });
			notify(`${j.name} started.`);
			setTimeout(load, 1500);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not start.', 'orange');
		}
	}

	const describe = (s: string) => {
		try {
			return cron.describe(s);
		} catch {
			return 'Invalid schedule';
		}
	};
</script>

<div class="card px-4">
	{#each list as j (j.id)}
		<div class="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-gray-700/30 py-3 last:border-0 {j.enabled ? '' : 'opacity-60'}">
			<div class="min-w-0 flex-1 basis-56">
				<div class="text-sm">{j.name}</div>
				<div class="mt-0.5 flex flex-wrap items-center gap-2 text-xs"><code class="rounded bg-gray-950/60 px-1.5 py-0.5 text-gray-300">{j.schedule}</code><span class="text-gray-400">{describe(j.schedule)}</span></div>
				<div class="mt-1 text-xs text-gray-500">{j.about}</div>
			</div>
			<div class="text-xs text-gray-400">
				<div>Last: {#if j.running}<span class="text-primary-light">running…</span>{:else if j.last}{cron.fmtRun(j.last.at)} · {#if j.last.error}<span class="text-red-400">{j.last.error}</span>{:else}{j.last.result}{/if}{:else}never{/if}</div>
				<div>Next: {j.enabled ? (j.next && !j.next.startsWith('0001') ? cron.fmtRun(j.next) : '—') : 'Paused'}</div>
			</div>
			<div class="flex gap-1.5"><button class="btn-ghost px-2.5 py-1 text-xs" onclick={() => edit(j)}>Edit</button><button class="btn-ghost px-2.5 py-1 text-xs" disabled={j.running} onclick={() => run(j)}>Run now</button></div>
		</div>
	{/each}
</div>
<p class="text-xs text-gray-500">Schedules run in the server's time zone, set with <code>TZ</code> on the container.</p>

<Modal bind:open title="Edit schedule" sub={editing?.name}>
	<div class="space-y-5 p-5">
		<p class="text-xs text-gray-400">{editing?.about}</p>
		<div class="space-y-2">
			<div class="text-sm font-medium">Quick picks</div>
			<div class="flex flex-wrap gap-1.5">
				{#each QUICK as [l, c] (c)}<button class="rounded-full border border-gray-700/60 bg-gray-800/40 px-3 py-1 text-xs text-gray-300 hover:text-white" onclick={() => (parts = c.split(' '))}>{l}</button>{/each}
				{#if editing && expr !== editing.default}<button class="rounded-full border border-gray-700/60 bg-gray-800/40 px-3 py-1 text-xs text-gray-300 hover:text-white" onclick={() => editing && (parts = editing.default.split(' '))}>Default</button>{/if}
			</div>
		</div>
		<div class="space-y-1.5 text-sm">
			<div class="font-medium">Schedule</div>
			<div class="grid grid-cols-5 gap-2">
				{#each PARTS as [l], i (l)}
					<label class="min-w-0 space-y-1 text-center">
						<span class="block text-[11px] font-medium text-gray-300">{l}</span>
						<input bind:value={parts[i]} spellcheck="false" autocomplete="off" class="w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-1 py-2 text-center font-mono outline-none focus:ring-2 focus:ring-primary/60" />
					</label>
				{/each}
			</div>
			<div class="text-xs text-gray-500"><code>*</code> = every · <code>*/5</code> = every 5th · <code>1-5</code> = range · <code>1,3</code> = list · weekday 0 = Sunday</div>
		</div>
		<div class="rounded-xl border border-gray-700/50 bg-gray-800/40 p-3">
			{#if preview.ok}
				<div class="text-sm font-medium text-green-400">{preview.text}</div>
				<div class="mt-1 font-mono text-xs text-gray-500">{expr}</div>
				<div class="mt-2 text-xs text-gray-400">Next runs</div>
				<ul class="mt-1 space-y-0.5 text-sm text-gray-300">{#each preview.runs as r (r.getTime())}<li>{cron.fmtRun(r)}</li>{/each}</ul>
			{:else}
				<div class="text-sm text-red-400">{preview.text}</div>
			{/if}
		</div>
		<Toggle checked={enabled} label="Enabled" about="Paused jobs don't run until you turn them back on." onchange={(on) => (enabled = on)} />
	</div>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (open = false)}>Cancel</button>
		<button class="btn px-6" disabled={!preview.ok} onclick={save}>Save</button>
	{/snippet}
</Modal>
