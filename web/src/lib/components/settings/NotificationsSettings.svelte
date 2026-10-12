<script lang="ts">
	// Settings → Notifications: one channel at a time (Discord, Webhook). Each has Enabled + Send test,
	// its connection, and which events it sends. Secrets are never shown again after saving.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Toggle from '#lib/components/Toggle.svelte';
	import Select from '#lib/components/Select.svelte';

	type Channel = {
		on: boolean; events: string[]; botName?: string; poster: boolean; mentionHere: boolean; method?: string;
		webhookUrlHint?: string; urlHint?: string; authSet: boolean;
		host?: string; port?: number; security?: string; username?: string; from?: string; fromName?: string; passwordSet?: boolean;
	};
	type Info = { channels: Record<string, Channel>; events: string[]; appUrl: string };

	const CHANNELS: [string, string, string][] = [
		['discord', 'Discord', 'Channel webhook'],
		['webhook', 'Webhook', 'Any service (n8n, Home Assistant…)'],
		['email', 'Email', 'SMTP, to the people involved']
	];
	const EVENTS: Record<string, [string, string]> = {
		request_submitted: ['Request submitted', 'Someone requests something.'],
		request_approved: ['Request approved', 'A waiting request was approved.'],
		request_declined: ['Request declined', 'A waiting request was declined.'],
		request_available: ['Available', 'A requested title is ready to watch in Plex.'],
		request_failed: ['Failed', "Radarr or Sonarr couldn't add it."],
		issue_reported: ['Issue reported', 'Someone reported a problem with a title.'],
		issue_resolved: ['Issue resolved', 'A reported problem was fixed.']
	};
	const SOON: string[] = [];
	const input = 'mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60';

	let info = $state<Info | null>(null);
	let tab = $state('discord');
	let form = $state<Channel & { secret: string; auth: string; clearAuth: boolean }>({ on: false, events: [], poster: true, mentionHere: false, authSet: false, secret: '', auth: '', clearAuth: false });
	let busy = $state('');

	async function load() {
		info = await api<Info>('/notifications');
		pick(tab);
	}
	$effect(() => {
		load();
	});
	function pick(k: string) {
		tab = k;
		form = { ...info!.channels[k], secret: '', auth: '', clearAuth: false };
	}
	const saved = $derived(info?.channels[tab]);
	const hasSecret = $derived(tab === 'discord' ? !!saved?.webhookUrlHint : tab === 'email' ? !!(form.host?.trim() && form.from?.trim()) : !!saved?.urlHint);

	function body() {
		const b: Record<string, unknown> = { on: form.on, events: form.events, poster: form.poster, mentionHere: form.mentionHere, botName: form.botName, method: form.method };
		if (tab === 'discord') b.webhookUrl = form.secret.trim();
		else if (tab === 'email') Object.assign(b, { host: form.host?.trim(), port: Number(form.port), security: form.security, username: form.username?.trim(), password: form.secret, from: form.from?.trim(), fromName: form.fromName, clearAuth: form.clearAuth });
		else Object.assign(b, { url: form.secret.trim(), auth: form.auth.trim(), clearAuth: form.clearAuth });
		return JSON.stringify(b);
	}
	async function save() {
		busy = 'save';
		try {
			const c = await api<Channel>(`/notifications/${tab}`, { method: 'PUT', body: body() });
			info!.channels[tab] = c;
			form = { ...c, secret: '', auth: '', clearAuth: false };
			notify(c.on ? `${CHANNELS.find(([k]) => k === tab)?.[1]} notifications saved.` : 'Saved. This channel is off.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		} finally {
			busy = '';
		}
	}
	async function test() {
		busy = 'test';
		try {
			await api(`/notifications/${tab}/test`, { method: 'POST', body: body() });
			notify('Test sent. Check that it arrived.', 'green');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Test not sent.', 'orange');
		} finally {
			busy = '';
		}
	}
	function setEvent(k: string, on: boolean) {
		form.events = on ? [...form.events, k] : form.events.filter((x) => x !== k);
	}
	const name = $derived(CHANNELS.find(([k]) => k === tab)?.[1] ?? '');
</script>

{#snippet heading(t: string)}<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">{t}</div>{/snippet}

{#if info}
	<nav class="no-scrollbar flex gap-1.5 overflow-x-auto">
		{#each CHANNELS as [k, l] (k)}
			<button class="flex shrink-0 items-center gap-2.5 rounded-xl px-3 py-2 text-left text-sm transition {k === tab ? 'bg-primary/15 text-white ring-1 ring-primary/50' : 'text-gray-400 hover:bg-white/5 hover:text-white'}" onclick={() => pick(k)}>
				<span class="size-2 shrink-0 rounded-full {info.channels[k].on ? 'bg-green-500' : 'bg-gray-600'}"></span>{l}<span class="text-[11px] text-gray-500">{info.channels[k].on ? 'On' : 'Off'}</span>
			</button>
		{/each}
	</nav>

	<div>
		{@render heading(name)}
		<div class="card px-4">
			<div class="flex items-center gap-4 py-3">
				<div class="flex-1"><Toggle checked={form.on} label="Enabled" about={form.on ? 'Sending notifications through this channel.' : 'Turned off. Nothing is sent.'} onchange={(on) => (form.on = on)} /></div>
				<button class="btn-ghost shrink-0 px-3 py-1 text-xs" disabled={!!busy || (!form.secret.trim() && !hasSecret)} onclick={test}>{busy === 'test' ? 'Sending…' : 'Send test'}</button>
			</div>
		</div>
	</div>

	<div>
		{@render heading('Connection')}
		<div class="card divide-y divide-gray-700/40 px-4">
			{#if tab === 'discord'}
				<div class="py-3">
					<label for="n-url" class="block text-sm">Webhook URL</label>
					<input id="n-url" class={input} type="password" autocomplete="off" spellcheck="false" bind:value={form.secret} placeholder={saved?.webhookUrlHint ? `Saved (${saved.webhookUrlHint}). Paste a new one to replace it.` : 'https://discord.com/api/webhooks/…'} />
					<p class="mt-1 text-xs text-gray-500">In Discord: the channel's settings, Integrations, Webhooks, New Webhook, Copy Webhook URL.</p>
				</div>
				<div class="py-3">
					<label for="n-bot" class="block text-sm">Bot name</label>
					<input id="n-bot" class={input} bind:value={form.botName} placeholder="Flixarr" />
				</div>
				<div class="py-3"><Toggle checked={form.poster} label="Include the poster" onchange={(on) => (form.poster = on)} /></div>
				<div class="py-3"><Toggle checked={form.mentionHere} label="Mention @here for new requests" about="Pings everyone online in the channel when someone requests something." onchange={(on) => (form.mentionHere = on)} /></div>
			{:else if tab === 'email'}
				<div class="grid gap-4 py-3 tablet:grid-cols-[1fr_7rem_10rem]">
					<div><label for="n-host" class="block text-sm">SMTP server</label><input id="n-host" class={input} bind:value={form.host} placeholder="smtp.example.com" autocomplete="off" spellcheck="false" /></div>
					<div><label for="n-port" class="block text-sm">Port</label><input id="n-port" class={input} type="number" bind:value={form.port} /></div>
					<div><label for="n-sec" class="block text-sm">Security</label><Select id="n-sec" bind:value={form.security as string} options={[{ value: 'starttls', label: 'STARTTLS' }, { value: 'tls', label: 'SSL/TLS' }, { value: 'none', label: 'None' }]} onchange={(v) => (form.port = v === 'tls' ? 465 : v === 'starttls' ? 587 : 25)} /></div>
				</div>
				<div class="grid gap-4 py-3 tablet:grid-cols-2">
					<div><label for="n-user" class="block text-sm">Username</label><input id="n-user" class={input} bind:value={form.username} autocomplete="off" spellcheck="false" placeholder="Often the From address" /></div>
					<div>
						<label for="n-pass" class="block text-sm">Password</label>
						<input id="n-pass" class={input} type="password" autocomplete="new-password" bind:value={form.secret} placeholder={form.passwordSet && !form.clearAuth ? 'Saved. Type a new one to replace it.' : ''} />
						{#if form.passwordSet && !form.clearAuth}<button class="mt-1 text-xs text-gray-500 hover:text-red-300" onclick={() => (form.clearAuth = true)}>Remove it</button>{/if}
					</div>
				</div>
				<div class="grid gap-4 py-3 tablet:grid-cols-2">
					<div><label for="n-from" class="block text-sm">From address</label><input id="n-from" class={input} bind:value={form.from} placeholder="flixarr@example.com" autocomplete="off" spellcheck="false" /></div>
					<div><label for="n-fromname" class="block text-sm">From name</label><input id="n-fromname" class={input} bind:value={form.fromName} placeholder="Flixarr" /></div>
				</div>
				<p class="py-3 text-xs text-gray-500">Emails go to the people involved: new requests and new issues to whoever manages them, everything else to the person who asked. Addresses come from their Plex accounts. Send test goes to you.</p>
			{:else}
				<div class="py-3">
					<label for="n-hook" class="block text-sm">URL</label>
					<input id="n-hook" class={input} type="password" autocomplete="off" spellcheck="false" bind:value={form.secret} placeholder={saved?.urlHint ? `Saved (${saved.urlHint}). Paste a new one to replace it.` : 'https://example.com/hook'} />
				</div>
				<div class="grid gap-4 py-3 tablet:grid-cols-2">
					<div>
						<label for="n-method" class="block text-sm">Method</label>
						<Select id="n-method" bind:value={form.method as string} options={[{ value: 'POST', label: 'POST' }, { value: 'PUT', label: 'PUT' }]} />
					</div>
					<div>
						<label for="n-auth" class="block text-sm">Authorization header</label>
						<input id="n-auth" class={input} type="password" autocomplete="off" spellcheck="false" bind:value={form.auth} placeholder={form.authSet && !form.clearAuth ? 'Saved. Type a new one to replace it.' : 'Bearer …'} />
						{#if form.authSet && !form.clearAuth}<button class="mt-1 text-xs text-gray-500 hover:text-red-300" onclick={() => (form.clearAuth = true)}>Remove it</button>{/if}
					</div>
				</div>
				<p class="py-3 text-xs text-gray-500">Sends JSON with the event, title, message, the title's type and TMDB ID, and the request (status, who asked, seasons){info.appUrl ? ', plus a link into Flixarr' : ''}.</p>
			{/if}
		</div>
		{#if !info.appUrl}<p class="mt-2 text-xs text-gray-500">Set the application URL in <a href="#general" class="text-primary-light hover:underline">General</a> so notifications link back to Flixarr.</p>{/if}
	</div>

	<div>
		{@render heading('Events')}
		<div class="card divide-y divide-gray-700/40 px-4">
			{#each info.events as k (k)}
				<div class="py-3"><Toggle checked={form.events.includes(k)} label={EVENTS[k]?.[0] ?? k} about={EVENTS[k]?.[1]} disabled={SOON.includes(k)} onchange={(on) => setEvent(k, on)} /></div>
			{/each}
		</div>
		<p class="mt-2 text-xs text-gray-500">Which events send through {name}. Web Push comes later.</p>
	</div>

	<div class="flex justify-end">
		<button class="btn px-6" disabled={!!busy} onclick={save}>{busy === 'save' ? 'Saving…' : 'Save'}</button>
	</div>
{/if}
