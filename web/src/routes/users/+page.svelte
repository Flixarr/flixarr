<script lang="ts">
	// Users (admins and Managers, #148): everyone who can sign in. Tapping someone opens their profile, where
	// they're changed (#149). Edit shows checkboxes and a bar to change many people at once (#147): Set role,
	// or Edit permissions where each switch is On, Off, or left Mixed (nobody changes). Adding local users
	// and importing Plex users stay with admins; the server skips anyone the viewer may not change.
	import { api, ago } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { goto } from '$app/navigation';
	import type { User } from '#lib/types.ts';
	import Avatar from '#lib/components/Avatar.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import Tooltip from '#lib/components/Tooltip.svelte';
	import { ROLES, ROLE_TONE, roleLabel, PERM_GROUPS, NO_APPROVALS, makePassword } from '#lib/roles.ts';

	type Row = User & { requests: number };
	type Friend = { plexId: number; username: string; email: string; avatar: string; added: boolean };
	const roleAbout = (r: string) => ({ User: 'Request and report issues', Trusted: approvals ? 'Plus choose quality and skip approval' : 'Plus choose quality', Manager: 'Plus manage requests, issues and users' })[r];

	let users = $state<Row[] | null>(null);
	let userQuery = $state('');
	let iAmAdmin = $state(false);
	let approvals = $state(true);
	let saving = $state(false);
	let importOpen = $state(false);
	let friends = $state<Friend[] | null>(null);
	let picked = $state<number[]>([]);
	let importError = $state('');

	$effect(() => {
		if (session.me && !session.me.managesUsers) goto('/', { replaceState: true });
	});
	async function load() {
		try {
			const d = await api<{ users: Row[]; admin: boolean; approvalRequired: boolean }>('/users');
			users = d.users;
			iAmAdmin = d.admin;
			approvals = d.approvalRequired;
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not load users.', 'orange');
		}
	}
	$effect(() => {
		document.title = 'Flixarr · Users';
		if (session.me?.managesUsers) load();
	});

	const q = $derived(userQuery.trim().toLowerCase());
	const shown = $derived((users ?? []).filter((u) => !q || u.username.toLowerCase().includes(q) || (u.email ?? '').toLowerCase().includes(q)));
	// Who the viewer may change: admins anyone; Managers anyone but admins and the owner.
	const changeable = (u: Row) => iAmAdmin || !(u.owner || u.perms.includes('admin'));

	// ---------- Edit: many at once (#147) ----------
	let editing = $state(false);
	let chosen = $state<number[]>([]);
	let roleMenu = $state(false);
	const able = $derived(shown.filter(changeable));
	const allChosen = $derived(able.length > 0 && able.every((u) => chosen.includes(u.id)));
	const who = $derived((users ?? []).filter((u) => chosen.includes(u.id)));
	function toggleEdit() {
		editing = !editing;
		chosen = [];
		roleMenu = false;
	}
	const choose = (id: number, on: boolean) => (chosen = on ? [...chosen, id] : chosen.filter((x) => x !== id));
	function chooseAll(on: boolean) {
		const ids = able.map((u) => u.id);
		chosen = on ? [...new Set([...chosen, ...ids])] : chosen.filter((x) => !ids.includes(x));
	}
	async function bulk(body: object) {
		saving = true;
		try {
			const r = await api<{ updated: number; skipped: { name: string; why: string }[] }>('/users/bulk', { method: 'POST', body: JSON.stringify({ ids: chosen, ...body }) });
			const n = (k: number) => `${k} ${k === 1 ? 'person' : 'people'}`;
			notify(`Updated ${n(r.updated)}.${r.skipped.length ? ` Skipped ${r.skipped.map((s) => `${s.name} (${s.why.toLowerCase()})`).join(', ')}.` : ''}`, r.skipped.length ? 'orange' : 'green');
			permsOpen = roleMenu = false;
			await load();
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		} finally {
			saving = false;
		}
	}
	// Edit permissions: each switch on, off, or (when the chosen people differ) keep.
	let permsOpen = $state(false);
	let setTo = $state<Record<string, 'on' | 'off' | 'keep'>>({});
	// Same groups and order as a profile (#158); only admins see the Admin switch.
	const groups = $derived(PERM_GROUPS.map(([g, list]) => [g, list.filter(([k]) => iAmAdmin || k !== 'admin')] as const));
	const rows = $derived(groups.flatMap(([, list]) => list));
	const have = (k: string) => who.filter((u) => u.perms.includes(k)).length;
	const startOf = (k: string) => (have(k) === who.length ? 'on' : have(k) === 0 ? 'off' : 'keep');
	function openPerms() {
		setTo = Object.fromEntries(rows.map(([k]) => [k, startOf(k)]));
		permsOpen = true;
		roleMenu = false;
	}
	function applyPerms() {
		const set: Record<string, boolean> = {};
		for (const [k, v] of Object.entries(setTo)) if (v !== 'keep' && v !== startOf(k)) set[k] = v === 'on';
		if (!Object.keys(set).length) return void (permsOpen = false);
		bulk({ set });
	}

	// ---------- Import Plex users, add local users (admins) ----------
	async function openImport() {
		importOpen = true;
		friends = null;
		importError = '';
		try {
			friends = await api<Friend[]>('/users/plex');
			picked = friends.filter((f) => !f.added).map((f) => f.plexId);
		} catch (e) {
			importError = e instanceof Error ? e.message : 'Could not reach Plex.';
		}
	}
	async function runImport() {
		saving = true;
		try {
			const { added } = await api<{ added: number }>('/users/plex', { method: 'POST', body: JSON.stringify({ plexIds: picked }) });
			importOpen = false;
			notify(added ? `Added ${added} ${added === 1 ? 'person' : 'people'}. They can sign in with Plex now.` : 'Nobody new to add.');
			load();
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not import.', 'orange');
		} finally {
			saving = false;
		}
	}
	const fresh = $derived(friends?.filter((f) => !f.added) ?? []);

	let addOpen = $state(false);
	let local = $state({ username: '', email: '', role: 'User', password: '' });
	function openAdd() {
		local = { username: '', email: '', role: 'User', password: makePassword() };
		addOpen = true;
	}
	async function addLocal() {
		saving = true;
		try {
			const u = await api<User>('/users/local', { method: 'POST', body: JSON.stringify(local) });
			addOpen = false;
			notify(`Added ${u.username}. Give them the username and password; they can change the password in their Profile.`);
			load();
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not add them.', 'orange');
		} finally {
			saving = false;
		}
	}
	const field = 'mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60';
</script>

{#snippet person(u: Row)}
	<Avatar user={u} class="size-10 text-sm" />
	<div class="min-w-0 flex-1">
		<div class="font-medium">{u.username}{#if u.id === session.me?.id}<span class="ml-2 text-xs font-normal text-gray-500">You</span>{/if}</div>
		<div class="truncate text-xs text-gray-400">{u.email || (u.local ? 'Local account' : 'Plex account')}{u.local && u.email ? ' · Local account' : ''}{u.owner ? ' · Owner' : ''} · {u.lastSeen ? `seen ${ago(u.lastSeen)}` : 'not seen yet'}</div>
	</div>
	<div class="hidden text-right text-xs text-gray-400 tablet:block">{u.requests} {u.requests === 1 ? 'request' : 'requests'}</div>
	{#if u.perms.includes('watchlist') && !u.perms.includes('admin')}<span class="hidden rounded-md bg-gray-800 px-1.5 py-0.5 text-[11px] text-gray-400 ring-1 ring-gray-700/60 tablet:inline">Watchlist</span>{/if}
	<span class="rounded-md px-1.5 py-0.5 text-[11px] ring-1 {ROLE_TONE[roleLabel(u)] ?? ROLE_TONE.User}">{roleLabel(u)}</span>
{/snippet}

<main class="mx-auto max-w-[1200px] px-4 pb-36 pt-20 laptop:px-10 laptop:pt-24">
	<div class="flex flex-wrap items-end justify-between gap-3">
		<div>
			<h1 class="text-2xl font-bold laptop:text-3xl">Users</h1>
			<p class="text-sm text-gray-400">{users ? `${users.length} ${users.length === 1 ? 'person' : 'people'} can sign in to Flixarr.` : ''}</p>
		</div>
		<div class="flex gap-2">
			{#if iAmAdmin && !editing}
				<button class="btn-ghost text-sm" onclick={openAdd}><Icon name="plus" class="size-4" /><span class="hidden tablet:inline">Add local user</span><span class="tablet:hidden">Add</span></button>
				<button class="btn-ghost text-sm" onclick={openImport}><Icon name="refresh" class="size-4" /><span class="hidden tablet:inline">Import Plex users</span><span class="tablet:hidden">Import</span></button>
			{/if}
			{#if users?.length}<button class="{editing ? 'btn' : 'btn-ghost'} text-sm" onclick={toggleEdit}>{editing ? 'Done' : 'Edit'}</button>{/if}
		</div>
	</div>

	{#if !users}
		<p class="mt-8 text-sm text-gray-400">Loading…</p>
	{:else}
		<div class="mt-5 flex items-center gap-3">
			{#if editing}
				<label class="flex shrink-0 cursor-pointer items-center gap-2 rounded-lg border border-gray-700/60 px-3 py-2 text-sm">
					<input type="checkbox" class="size-4 accent-[var(--color-primary)]" checked={allChosen} onchange={(e) => chooseAll((e.currentTarget as HTMLInputElement).checked)} />{q ? 'All matches' : 'All'}
				</label>
			{/if}
			{#if users.length > 6 || editing}
				<input class="w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60 tablet:w-72" placeholder="Search people" bind:value={userQuery} />
			{/if}
		</div>
		<div class="mt-3 space-y-2">
			{#each shown as u (u.id)}
				{#if editing}
					{@const on = chosen.includes(u.id)}
					{@const ok = changeable(u)}
					<label class="card flex w-full items-center gap-3 p-3 transition {ok ? 'cursor-pointer' : 'opacity-50'} {on ? 'bg-primary/5 ring-1 ring-primary/70' : ''}">
						<input type="checkbox" class="size-5 shrink-0 accent-[var(--color-primary)]" checked={on} disabled={!ok} onchange={(e) => choose(u.id, (e.currentTarget as HTMLInputElement).checked)} />
						{@render person(u)}
						{#if !ok}<span class="text-[11px] text-gray-500">Admins only</span>{/if}
					</label>
				{:else}
					<a href="/users/{u.id}" class="card flex w-full items-center gap-3 p-3 text-left transition hover:ring-1 hover:ring-primary/50">
						{@render person(u)}
						<Icon name="chevron" class="size-4 text-gray-500" />
					</a>
				{/if}
			{/each}
		</div>
	{/if}
</main>

{#if editing && chosen.length}
	<div class="fixed inset-x-0 bottom-[calc(4.5rem+env(safe-area-inset-bottom))] z-40 px-3 tablet:bottom-6">
		<div class="relative mx-auto flex max-w-2xl flex-wrap items-center gap-2 rounded-2xl border border-gray-700/60 bg-gray-900/95 p-2.5 shadow-2xl backdrop-blur-xl">
			<span class="px-2 text-sm font-medium">{chosen.length} selected</span>
			<!-- On phones the role menu spans the bar, so it never runs off the screen (#155). -->
			<div class="ml-auto tablet:relative">
				<button class="btn-ghost text-sm" disabled={saving} onclick={() => (roleMenu = !roleMenu)}>Set role</button>
				{#if roleMenu}
					<div class="absolute inset-x-0 bottom-full mb-2 overflow-hidden rounded-xl border border-gray-700/60 bg-gray-900 py-1.5 shadow-2xl tablet:inset-x-auto tablet:bottom-12 tablet:right-0 tablet:mb-0 tablet:w-64">
						{#each ROLES as r (r)}
							<button class="block w-full px-4 py-2 text-left text-sm hover:bg-white/5" onclick={() => bulk({ role: r })}><span class="block">{r}</span><span class="block text-xs text-gray-500">{roleAbout(r)}</span></button>
						{/each}
						<p class="border-t border-gray-700/40 px-4 pt-2 text-[11px] text-gray-500">Admin and Plex Watchlist requests stay as they are.</p>
					</div>
				{/if}
			</div>
			<button class="btn text-sm" disabled={saving} onclick={openPerms}>Edit permissions</button>
			<button class="rounded-lg p-2 text-gray-400 hover:bg-white/10" aria-label="Clear" onclick={() => ((chosen = []), (roleMenu = false))}><Icon name="close" class="size-4" /></button>
		</div>
	</div>
{/if}

<Modal bind:open={permsOpen} title="Edit permissions" sub="{who.length} {who.length === 1 ? 'person' : 'people'}: {who.slice(0, 3).map((u) => u.username).join(', ')}{who.length > 3 ? ` and ${who.length - 3} more` : ''}">
	<div class="space-y-4 p-5">
		<p class="text-xs text-gray-500">Mixed means some of them have it. Leave it on Mixed and nobody changes.</p>
		{#each groups as [group, list] (group)}
			<div>
				<div class="mb-1.5 text-xs font-semibold uppercase tracking-wide text-gray-500">{group}</div>
				<div class="divide-y divide-gray-700/40 rounded-xl border border-gray-700/50 bg-gray-800/30 px-4">
					{#each list as [k, label] (k)}
						{@const n = have(k)}
						{@const mixed = n > 0 && n < who.length}
						{@const idle = k === 'autoApprove' && !approvals}
						<div class="flex items-center justify-between gap-3 py-3 {idle ? 'pointer-events-none opacity-60' : ''}">
							<span class="min-w-0"><span class="block text-sm">{label}</span><span class="block text-xs text-gray-500">{idle ? NO_APPROVALS : mixed ? `${n} of ${who.length} have it` : n ? 'All of them have it' : 'None of them have it'}</span></span>
							<span class="flex shrink-0 gap-0.5 rounded-lg border border-gray-700/60 p-0.5 text-xs" role="radiogroup" aria-label={label}>
								{#each [['off', 'Off'], ...(mixed ? [['keep', 'Mixed']] : []), ['on', 'On']] as [v, t] (v)}
									<button role="radio" aria-checked={setTo[k] === v} class="rounded-md px-2.5 py-1 {setTo[k] === v ? (v === 'on' ? 'bg-primary text-white' : v === 'off' ? 'bg-gray-700 text-white' : 'bg-gray-800 text-gray-200 ring-1 ring-gray-600') : 'text-gray-400 hover:text-white'}" onclick={() => (setTo[k] = v as 'on' | 'off' | 'keep')}>{t}</button>
								{/each}
							</span>
						</div>
					{/each}
				</div>
			</div>
		{/each}
	</div>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (permsOpen = false)}>Cancel</button>
		<button class="btn px-6" disabled={saving} onclick={applyPerms}>{saving ? 'Saving…' : `Apply to ${who.length}`}</button>
	{/snippet}
</Modal>

<Modal bind:open={addOpen} title="Add local user" sub="For someone without a Plex account">
	<div class="space-y-4 p-5">
		<div><label for="lu-name" class="block text-sm">Username</label><input id="lu-name" class={field} autocomplete="off" bind:value={local.username} /></div>
		<div><label for="lu-email" class="block text-sm">Email <span class="text-gray-500">(optional, for notifications)</span></label><input id="lu-email" type="email" class={field} bind:value={local.email} /></div>
		<div>
			<div class="text-sm">Role</div>
			<div class="mt-1 grid grid-cols-2 gap-1 rounded-xl border border-gray-700/60 p-1 text-sm" role="radiogroup" aria-label="Role">
				{#each ['User', 'Trusted'] as r (r)}
					<button class="rounded-lg px-2 py-1.5 {local.role === r ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'}" role="radio" aria-checked={local.role === r} onclick={() => (local.role = r)}>{r}</button>
				{/each}
			</div>
			<p class="mt-1 text-xs text-gray-500">Change their switches afterwards on their profile, like anyone else's.</p>
		</div>
		<div>
			<label for="lu-pass" class="block text-sm">Password</label>
			<div class="flex gap-2"><input id="lu-pass" type="text" autocomplete="new-password" class={field} bind:value={local.password} /><span class="mt-1 shrink-0"><Tooltip text="New password"><button class="btn-ghost h-full px-3" aria-label="New password" onclick={() => (local.password = makePassword())}><Icon name="refresh" class="size-4" /></button></Tooltip></span></div>
			<p class="mt-1 text-xs text-gray-500">8 or more characters. Give it to them; they can change it in their Profile.</p>
		</div>
	</div>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (addOpen = false)}>Cancel</button>
		<button class="btn px-6" disabled={saving || !local.username.trim() || local.password.length < 8} onclick={addLocal}>{saving ? 'Adding…' : 'Add'}</button>
	{/snippet}
</Modal>

<Modal bind:open={importOpen} title="Import Plex users" sub="People your Plex server is shared with">
	<div class="p-5">
		{#if importError}
			<p class="rounded-lg bg-yellow-500/10 px-3 py-2 text-sm text-yellow-300 ring-1 ring-yellow-500/30">{importError}</p>
		{:else if !friends}
			<p class="text-sm text-gray-400">Asking Plex who your server is shared with…</p>
		{:else if !friends.length}
			<p class="text-sm text-gray-400">Your Plex server isn't shared with anyone yet.</p>
		{:else}
			<div class="max-h-[50dvh] divide-y divide-gray-700/40 overflow-y-auto rounded-lg border border-gray-700/50 bg-gray-900/40">
				{#each friends as f (f.plexId)}
					<label class="flex items-center gap-3 px-3 py-2 text-sm {f.added ? 'opacity-50' : 'cursor-pointer hover:bg-white/5'}">
						<input type="checkbox" class="accent-[var(--color-primary)]" disabled={f.added} checked={f.added || picked.includes(f.plexId)}
							onchange={(e) => (picked = (e.currentTarget as HTMLInputElement).checked ? [...picked, f.plexId] : picked.filter((x) => x !== f.plexId))} />
						<span class="min-w-0 flex-1"><span class="block truncate">{f.username}</span>{#if f.email}<span class="block truncate text-xs text-gray-500">{f.email}</span>{/if}</span>
						{#if f.added}<span class="text-xs text-gray-500">Already added</span>{/if}
					</label>
				{/each}
			</div>
			<p class="mt-3 text-xs text-gray-500">They're added as Users (request movies and series). Change anyone's role afterwards.</p>
		{/if}
	</div>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (importOpen = false)}>Cancel</button>
		<button class="btn px-6" disabled={saving || !picked.length || !fresh.length} onclick={runImport}>{saving ? 'Adding…' : picked.length ? `Add ${picked.length}` : 'Add'}</button>
	{/snippet}
</Modal>
