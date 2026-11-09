<script lang="ts">
	// One Radarr or Sonarr server: how it's doing at a glance, with the queue and any problems underneath.
	import { untrack } from 'svelte';
	import { api, fmtBytes, ago } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { QueueRow, ServiceView } from '#lib/types.ts';
	import Icon from '#lib/components/Icon.svelte';
	import Tooltip from '#lib/components/Tooltip.svelte';
	import ServiceStat from './ServiceStat.svelte';
	import { health, queueLabel, type Kind } from './services.ts';

	let { s, v, now, onedit, onremove, onchange }: { s: Kind; v: ServiceView; now: number; onedit: () => void; onremove: () => void; onchange: (v: ServiceView) => void } = $props();

	const QUEUE_ROWS = 5;
	const STATE_TEXT: Record<string, string> = { queued: 'Waiting', paused: 'Paused', importing: 'Importing', warning: 'Needs attention', error: 'Failed' };
	const STATE_TONE: Record<string, string> = { warning: 'text-yellow-400', error: 'text-red-400', paused: 'text-gray-400', queued: 'text-gray-500', importing: 'text-green-400' };
	const PILL: Record<string, { text: string; tone: string; dot: string }> = {
		checking: { text: 'Checking…', tone: 'text-gray-400 ring-gray-700/60', dot: 'bg-gray-500' },
		down: { text: 'Not connected', tone: 'bg-red-500/10 text-red-300 ring-red-500/30', dot: 'bg-red-500' },
		issues: { text: '', tone: 'bg-yellow-500/10 text-yellow-300 ring-yellow-500/30', dot: 'bg-yellow-400' },
		ok: { text: 'Healthy', tone: 'bg-green-500/10 text-green-300 ring-green-500/30', dot: 'bg-green-500' }
	};

	let menu = $state(false);
	let refreshing = $state(false);
	// Whether the queue is open is remembered in this browser.
	const openKey = 'flixarr.queueOpen';
	let opened = $state(!!JSON.parse(localStorage.getItem(openKey) || '{}')[untrack(() => s.kind)]);
	function toggleQueue() {
		opened = !opened;
		localStorage.setItem(openKey, JSON.stringify({ ...JSON.parse(localStorage.getItem(openKey) || '{}'), [s.kind]: opened }));
	}

	$effect(() => {
		if (!menu) return;
		const away = (e: PointerEvent) => !(e.target as Element).closest('[data-svc-menu]') && (menu = false);
		document.addEventListener('pointerdown', away);
		return () => document.removeEventListener('pointerdown', away);
	});

	async function refresh() {
		refreshing = true;
		try {
			const n = await api<ServiceView>(`/services/${s.kind}/refresh`, { method: 'POST' });
			onchange(n);
			notify(`${n.name} refreshed.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Unable to refresh', 'orange');
		} finally {
			refreshing = false;
			menu = false;
		}
	}
	async function copy() {
		menu = false;
		try {
			await navigator.clipboard.writeText(web);
			notify('Address copied.');
		} catch {
			notify(web, 'orange');
		}
	}

	// "Next update in 42 sec" normally; "Updates paused" when the job is off; "Updated X ago" in yellow
	// when the last read is overdue. The other wording is the tooltip.
	function updateText(): { text: string; title: string; warn: boolean } {
		const st = v.status!;
		const age = `Updated ${ago(st.updatedAt, now)}`;
		if (v.updatesPaused) return { text: 'Updates paused', title: age, warn: false };
		if (now - Date.parse(st.updatedAt) > 420000) return { text: age, title: 'The refresh-status job is overdue', warn: true }; // idle servers are read every 5 min
		const secs = v.nextUpdate ? Math.round((Date.parse(v.nextUpdate) - now) / 1000) : -1;
		if (secs <= 0) return { text: 'Updating…', title: age, warn: false };
		const left = secs < 60 ? `${secs} sec` : secs < 3600 ? `${Math.round(secs / 60)} min` : `${Math.round(secs / 3600)} hr`;
		return { text: `Next update in ${left}`, title: age, warn: false };
	}

	const st = $derived(v.status);
	const web = $derived(v.webUrl || v.url);
	const condition = $derived(health(v));
	const issues = $derived(st?.health.length ?? 0);
	const queue = $derived(st?.queue ?? []);
	const trouble = $derived(queue.some((q) => q.state === 'warning' || q.state === 'error'));
	const showQueue = $derived(queue.length > 0 && opened);
	const used = $derived(st?.totalSpace ? Math.round((1 - st.freeSpace / st.totalSpace) * 100) : 0);
	const installed = $derived(v.profiles.filter((p) => p.installed));
	const edited = $derived(installed.filter((p) => p.state === 'customized').length);
	const ready = $derived(installed.filter((p) => p.updateAvailable).length);
	const short = (x?: string) => (x ? 'v' + x.split('.').slice(0, 2).join('.') : '');
	const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;
	const queueValue = $derived(queue.length ? plural(queue.length, 'item') : 'Empty');
	const queueSub = $derived(queue.length ? queueLabel(queue).map((p) => p.text).join(' · ') : 'Nothing downloading');
	const queueTone = $derived(queue.some((q) => q.state === 'error') ? 'text-red-400' : trouble ? 'text-yellow-400' : '');
</script>

{#snippet queueRow(q: QueueRow)}
	<div class="py-2.5">
		<div class="flex items-baseline justify-between gap-3 text-xs"><span class="min-w-0 truncate text-gray-200">{q.title}</span>
			<span class="shrink-0 tabular-nums {STATE_TONE[q.state] ?? 'text-gray-400'}">{q.state === 'downloading' ? `${q.percent}%${q.timeLeft ? ` · ${q.timeLeft}` : ''}` : STATE_TEXT[q.state]}</span></div>
		<div class="mt-1.5 h-1 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full {q.state === 'importing' ? 'arr-stripes' : ''}"
			style="width:{q.state === 'queued' ? 0 : Math.max(q.percent, 2)}%; background:{q.state === 'error' ? '#ef4444' : q.state === 'warning' ? '#facc15' : q.state === 'paused' ? '#6b7280' : s.color}"></div></div>
		<div class="mt-1 text-[11px] text-gray-500">{q.quality}{q.size ? ` · ${fmtBytes(q.size)}` : ''}</div>
		{#if q.message}<div class="mt-1 text-[11px] {STATE_TONE[q.state]}">⚠ {q.message}</div>{/if}
		{#if (q.state === 'warning' || q.state === 'error') && (q.requestId || q.tmdbId)}
			<a class="mt-1 inline-flex items-center gap-1 text-[11px] text-primary-light hover:underline" href={q.requestId ? `/requests?open=${q.requestId}` : `/${q.type}/${q.tmdbId}?admin=1`}>Fix it on {q.requestId ? 'its request' : 'the title'}<Icon name="chevron" class="size-3" /></a>
		{/if}
	</div>
{/snippet}

<article class="card flex flex-col overflow-hidden" style="border-top:2px solid rgb({s.rgb} / 0.55)">
	<div class="flex-1 p-4">
		<header class="flex items-center gap-3">
			<Tooltip text="Open {v.name}" align="left"><a href={web} target="_blank" rel="noopener" aria-label="Open {v.name}" class="block shrink-0 rounded-md transition hover:opacity-80"><img src="/logos/{s.kind}.svg" alt="" class="size-9" /></a></Tooltip>
			<div class="min-w-0 flex-1">
				<a href={web} target="_blank" rel="noopener" class="group/n inline-flex items-center gap-1 font-semibold hover:underline hover:underline-offset-4">{v.name}<span class="text-gray-500 opacity-0 transition group-hover/n:opacity-100"><Icon name="external" class="size-3" /></span></a>
				<div class="truncate text-[11px] text-gray-500">{s.about}{st?.version ? ` ${short(st.version)}` : ''}</div>
			</div>
			<span class="inline-flex shrink-0 items-center gap-1.5 rounded-full px-2.5 py-1 text-xs ring-1 {PILL[condition].tone}"><span class="size-1.5 rounded-full {PILL[condition].dot}"></span>{condition === 'issues' ? plural(issues, 'issue') : PILL[condition].text}</span>
			<div class="relative" data-svc-menu>
				<button class="grid size-8 place-items-center rounded-lg text-gray-400 hover:bg-white/5 hover:text-white" aria-label="More for {v.name}" aria-expanded={menu} onclick={() => (menu = !menu)}><Icon name="dots" class="size-4" /></button>
				{#if menu}
					<div class="absolute right-0 top-full z-20 mt-1 w-44 overflow-hidden rounded-lg border border-gray-700/60 bg-gray-900 py-1 text-xs shadow-[0_16px_40px_-4px_rgb(0_0_0/0.75),0_4px_12px_rgb(0_0_0/0.5)]">
						<button class="flex w-full items-center gap-2 px-3 py-2 text-left text-gray-300 hover:bg-white/5 hover:text-white disabled:opacity-50" disabled={refreshing} onclick={refresh}><Icon name="refresh" class="size-3.5" />{refreshing ? 'Refreshing…' : 'Refresh now'}</button>
						<button class="flex w-full items-center gap-2 px-3 py-2 text-left text-gray-300 hover:bg-white/5 hover:text-white" onclick={copy}><Icon name="copy" class="size-3.5" />Copy address</button>
						<button class="flex w-full items-center gap-2 px-3 py-2 text-left text-red-300 hover:bg-white/5" onclick={() => { menu = false; onremove(); }}><Icon name="trash" class="size-3.5" />Remove</button>
					</div>
				{/if}
			</div>
		</header>

		{#if st && !st.connected}
			<div class="mt-4 rounded-lg bg-red-500/10 px-3 py-2 text-xs text-red-300 ring-1 ring-red-500/30">{st.error}</div>
			<div class="mt-2 flex justify-between text-[11px] text-gray-500"><span>Last connected {ago(st.lastConnected, now)}</span><span>Updated {ago(st.updatedAt, now)}</span></div>
		{:else if st}
			{@const cut = st.folder.lastIndexOf('/')}
			<div class="mt-4 grid grid-cols-2 gap-2">
				<ServiceStat label="Free space" value={fmtBytes(st.freeSpace)} tone={used > 90 ? 'text-red-400' : ''} sub={st.totalSpace ? `${used}% of ${fmtBytes(st.totalSpace)} used` : undefined}>
					{#if st.totalSpace}<span class="mt-2 block h-1 overflow-hidden rounded-full bg-gray-700/60"><span class="block h-full rounded-full" style="width:{used}%; background:{used > 90 ? '#ef4444' : s.color}"></span></span>{/if}
				</ServiceStat>
				<ServiceStat label="Queue" value={queueValue} sub={queueSub} tone={queueTone} expanded={showQueue} onclick={queue.length ? toggleQueue : undefined} />
				<ServiceStat label="Download sources" value={plural(st.indexers, 'indexer')} tone={st.indexersDown || st.clientsDown ? 'text-yellow-400' : ''}
					sub={[st.indexersDown ? `${st.indexersDown} down` : '', st.clients.length ? `${st.clients.join(', ')}${st.clientsDown ? ' down' : ''}` : 'No download client'].filter(Boolean).join(' · ')} />
				{#if v.profiles.length}
					{#if !installed.length}
						<ServiceStat label="Flixarr profiles" value="Not added" sub="Tap to add them" onclick={onedit} />
					{:else}
						<ServiceStat label="Flixarr profiles" value={`${installed.length} added`} href="#profiles" tone={edited ? 'text-yellow-300' : ''}
							sub={edited ? `${edited} edited in ${s.name}` : ready ? `${ready === 1 ? 'An update is' : `${ready} updates are`} ready` : 'Up to date'} />
					{/if}
				{/if}
			</div>

			{#if showQueue}
				<div class="mt-2 divide-y divide-gray-700/30 rounded-lg bg-gray-900/50 px-3 ring-1 ring-gray-700/40">
					{#each queue.slice(0, QUEUE_ROWS) as q, i (i)}{@render queueRow(q)}{/each}
					{#if queue.length > QUEUE_ROWS}<div class="py-2 text-[11px] text-gray-500">{queue.length - QUEUE_ROWS} more in the queue</div>{/if}
				</div>
			{/if}

			{#if issues}
				<ul class="mt-2 space-y-1 rounded-lg bg-yellow-500/10 px-3 py-2 text-[11px] text-yellow-300 ring-1 ring-yellow-500/30">
					{#each st.health as h (h)}<li>⚠ {h}</li>{/each}
				</ul>
			{/if}

			<div class="mt-3 truncate font-mono text-[10px] text-gray-500">{st.folder.slice(0, cut + 1)}<span class="text-gray-400">{st.folder.slice(cut + 1)}</span></div>
		{/if}
	</div>

	<footer class="flex items-center gap-2 border-t border-gray-700/40 px-4 py-2.5 text-xs">
		{#if st?.connected}
			{@const u = updateText()}
			<Tooltip text={u.title} align="left"><span class="cursor-default text-[11px] tabular-nums outline-none {u.warn ? 'text-yellow-400' : 'text-gray-500'}">{u.text}</span></Tooltip>
		{/if}
		<span class="flex-1"></span>
		<button class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-gray-300 hover:bg-white/5 hover:text-white" onclick={onedit}><Icon name="settings" class="size-3.5" />Edit</button>
		<a href={web} target="_blank" rel="noopener" class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-gray-300 hover:bg-white/5 hover:text-white">Open<Icon name="external" class="size-3.5" /></a>
	</footer>
</article>
