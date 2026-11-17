<script lang="ts">
	// Settings → Services (design C4, #233): one line on how everything is, four numbers that jump to their
	// section, then what needs a look, every download, storage by drive, the connections and instant updates.
	// Works the same for a new install (nothing connected: no numbers, just the connections to set up).
	import { api, fmtBytes } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { ServiceKind, ServiceView } from '#lib/types.ts';
	import Tooltip from '#lib/components/Tooltip.svelte';
	import { on } from '#lib/live.svelte.ts';
	import ConnectionModal from './ConnectionModal.svelte';
	import ConnectionRow from './ConnectionRow.svelte';
	import DownloadsSection from './DownloadsSection.svelte';
	import InstantUpdates from './InstantUpdates.svelte';
	import MaintainerrModal from './MaintainerrModal.svelte';
	import SettingsSection from './SettingsSection.svelte';
	import { KINDS, health, isStuck, drives, driveName, maintainerrOn, plural, lowerFirst, shortVersion, updateText, type MaintainerrInfo, type Services } from './services.ts';

	let saved = $state<Services | null>(null);
	let maint = $state<MaintainerrInfo | null>(null);
	let now = $state(Date.now());
	let modal = $state<ConnectionModal>();
	let maintModal = $state<MaintainerrModal>();

	// What Radarr/Sonarr were last read as. Reloads when that changes (pushed over the live connection after
	// every read, including webhook-triggered ones) or after a gap in the connection.
	const loadMaint = () => api<MaintainerrInfo>('/maintainerr').then((d) => (maint = d)).catch(() => {});
	$effect(() => {
		const load = () => api<Services>('/services').then((d) => (saved = d)).catch(() => {});
		load();
		loadMaint();
		const tick = setInterval(() => (now = Date.now()), 5000); // "Next update in" only; no requests
		const offs = [on('services', load), on('resync', () => (load(), loadMaint()))];
		return () => {
			clearInterval(tick);
			offs.forEach((off) => off());
		};
	});

	const setUp = $derived(saved ? KINDS.filter((k) => saved![k.kind]) : []);
	const live = $derived(saved ? setUp.filter((k) => saved![k.kind]!.status?.connected) : []);
	const maintSet = $derived(maintainerrOn(maint));
	const maintOk = $derived(maintSet && !!maint?.connected);
	const kindOf = (k: ServiceKind) => KINDS.find((x) => x.kind === k)!;
	const webOf = (v: ServiceView) => v.webUrl || v.url;

	// Problems: a server that can't be reached (Edit connection), then each health warning (Open the app).
	type Problem = { logo: string; text: string; red: boolean; fix: () => void; fixText: string; href?: string };
	const problems = $derived.by(() => {
		if (!saved) return [] as Problem[];
		const out: Problem[] = [];
		for (const k of setUp) {
			const v = saved[k.kind]!, st = v.status;
			if (!st) continue;
			if (!st.connected) out.push({ logo: k.kind, text: `Can't connect: ${st.error ?? 'no answer'}`, red: true, fix: () => modal?.edit(k.kind), fixText: 'Edit connection' });
			else for (const h of st.health) out.push({ logo: k.kind, text: h, red: false, fix: () => {}, fixText: `Open ${v.name}`, href: webOf(v) });
		}
		if (maintSet && !maint!.connected) out.push({ logo: 'maintainerr', text: `Can't connect: ${maint!.error ?? 'no answer'}`, red: true, fix: () => maintModal?.edit(), fixText: 'Edit connection' });
		return out;
	});
	const problemFrom = $derived([...new Set(problems.map((p) => (p.logo === 'maintainerr' ? 'Maintainerr' : kindOf(p.logo as ServiceKind).name)))].join(' and '));

	const queue = $derived(saved ? live.flatMap((k) => saved![k.kind]!.status!.queue) : []);
	const stuck = $derived(queue.filter(isStuck).length);
	const storage = $derived(saved ? drives(saved) : []);
	const fullest = $derived(storage.slice().sort((a, b) => b.used - a.used)[0]);

	// The one line at the top.
	const summary = $derived.by(() => {
		if (!setUp.length) return { text: 'Nothing connected yet', sub: 'Connect Radarr and Sonarr so requests have somewhere to go.', dot: 'bg-gray-500' };
		const bad = setUp.filter((k) => ['down', 'issues'].includes(health(saved![k.kind]!)));
		if (bad.length) {
			const parts = bad.map((k) => { const st = saved![k.kind]!.status!; return st.connected ? `${plural(st.health.length, 'warning')} from ${k.name}` : `${k.name} can't be reached`; });
			if (stuck) parts.push(`${stuck} download${stuck === 1 ? ' needs' : 's need'} attention`);
			return { text: `${bad.map((k) => k.name).join(' and ')} ${bad.length === 1 ? 'needs' : 'need'} attention`, sub: parts.join(' · '), dot: bad.some((k) => !saved![k.kind]!.status!.connected) ? 'bg-red-500' : 'bg-yellow-400' };
		}
		const full = storage.find((d) => d.used > 90);
		if (full) return { text: `${driveName(full)} drive is almost full`, sub: `${fmtBytes(full.free)} free of ${fmtBytes(full.total)}.`, dot: 'bg-red-500' };
		if (stuck) return { text: `${stuck} download${stuck === 1 ? ' needs' : 's need'} attention`, sub: 'Everything else is running.', dot: 'bg-yellow-400' };
		return { text: 'Everything is running', sub: queue.length ? `${queue.length} downloading right now.` : 'Nothing is downloading right now.', dot: 'bg-green-500' };
	});

	// The Connected number: Radarr, Sonarr, and Maintainerr once it's set up.
	const conn = $derived.by(() => {
		const all = [...KINDS.map((k) => ({ name: k.name, set: !!saved?.[k.kind], ok: live.includes(k) })), ...(maintSet ? [{ name: 'Maintainerr', set: true, ok: maintOk }] : [])];
		const ok = all.filter((x) => x.ok).length, off = all.filter((x) => x.set && !x.ok).map((x) => x.name), missing = all.filter((x) => !x.set).map((x) => x.name);
		const sub = off.length ? `${off.length > 1 ? `${off.length} can't` : `${off[0]} can't`} connect` : missing.length ? `${missing.join(' and ')} ${missing.length > 1 ? "aren't" : "isn't"} connected` : 'Everything is connected';
		return { value: `${ok} of ${all.length}`, sub, warn: ok < all.length };
	});

	const jump = (id: string) => document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
	async function refresh(k: ServiceKind) {
		try {
			const n = await api<ServiceView>(`/services/${k}/refresh`, { method: 'POST' });
			saved![k] = n;
			notify(`${n.name} refreshed.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Unable to refresh', 'orange');
		}
	}
	const pill = (v: ServiceView) => ({ checking: 'Checking…', down: "Can't connect", ok: 'Healthy', issues: plural(v.status?.health.length ?? 0, 'issue') })[health(v)];
	const freed = $derived(maint?.stats.reduce((n, s) => n + s.handledBytes, 0) ?? 0);
</script>

{#snippet tile(id: string, label: string, value: string, sub: string, tone: string)}
	<button class="block min-w-0 rounded-xl bg-gray-800/40 p-3 text-left ring-1 ring-gray-700/40 transition hover:bg-gray-800/70" onclick={() => jump(id)}>
		<span class="block text-[11px] text-gray-500">{label}</span>
		<span class="mt-0.5 block truncate text-xl font-bold tracking-tight tabular-nums {tone}">{value}</span>
		<span class="block truncate text-[11px] text-gray-500">{sub}</span>
	</button>
{/snippet}

{#if saved}
	<div class="space-y-6">
		<div>
			<div class="flex items-center gap-3 px-1">
				<span class="size-2.5 shrink-0 rounded-full {summary.dot}"></span>
				<div class="min-w-0"><div class="text-base font-semibold">{summary.text}</div><div class="text-xs text-gray-400">{summary.sub}</div></div>
			</div>
			<!-- Four zeros on a new install say nothing, so the numbers wait until something is connected. -->
			{#if setUp.length}
				<div class="mt-5 grid grid-cols-2 gap-3 tablet:grid-cols-4">
					{@render tile('svc-problems', 'Problems', String(problems.length), problems.length ? `From ${problemFrom}` : 'None', problems.length ? 'text-yellow-400' : 'text-green-400')}
					{@render tile('svc-downloads', 'In the queue', String(queue.length), stuck ? `${stuck} ${stuck === 1 ? 'needs' : 'need'} attention` : queue.length ? 'All moving' : 'Nothing downloading', stuck ? 'text-yellow-400' : '')}
					{@render tile('svc-storage', 'Free space', fullest ? fmtBytes(fullest.free) : 'None', fullest ? (storage.length > 1 ? `Lowest of ${storage.length} drives` : driveName(fullest)) : 'Nothing reported', fullest && fullest.used > 90 ? 'text-red-400' : '')}
					{@render tile('svc-connections', 'Connected', conn.value, conn.sub, conn.warn ? 'text-yellow-400' : '')}
				</div>
			{/if}
		</div>

		{#if problems.length}
			<SettingsSection title="Needs a look" id="svc-problems">
				<div class="card divide-y divide-gray-700/40">
					{#each problems as p, i (i)}
						<div class="flex items-center gap-3 px-4 py-2.5 text-sm">
							<img src="/logos/{p.logo}.svg" alt="" class="size-5 shrink-0" />
							<span class="min-w-0 flex-1 {p.red ? 'text-red-300' : 'text-yellow-300'}">{p.text}</span>
							{#if p.href}<a class="shrink-0 whitespace-nowrap text-xs text-primary-light hover:underline" href={p.href} target="_blank" rel="noopener">{p.fixText}</a>
							{:else}<button class="shrink-0 whitespace-nowrap text-xs text-primary-light hover:underline" onclick={p.fix}>{p.fixText}</button>{/if}
						</div>
					{/each}
				</div>
			</SettingsSection>
		{/if}

		{#if live.length}<DownloadsSection {saved} />{/if}

		{#if storage.length}
			<SettingsSection title="Storage" id="svc-storage">
				<div class="card divide-y divide-gray-700/40">
					{#each storage as d (d.folders.join())}
						<div class="flex items-center gap-3 px-4 py-3">
							<span class="grid size-8 shrink-0 place-items-center rounded-lg bg-gray-800 text-gray-300"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="size-4"><ellipse cx="12" cy="6" rx="8" ry="3" /><path d="M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6" /></svg></span>
							<div class="min-w-0 flex-1">
								<div class="flex items-baseline justify-between gap-3">
									<span class="text-sm font-medium">{driveName(d)}<span class="ml-1.5 inline-flex gap-1 align-[-2px]">{#each d.kinds as k (k)}<img src="/logos/{k}.svg" alt="" class="size-3.5" />{/each}</span></span>
									<span class="text-sm"><b class={d.used > 90 ? 'text-red-400' : ''}>{fmtBytes(d.free)}</b> <span class="text-xs text-gray-500">free{d.total ? ` of ${fmtBytes(d.total)}` : ''}</span></span>
								</div>
								{#if d.total}
									<div class="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full {d.used > 90 ? 'bg-red-500' : d.used > 80 ? 'bg-yellow-400' : 'bg-gray-400'}" style="width:{d.used}%"></div></div>
								{:else}<div class="mt-1 text-[11px] text-gray-500">Drive size not reported</div>{/if}
								<div class="mt-1 truncate font-mono text-[10px] text-gray-500">{d.folders.join('  ·  ')}</div>
							</div>
						</div>
					{/each}
				</div>
			</SettingsSection>
		{/if}

		<SettingsSection title="Connections" id="svc-connections">
			<div class="card divide-y divide-gray-700/40">
				{#each KINDS as k (k.kind)}
					{@const v = saved[k.kind]}
					{#if !v}
						<ConnectionRow logo={k.kind} name={k.name} about={`${k.name === 'Radarr' ? 'Movie' : 'Series'} requests are sent to ${k.name}.`} status="none" onedit={() => modal?.edit(k.kind)} />
					{:else}
						{@const st = v.status}
						{@const installed = v.profiles.filter((p) => p.installed)}
						{@const edited = installed.filter((p) => p.state === 'customized').length}
						{@const ready = installed.filter((p) => p.updateAvailable).length}
						<ConnectionRow logo={k.kind} name={v.name} version={shortVersion(st?.version)} about={k.about} status={health(v)} pill={pill(v)} web={webOf(v)}
							onedit={() => modal?.edit(k.kind)} onremove={() => modal?.askRemove(k.kind)} onrefresh={() => refresh(k.kind)}>
							{#if st && !st.connected}
								<span class="text-red-300">{st.error}</span> Last connected {st.lastConnected ? new Date(st.lastConnected).toLocaleString() : 'never'}.
							{:else if st}
								{@const u = updateText(v, now)}
								{plural(st.indexers, 'indexer')}{#if st.indexersDown} <span class="text-yellow-400">({st.indexersDown} down)</span>{/if}
								· {st.clients.length ? st.clients.join(', ') : 'No download client'}{#if st.clientsDown} <span class="text-yellow-400">(down)</span>{/if}
								{#if v.profiles.length}
									· {#if !installed.length}Flixarr profiles not added, <button class="text-primary-light hover:underline" onclick={() => modal?.edit(k.kind)}>Add them</button>
									{:else}<span class={edited ? 'text-yellow-300' : ''}>{installed.length} Flixarr profile{installed.length === 1 ? '' : 's'}, {edited ? `${edited} edited in ${k.name}` : ready ? `${ready === 1 ? 'an update is' : `${ready} updates are`} ready` : 'up to date'}</span>{/if}
								{/if}
								· <Tooltip text={u.title}><span class="cursor-default outline-none {u.warn ? 'text-yellow-400' : ''}">{lowerFirst(u.text)}</span></Tooltip>
							{:else}Checking…{/if}
						</ConnectionRow>
					{/if}
				{/each}
				{#if !maintSet}
					<ConnectionRow logo="maintainerr" name="Maintainerr" about="Optional. Removes titles from Plex by rules you set, with a Leaving Soon row and Keep buttons." status="none" onedit={() => maintModal?.edit()} />
				{:else}
					<ConnectionRow logo="maintainerr" name="Maintainerr" version={shortVersion(maint!.version)} about="" status={maint!.connected ? 'ok' : 'down'} pill={maint!.connected ? 'Healthy' : "Can't connect"}
						web={maint!.url} onedit={() => maintModal?.edit()} onremove={() => maintModal?.askRemove()} onrefresh={() => loadMaint().then(() => notify('Maintainerr refreshed.'))}>
						{#if maint!.connected}
							<a href="/leaving" class="hover:text-gray-300 hover:underline">{maint!.leaving ?? 0} leaving soon</a> · {maint!.keeps ?? 0} kept by Flixarr · {fmtBytes(freed)} freed
						{:else}<span class="text-red-300">{maint!.error ?? "Maintainerr isn't answering."}</span>{/if}
					</ConnectionRow>
				{/if}
			</div>
		</SettingsSection>

		<InstantUpdates kinds={setUp} />
	</div>
	<ConnectionModal bind:this={modal} bind:saved={saved} />
	<MaintainerrModal bind:this={maintModal} bind:info={maint} onchange={loadMaint} />
{/if}
