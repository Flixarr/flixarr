<script lang="ts">
	// Settings → Notifications: a card per channel. Each service (Discord, email, webhooks…) can have any
	// number of channels, each with its own name, connection and events. A channel opens in a window with
	// Enabled + Send test, its connection and its events. Secrets are never shown again after saving.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Toggle from '#lib/components/Toggle.svelte';
	import Select from '#lib/components/Select.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import Icon from '#lib/components/Icon.svelte';

	type Channel = {
		id: string; service: string; name: string;
		on: boolean; events: string[]; botName?: string; poster: boolean; mentionHere: boolean; method?: string;
		webhookUrlHint?: string; urlHint?: string; authSet: boolean;
		host?: string; port?: number; security?: string; username?: string; from?: string; fromName?: string; passwordSet?: boolean;
		target?: string; server?: string; tokenSet?: boolean;
	};
	type Info = { channels: Channel[]; defaults: Record<string, Channel>; events: string[]; appUrl: string };

	const CHANNELS: [string, string, string][] = [
		['discord', 'Discord', 'Channel webhook'],
		['webhook', 'Webhook', 'Any service (n8n, Home Assistant…)'],
		['email', 'Email', 'SMTP, to the people involved'],
		['push', 'Web Push', 'Browser and phone notifications'],
		['telegram', 'Telegram', 'A chat or group'],
		['slack', 'Slack', 'Channel webhook'],
		['pushover', 'Pushover', 'Phone notifications'],
		['gotify', 'Gotify', 'Self-hosted'],
		['ntfy', 'ntfy', 'ntfy.sh or self-hosted'],
		['pushbullet', 'Pushbullet', 'Phone and browser']
	];
	// The text services: which fields each one needs, with where to find them.
	type Field = { label: string; placeholder: string; help: string; optional?: boolean };
	const SIMPLE: Record<string, { token: Field; target?: Field; server?: Field }> = {
		telegram: { token: { label: 'Bot token', placeholder: '123456:ABC…', help: 'Make a bot with @BotFather in Telegram; it gives you the token.' }, target: { label: 'Chat ID', placeholder: '-1001234567890', help: 'Add the bot to the chat, then get the chat ID (for example from @userinfobot).' } },
		slack: { token: { label: 'Webhook URL', placeholder: 'https://hooks.slack.com/services/…', help: 'In Slack: an app with Incoming Webhooks turned on, Add New Webhook to Workspace, pick the channel.' } },
		pushover: { token: { label: 'Application token', placeholder: 'azGDORePK8gMaC0QOYAMyEEuzJnyUi', help: 'Create an application on pushover.net to get its API token.' }, target: { label: 'User key', placeholder: 'uQiRzpo4DXghDmr9QzzfQu27cmVRsG', help: 'Shown on your pushover.net dashboard (or a group key).' } },
		gotify: { server: { label: 'Gotify address', placeholder: 'https://gotify.example.com', help: '' }, token: { label: 'Application token', placeholder: 'AbCdEf…', help: 'In Gotify: Apps, Create Application, copy its token.' } },
		ntfy: { server: { label: 'Server', placeholder: 'https://ntfy.sh', help: 'ntfy.sh or your own ntfy server.' }, target: { label: 'Topic', placeholder: 'flixarr-alerts', help: 'Subscribe to the same topic in the ntfy app.' }, token: { label: 'Access token', placeholder: 'tk_…', help: 'Only if the topic is protected.', optional: true } },
		pushbullet: { token: { label: 'Access token', placeholder: 'o.AbCdEf…', help: 'pushbullet.com, Settings, Account, Create Access Token.' } }
	};
	const EVENTS: Record<string, [string, string]> = {
		request_submitted: ['Request submitted', 'Someone requests something.'],
		request_approved: ['Request approved', 'A waiting request was approved.'],
		request_declined: ['Request declined', 'A waiting request was declined.'],
		request_available: ['Available', 'A requested title is ready to watch in Plex.'],
		request_failed: ['Failed', "Radarr or Sonarr couldn't add it."],
		issue_reported: ['Issue reported', 'Someone reported a problem with a title.'],
		issue_resolved: ['Issue resolved', 'A reported problem was fixed.'],
		request_leaving: ['Leaving soon', 'A requested title leaves Plex within a week (Maintainerr). Goes to the person who asked.'],
		server_problem: ['Radarr or Sonarr problem', "A server can't be reached, a download failed or is stuck, or it reports a health issue. Admins only."]
	};
	const SOON: string[] = [];
	const input = 'mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60';

	let info = $state<Info | null>(null);
	let tab = $state('discord'); // the open channel's service
	let editing = $state(''); // the open channel's ID, '' for a new one
	let open = $state(false);
	let picking = $state(false);
	let confirmDelete = $state(false);
	let form = $state<Channel & { secret: string; auth: string; clearAuth: boolean }>({ id: '', service: 'discord', name: '', on: false, events: [], poster: true, mentionHere: false, authSet: false, secret: '', auth: '', clearAuth: false });
	let busy = $state('');

	async function load() {
		info = await api<Info>('/notifications');
	}
	$effect(() => {
		load();
	});
	const label = (k: string) => CHANNELS.find(([c]) => c === k)?.[1] ?? k;
	const hasPush = $derived(!!info?.channels.some((c) => c.service === 'push'));

	function edit(c: Channel) {
		tab = c.service;
		editing = c.id;
		form = { ...c, secret: '', auth: '', clearAuth: false };
		open = true;
	}
	function add(k: string) {
		picking = false;
		const taken = info!.channels.filter((c) => c.service === k).length;
		edit({ ...info!.defaults[k], id: '', name: taken ? `${label(k)} ${taken + 1}` : label(k), on: true });
	}
	const saved = $derived(editing ? info?.channels.find((c) => c.id === editing) : undefined);
	const hasSecret = $derived(
		tab === 'push' ? true : tab === 'discord' ? !!saved?.webhookUrlHint : tab === 'email' ? !!(form.host?.trim() && form.from?.trim()) : SIMPLE[tab] ? !!saved?.tokenSet || !!SIMPLE[tab].token.optional : !!saved?.urlHint
	);

	function body() {
		const b: Record<string, unknown> = { id: editing, service: tab, name: form.name, on: form.on, events: form.events, poster: form.poster, mentionHere: form.mentionHere, botName: form.botName, method: form.method };
		if (tab === 'discord') b.webhookUrl = form.secret.trim();
		else if (SIMPLE[tab]) Object.assign(b, { token: form.secret.trim(), target: form.target?.trim(), server: form.server?.trim(), clearAuth: form.clearAuth });
		else if (tab === 'email') Object.assign(b, { host: form.host?.trim(), port: Number(form.port), security: form.security, username: form.username?.trim(), password: form.secret, from: form.from?.trim(), fromName: form.fromName, clearAuth: form.clearAuth });
		else Object.assign(b, { url: form.secret.trim(), auth: form.auth.trim(), clearAuth: form.clearAuth });
		return JSON.stringify(b);
	}
	async function save() {
		busy = 'save';
		try {
			const c = editing ? await api<Channel>(`/notifications/${editing}`, { method: 'PUT', body: body() }) : await api<Channel>('/notifications', { method: 'POST', body: body() });
			await load();
			open = false;
			notify(c.on ? `${c.name} saved.` : `${c.name} saved. It's off.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		} finally {
			busy = '';
		}
	}
	async function remove() {
		busy = 'delete';
		try {
			await api(`/notifications/${editing}`, { method: 'DELETE' });
			const n = form.name;
			await load();
			confirmDelete = false;
			notify(`${n} deleted.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not delete.', 'orange');
		} finally {
			busy = '';
		}
	}
	async function test() {
		busy = 'test';
		try {
			await api('/notifications/test', { method: 'POST', body: body() });
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
	const name = $derived(label(tab));
	// What a card says it sends: the events by name, or how many.
	function sends(c: Channel) {
		const ev = c.events.filter((e) => info!.events.includes(e));
		if (!ev.length) return 'No events picked';
		if (ev.length === info!.events.length) return 'Every event';
		if (ev.length <= 2) return ev.map((e) => EVENTS[e]?.[0] ?? e).join(', ');
		return `${ev.length} events`;
	}
</script>

{#snippet heading(t: string)}<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">{t}</div>{/snippet}

{#if info}
	<div>
		{@render heading('Channels')}
		<div class="grid gap-3 tablet:grid-cols-2">
			{#each info.channels as c (c.id)}
				<button class="card flex items-center gap-3 px-4 py-3 text-left transition hover:ring-1 hover:ring-gray-600" onclick={() => edit(c)}>
					<span class="size-2 shrink-0 rounded-full {c.on ? 'bg-green-500' : 'bg-gray-600'}"></span>
					<span class="min-w-0 flex-1">
						<span class="block truncate text-sm font-medium">{c.name}</span>
						<span class="block truncate text-xs text-gray-500">{label(c.service)} · {sends(c)}</span>
					</span>
					<span class="shrink-0 text-[11px] {c.on ? 'text-green-400' : 'text-gray-500'}">{c.on ? 'On' : 'Off'}</span>
				</button>
			{/each}
			<button class="flex items-center justify-center gap-2 rounded-xl border border-dashed border-gray-700 p-4 text-sm text-gray-400 transition hover:border-gray-500 hover:text-white" onclick={() => (picking = true)}>
				<Icon name="plus" class="size-4" />Add channel
			</button>
		</div>
		<p class="mt-2 text-xs text-gray-500">Add as many channels of each service as you need, each with its own events. An event goes to every channel that has it turned on.</p>
		{#if !info.appUrl}<p class="mt-2 text-xs text-gray-500">Set the application URL in <a href="#general" class="text-primary-light hover:underline">General</a> so notifications link back to Flixarr.</p>{/if}
	</div>

	<Modal bind:open={picking} title="Add channel" sub="Pick where notifications go.">
		<div class="grid gap-2 p-5 tablet:grid-cols-2">
			{#each CHANNELS as [k, l, about] (k)}
				{@const off = k === 'push' && hasPush}
				<button class="rounded-xl px-3 py-2.5 text-left ring-1 ring-gray-700/60 transition {off ? 'cursor-default opacity-50' : 'hover:bg-white/5 hover:ring-gray-500'}" disabled={off} onclick={() => add(k)}>
					<span class="block text-sm">{l}</span>
					<span class="block text-xs text-gray-500">{off ? 'Already set up. Each person picks their devices.' : about}</span>
				</button>
			{/each}
		</div>
	</Modal>

	<Modal bind:open title={editing ? form.name || name : `Add ${name} channel`} sub={name}>
		<div class="space-y-5 p-5">
			<div class="card divide-y divide-gray-700/40 px-4">
				<div class="py-3">
					<label for="n-name" class="block text-sm">Name</label>
					<input id="n-name" class={input} bind:value={form.name} maxlength="60" placeholder="{name}: requests" />
				</div>
				<div class="flex items-center gap-4 py-3">
					<div class="flex-1"><Toggle checked={form.on} label="Enabled" about={form.on ? 'Sending notifications through this channel.' : 'Turned off. Nothing is sent.'} onchange={(on) => (form.on = on)} /></div>
					<button class="btn-ghost shrink-0 px-3 py-1 text-xs" disabled={!!busy || (!form.secret.trim() && !hasSecret)} onclick={test}>{busy === 'test' ? 'Sending…' : 'Send test'}</button>
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
				{:else if SIMPLE[tab]}
					{@const f = SIMPLE[tab]}
					{#if f.server}
						<div class="py-3"><label for="n-server" class="block text-sm">{f.server.label}</label><input id="n-server" class={input} bind:value={form.server} placeholder={f.server.placeholder} autocomplete="off" spellcheck="false" />{#if f.server.help}<p class="mt-1 text-xs text-gray-500">{f.server.help}</p>{/if}</div>
					{/if}
					{#if f.target}
						<div class="py-3"><label for="n-target" class="block text-sm">{f.target.label}</label><input id="n-target" class={input} bind:value={form.target} placeholder={f.target.placeholder} autocomplete="off" spellcheck="false" /><p class="mt-1 text-xs text-gray-500">{f.target.help}</p></div>
					{/if}
					<div class="py-3">
						<label for="n-token" class="block text-sm">{f.token.label}{#if f.token.optional} <span class="text-gray-500">(optional)</span>{/if}</label>
						<input id="n-token" class={input} type="password" autocomplete="off" spellcheck="false" bind:value={form.secret} placeholder={saved?.tokenSet && !form.clearAuth ? 'Saved. Paste a new one to replace it.' : f.token.placeholder} />
						<p class="mt-1 text-xs text-gray-500">{f.token.help}</p>
						{#if saved?.tokenSet && f.token.optional && !form.clearAuth}<button class="mt-1 text-xs text-gray-500 hover:text-red-300" onclick={() => (form.clearAuth = true)}>Remove it</button>{/if}
					</div>
				{:else if tab === 'push'}
					<p class="py-3 text-sm text-gray-400">No setup needed. Each person turns it on for each of their devices in their Profile. On iPhone, Flixarr has to be added to the home screen first (iOS 16.4 or newer). Like email, it goes to the people involved. Send test goes to your own devices.</p>
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
			</div>

			<div>
				{@render heading('Events')}
				<div class="card divide-y divide-gray-700/40 px-4">
					{#each info.events as k (k)}
						<div class="py-3"><Toggle checked={form.events.includes(k)} label={EVENTS[k]?.[0] ?? k} about={EVENTS[k]?.[1]} disabled={SOON.includes(k)} onchange={(on) => setEvent(k, on)} /></div>
					{/each}
				</div>
				<p class="mt-2 text-xs text-gray-500">Which events send through this channel.</p>
			</div>
		</div>
		{#snippet footer()}
			{#if editing}<button class="btn-ghost mr-auto text-red-300" onclick={() => { open = false; confirmDelete = true; }}>Delete</button>{/if}
			<button class="btn-ghost" onclick={() => (open = false)}>Cancel</button>
			<button class="btn px-6" disabled={!!busy} onclick={save}>{busy === 'save' ? 'Saving…' : editing ? 'Save' : 'Add channel'}</button>
		{/snippet}
	</Modal>

	<Modal bind:open={confirmDelete} title="Delete {form.name}?" sub="This channel and its settings are deleted from Flixarr.">
		<p class="p-5 text-sm text-gray-300">Nothing is sent through it anymore. Other {name} channels keep working.</p>
		{#snippet footer()}
			<button class="btn-ghost" onclick={() => { confirmDelete = false; open = true; }}>Cancel</button>
			<button class="btn px-6" disabled={!!busy} onclick={remove}>{busy === 'delete' ? 'Deleting…' : 'Delete'}</button>
		{/snippet}
	</Modal>
{/if}
