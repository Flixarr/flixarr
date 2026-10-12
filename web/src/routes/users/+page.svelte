<script lang="ts">
	// Users (admins): everyone who can sign in, their role and permission switches, and importing the
	// people your Plex server is shared with. A role is a preset; change any switch and it becomes Custom.
	import { api, fmtDate } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { session, can } from '#lib/session.svelte.ts';
	import { goto } from '$app/navigation';
	import type { User } from '#lib/types.ts';
	import Avatar from '#lib/components/Avatar.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import Toggle from '#lib/components/Toggle.svelte';

	type Row = User & { requests: number };
	type Friend = { plexId: number; username: string; email: string; avatar: string; added: boolean };

	const PERMS: [string, string, string][] = [
		['admin', 'Admin', 'Full access to everything, including Settings and Users.'],
		['manageRequests', 'Manage requests', "Approve, decline, retry and remove anyone's requests."],
		['autoApprove', 'Auto-approve', 'Their requests skip approval.'],
		['requestMovies', 'Request movies', ''],
		['requestTv', 'Request series', ''],
		['advanced', 'Choose quality', 'Pick a quality other than the default when requesting.'],
		['viewRequests', 'View all requests', "See everyone's requests, not just their own."],
		['reportIssues', 'Report issues', 'Report problems on titles in the library.'],
		['manageIssues', 'Manage issues', "See, resolve and remove anyone's issues."],
		['watchlist', 'Plex Watchlist requests', 'Arrives with Watchlist requests.']
	];
	const ROLES = ['Admin', 'Trusted', 'User', 'Custom'];
	const ROLE_TONE: Record<string, string> = {
		Admin: 'bg-primary/15 text-primary-light ring-primary/40', Trusted: 'bg-green-500/15 text-green-400 ring-green-500/40',
		Custom: 'bg-purple-500/15 text-purple-300 ring-purple-500/40', User: 'bg-gray-800 text-gray-300 ring-gray-700/60'
	};

	let users = $state<Row[] | null>(null);
	let presets = $state<Record<string, string[]>>({});
	let editing = $state<Row | null>(null);
	let perms = $state<string[]>([]);
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
		load();
	});

	// The role a set of switches matches (same rule as the server).
	function roleOf(p: string[]) {
		if (p.includes('admin')) return 'Admin';
		for (const r of ['Trusted', 'User']) if (presets[r]?.length === p.length && presets[r].every((x) => p.includes(x))) return r;
		return 'Custom';
	}
	const role = $derived(roleOf(perms));
	// Admin turns everything on; the rest of the switches show as on and locked.
	const isAdmin = $derived(perms.includes('admin'));

	function edit(u: Row) {
		editing = u;
		perms = [...u.perms];
		editOpen = true;
	}
	function setPerm(k: string, on: boolean) {
		perms = on ? [...perms, k] : perms.filter((x) => x !== k);
	}
	async function save() {
		saving = true;
		try {
			const u = await api<User>(`/users/${editing!.id}`, { method: 'PUT', body: JSON.stringify({ perms: isAdmin ? presets.Admin : perms }) });
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
</script>

<main class="mx-auto max-w-[1200px] px-4 pb-28 pt-20 laptop:px-10 laptop:pt-24">
	<div class="flex flex-wrap items-end justify-between gap-3">
		<div>
			<h1 class="text-2xl font-bold laptop:text-3xl">Users</h1>
			<p class="text-sm text-gray-400">{users ? `${users.length} ${users.length === 1 ? 'person' : 'people'} can sign in to Flixarr.` : ''}</p>
		</div>
		<button class="btn-ghost text-sm" onclick={openImport}><Icon name="refresh" class="size-4" />Import Plex users</button>
	</div>

	{#if !users}
		<p class="mt-8 text-sm text-gray-400">Loading…</p>
	{:else}
		<div class="mt-5 space-y-2">
			{#each users as u (u.id)}
				<button class="card flex w-full items-center gap-3 p-3 text-left transition hover:ring-1 hover:ring-primary/50" onclick={() => edit(u)}>
					<Avatar user={u} class="size-10 text-sm" />
					<div class="min-w-0 flex-1">
						<div class="font-medium">{u.username}{#if u.id === session.me?.id}<span class="ml-2 text-xs font-normal text-gray-500">You</span>{/if}</div>
						<div class="truncate text-xs text-gray-400">{u.email || 'Plex account'}{u.owner ? ' · Owner' : ''} · joined {fmtDate(u.createdAt, { month: 'short', day: 'numeric', year: 'numeric' })}</div>
					</div>
					<div class="hidden text-right text-xs text-gray-400 tablet:block">{u.requests} {u.requests === 1 ? 'request' : 'requests'}</div>
					<span class="rounded-md px-1.5 py-0.5 text-[11px] ring-1 {ROLE_TONE[u.role] ?? ROLE_TONE.User}">{u.role}</span>
					<Icon name="chevron" class="size-4 text-gray-500" />
				</button>
			{/each}
		</div>
	{/if}
</main>

<Modal bind:open={editOpen} title={editing?.username ?? ''} sub={editing?.email || 'Plex account'}>
	{#if editing}
		<div class="space-y-5 p-5">
			<div class="space-y-2">
				<div class="text-sm font-medium">Role</div>
				<div class="grid grid-cols-4 gap-1 rounded-xl border border-gray-700/60 p-1 text-sm" role="radiogroup" aria-label="Role">
					{#each ROLES as r (r)}
						<button class="rounded-lg px-2 py-1.5 {r === role ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'} {r === 'Custom' ? 'pointer-events-none' : ''}"
							role="radio" aria-checked={r === role} disabled={editing.owner && r !== 'Admin'} onclick={() => r !== 'Custom' && (perms = [...presets[r]])}>{r}</button>
					{/each}
				</div>
				<p class="text-xs text-gray-500">{editing.owner ? 'The owner always stays an admin.' : 'A role is a preset. Change any switch below and the role becomes Custom.'}</p>
			</div>
			<div class="divide-y divide-gray-700/40 rounded-xl border border-gray-700/50 bg-gray-800/30 px-4">
				{#each PERMS as [k, label, about] (k)}
					<div class="py-3">
						<Toggle checked={isAdmin || perms.includes(k)} {label} {about} disabled={(k === 'admin' && editing.owner) || (k !== 'admin' && isAdmin)} onchange={(on) => setPerm(k, on)} />
					</div>
				{/each}
			</div>
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
