<script lang="ts">
	// Users (admins): everyone who can sign in, their role and permission switches, and importing the
	// people your Plex server is shared with. A role is a preset; change any of its switches and it becomes
	// Custom. Admin and Plex Watchlist requests are turned on by hand and never change the role (#146).
	import { api, fmtDate } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { session, can } from '#lib/session.svelte.ts';
	import { goto } from '$app/navigation';
	import type { User } from '#lib/types.ts';
	import Avatar from '#lib/components/Avatar.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import Toggle from '#lib/components/Toggle.svelte';
	import Select from '#lib/components/Select.svelte';
	import { ROLES, MANUAL_PERMS, ROLE_TONE, roleLabel } from '#lib/roles.ts';

	type Row = User & { requests: number };
	type Friend = { plexId: number; username: string; email: string; avatar: string; added: boolean };

	const PERMS: [string, string, string][] = [
		['manageRequests', 'Manage requests', "Approve, decline, retry and remove anyone's requests."],
		['autoApprove', 'Auto-approve', 'Their requests skip approval.'],
		['requestMovies', 'Request movies', ''],
		['requestTv', 'Request series', ''],
		['advanced', 'Choose quality', 'Pick a quality other than the default when requesting.'],
		['reportIssues', 'Report issues', 'Report problems on titles in the library.'],
		['manageIssues', 'Manage issues', "See, resolve and remove anyone's issues."],
		['redownload', 'Fix and redownload', 'Fix missing or bad episodes and movies: search again, or delete the file, block its release and download a new copy. Also needed to delete files from the admin panel.']
	];
	// Turned on by hand, never by a role.
	const MANUAL: [string, string, string][] = [
		['admin', 'Admin', 'Full access to everything, including Settings and Users.'],
		['watchlist', 'Plex Watchlist requests', 'Can switch on requests from their Plex Watchlist (in their Profile).']
	];

	let users = $state<Row[] | null>(null);
	let userQuery = $state('');
	let presets = $state<Record<string, string[]>>({});
	let editing = $state<Row | null>(null);
	let perms = $state<string[]>([]);
	let movieLimit = $state(0);
	let seasonLimit = $state(0);
	let editOpen = $state(false);
	let removeOpen = $state(false);
	let saving = $state(false);
	let importOpen = $state(false);
	let friends = $state<Friend[] | null>(null);
	let picked = $state<number[]>([]);
	let importError = $state('');

	$effect(() => {
		if (session.me && !can('admin')) goto('/', { replaceState: true });
	});
	async function load() {
		try {
			const d = await api<{ users: Row[]; roles: Record<string, string[]> }>('/users');
			users = d.users;
			presets = d.roles;
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not load users.', 'orange');
		}
	}
	$effect(() => {
		document.title = 'Flixarr · Users';
		if (can('admin')) load();
	});

	// The role a set of switches matches, the manual ones aside (same rule as the server).
	function roleOf(p: string[]) {
		const rest = p.filter((x) => !MANUAL_PERMS.includes(x));
		for (const r of ROLES) if (presets[r]?.length === rest.length && presets[r].every((x) => rest.includes(x))) return r;
		return 'Custom';
	}
	// Picking a role sets its switches and keeps the manual ones as they are.
	const pickRole = (r: string) => (perms = [...presets[r], ...perms.filter((x) => MANUAL_PERMS.includes(x))]);
	const role = $derived(roleOf(perms));
	// Admin turns everything on; the rest of the switches show as on and locked.
	const isAdmin = $derived(perms.includes('admin'));

	function edit(u: Row) {
		editing = u;
		perms = [...u.perms];
		movieLimit = u.movieLimit;
		seasonLimit = u.seasonLimit;
		editOpen = true;
	}
	function setPerm(k: string, on: boolean) {
		perms = on ? [...perms, k] : perms.filter((x) => x !== k);
	}
	async function save() {
		saving = true;
		try {
			const u = await api<User>(`/users/${editing!.id}`, { method: 'PUT', body: JSON.stringify({ perms, movieLimit, seasonLimit }) });
			users = users!.map((x) => (x.id === u.id ? { ...x, ...u } : x));
			editOpen = false;
			notify(`Saved ${u.username}.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		} finally {
			saving = false;
		}
	}
	async function remove() {
		const u = editing!;
		removeOpen = editOpen = false;
		try {
			await api(`/users/${u.id}`, { method: 'DELETE' });
			users = users!.filter((x) => x.id !== u.id);
			notify(`Removed ${u.username}.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not remove.', 'orange');
		}
	}

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

	// Local accounts: username and password, for people without Plex.
	let addOpen = $state(false);
	let local = $state({ username: '', email: '', role: 'User', password: '' });
	let newPassword = $state('');
	function makePassword() {
		const words = crypto.getRandomValues(new Uint32Array(4));
		return Array.from(words, (n) => n.toString(36).slice(0, 5)).join('-');
	}
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
	async function setPassword() {
		saving = true;
		try {
			await api(`/users/${editing!.id}/password`, { method: 'PUT', body: JSON.stringify({ password: newPassword }) });
			notify(`New password set for ${editing!.username}. They're signed out everywhere.`);
			newPassword = '';
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not set it.', 'orange');
		} finally {
			saving = false;
		}
	}
	const field = 'mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60';
</script>

<main class="mx-auto max-w-[1200px] px-4 pb-28 pt-20 laptop:px-10 laptop:pt-24">
	<div class="flex flex-wrap items-end justify-between gap-3">
		<div>
			<h1 class="text-2xl font-bold laptop:text-3xl">Users</h1>
			<p class="text-sm text-gray-400">{users ? `${users.length} ${users.length === 1 ? 'person' : 'people'} can sign in to Flixarr.` : ''}</p>
		</div>
		<div class="flex gap-2">
			<button class="btn-ghost text-sm" onclick={openAdd}><Icon name="plus" class="size-4" />Add local user</button>
			<button class="btn-ghost text-sm" onclick={openImport}><Icon name="refresh" class="size-4" />Import Plex users</button>
		</div>
	</div>

	{#if !users}
		<p class="mt-8 text-sm text-gray-400">Loading…</p>
	{:else}
		{#if users.length > 6}
			<input class="mt-5 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60 tablet:w-72" placeholder="Search people" bind:value={userQuery} />
		{/if}
		<div class="mt-3 space-y-2">
			{#each users.filter((u) => !userQuery.trim() || u.username.toLowerCase().includes(userQuery.trim().toLowerCase()) || (u.email ?? '').toLowerCase().includes(userQuery.trim().toLowerCase())) as u (u.id)}
				<button class="card flex w-full items-center gap-3 p-3 text-left transition hover:ring-1 hover:ring-primary/50" onclick={() => edit(u)}>
					<Avatar user={u} class="size-10 text-sm" />
					<div class="min-w-0 flex-1">
						<div class="font-medium">{u.username}{#if u.id === session.me?.id}<span class="ml-2 text-xs font-normal text-gray-500">You</span>{/if}</div>
						<div class="truncate text-xs text-gray-400">{u.email || (u.local ? 'Local account' : 'Plex account')}{u.local && u.email ? ' · Local account' : ''}{u.owner ? ' · Owner' : ''} · joined {fmtDate(u.createdAt, { month: 'short', day: 'numeric', year: 'numeric' })}</div>
					</div>
					<div class="hidden text-right text-xs text-gray-400 tablet:block">{u.requests} {u.requests === 1 ? 'request' : 'requests'}</div>
					{#if u.perms.includes('watchlist') && !u.perms.includes('admin')}<span class="hidden rounded-md bg-gray-800 px-1.5 py-0.5 text-[11px] text-gray-400 ring-1 ring-gray-700/60 tablet:inline">Watchlist</span>{/if}
					<span class="rounded-md px-1.5 py-0.5 text-[11px] ring-1 {ROLE_TONE[roleLabel(u)] ?? ROLE_TONE.User}">{roleLabel(u)}</span>
					<Icon name="chevron" class="size-4 text-gray-500" />
				</button>
			{/each}
		</div>
	{/if}
</main>

<Modal bind:open={editOpen} title={editing?.username ?? ''} sub={editing?.email || (editing?.local ? 'Local account' : 'Plex account')}>
	{#if editing}
		<div class="space-y-5 p-5">
			<div class="space-y-2">
				<div class="text-sm font-medium">Role</div>
				<div class="grid grid-cols-4 gap-1 rounded-xl border border-gray-700/60 p-1 text-sm {isAdmin ? 'opacity-50' : ''}" role="radiogroup" aria-label="Role">
					{#each [...ROLES, 'Custom'] as r (r)}
						<button class="rounded-lg px-2 py-1.5 {r === role ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'} {r === 'Custom' ? 'pointer-events-none' : ''}"
							role="radio" aria-checked={r === role} disabled={isAdmin} onclick={() => r !== 'Custom' && pickRole(r)}>{r}</button>
					{/each}
				</div>
				<p class="text-xs text-gray-500">{editing.owner ? 'The owner always stays an admin.' : isAdmin ? 'Admins can do everything. Their role counts again if Admin is turned off.' : 'A role is a preset. Change any of its switches and the role becomes Custom.'}</p>
			</div>
			{#snippet limitPicker(id: string, label: string, unit: string, steps: number[], value: number, set: (n: number) => void)}
				<div>
					<label for={id} class="block text-xs text-gray-400">{label}</label>
					<Select {id} {value} disabled={isAdmin} options={[{ value: 0, label: 'Unlimited' }, ...steps.map((n) => ({ value: n, label: `${n} ${unit}${n === 1 ? '' : 's'} a week` }))]} onchange={set} />
				</div>
			{/snippet}
			<div class="divide-y divide-gray-700/40 rounded-xl border border-gray-700/50 bg-gray-800/30 px-4">
				{#each PERMS as [k, label, about] (k)}
					<div class="py-3">
						<Toggle checked={isAdmin || perms.includes(k)} {label} {about} disabled={isAdmin} onchange={(on) => setPerm(k, on)} />
					</div>
				{/each}
			</div>
			<div class="space-y-2">
				<div class="text-sm font-medium">Turned on by hand</div>
				<div class="divide-y divide-gray-700/40 rounded-xl border border-gray-700/50 bg-gray-800/30 px-4">
					{#each MANUAL as [k, label, about] (k)}
						<div class="py-3">
							<Toggle checked={(k === 'watchlist' && isAdmin) || perms.includes(k)} {label} {about} disabled={(k === 'admin' && editing.owner) || (k === 'watchlist' && isAdmin)} onchange={(on) => setPerm(k, on)} />
						</div>
					{/each}
				</div>
				<p class="text-xs text-gray-500">Never part of a role: picking a role leaves these as they are.</p>
			</div>
			<div class="space-y-2">
				<div class="text-sm font-medium">Request limits</div>
				<div class="grid grid-cols-2 gap-3">
					{@render limitPicker('lim-movies', 'Movies', 'movie', [1, 3, 5, 10, 20], movieLimit, (n) => (movieLimit = n))}
					{@render limitPicker('lim-seasons', 'Series seasons', 'season', [1, 3, 5, 10, 20], seasonLimit, (n) => (seasonLimit = n))}
				</div>
				<p class="text-xs text-gray-500">{isAdmin ? 'Admins have no limits.' : 'Counted over the last 7 days. Declined and failed requests are not counted.'}</p>
			</div>
			{#if editing.local}
				<div class="space-y-2">
					<div class="text-sm font-medium">Password</div>
					<div class="flex gap-2">
						<input class="{field} mt-0" type="text" autocomplete="new-password" placeholder="New password (8 or more characters)" bind:value={newPassword} />
						<button class="btn-ghost shrink-0 text-sm" onclick={() => (newPassword = makePassword())}>Make one</button>
						<button class="btn shrink-0 px-4 text-sm" disabled={saving || newPassword.length < 8} onclick={setPassword}>Set</button>
					</div>
					<p class="text-xs text-gray-500">Setting a new password signs them out everywhere.</p>
				</div>
			{/if}
		</div>
	{/if}
	{#snippet footer()}
		{#if editing && !editing.owner && editing.id !== session.me?.id}
			<button class="btn-ghost mr-auto text-red-300" onclick={() => (removeOpen = true)}>Remove</button>
		{/if}
		<button class="btn-ghost" onclick={() => (editOpen = false)}>Cancel</button>
		<button class="btn px-6" disabled={saving} onclick={save}>{saving ? 'Saving…' : 'Save'}</button>
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
			<p class="mt-1 text-xs text-gray-500">Change their switches afterwards, like anyone else's.</p>
		</div>
		<div>
			<label for="lu-pass" class="block text-sm">Password</label>
			<div class="flex gap-2"><input id="lu-pass" type="text" autocomplete="new-password" class={field} bind:value={local.password} /><button class="btn-ghost mt-1 shrink-0 text-sm" onclick={() => (local.password = makePassword())}>Make one</button></div>
			<p class="mt-1 text-xs text-gray-500">8 or more characters. Give it to them; they can change it in their Profile.</p>
		</div>
	</div>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (addOpen = false)}>Cancel</button>
		<button class="btn px-6" disabled={saving || !local.username.trim() || local.password.length < 8} onclick={addLocal}>{saving ? 'Adding…' : 'Add'}</button>
	{/snippet}
</Modal>

<Modal bind:open={removeOpen} title="Remove {editing?.username}?">
	<p class="p-5 text-sm text-gray-300">They're signed out and their requests are removed from Flixarr. Nothing changes in Plex, Radarr or Sonarr. If your Plex server is still shared with them, they can sign in again as a new user.</p>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (removeOpen = false)}>Keep</button>
		<button class="btn px-6" onclick={remove}>Remove</button>
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
