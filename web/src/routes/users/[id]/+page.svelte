<script lang="ts">
	// A person's profile (#149), opened from Users: who they are, their numbers and latest requests, and
	// below that whatever the viewer may change (role and switches, limits; for admins also password and
	// Remove). Admins and Managers only; Managers can't change admins or the owner (the server checks too).
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, ago, fmtDate } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import type { Card, User } from '#lib/types.ts';
	import Avatar from '#lib/components/Avatar.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import Toggle from '#lib/components/Toggle.svelte';
	import Select from '#lib/components/Select.svelte';
	import Row from '#lib/components/Row.svelte';
	import Poster from '#lib/components/Poster.svelte';
	import { ROLES, MANUAL_PERMS, ROLE_TONE, roleLabel, PERM_TEXT, MANUAL_TEXT, NO_APPROVALS, makePassword } from '#lib/roles.ts';

	type Stats = { requests: number; available: number; waiting: number; inProgress: number; openIssues: number };
	type Info = { user: User; stats: Stats; requests: Card[]; roles: Record<string, string[]>; admin: boolean; canChange: boolean; approvalRequired: boolean };
	let info = $state<Info | null>(null);
	let perms = $state<string[]>([]);
	let movieLimit = $state(0);
	let seasonLimit = $state(0);
	let saving = $state(false);
	let removeOpen = $state(false);
	let newPassword = $state('');

	$effect(() => {
		if (session.me && !session.me.managesUsers) goto('/', { replaceState: true });
	});
	async function load(id: string) {
		try {
			info = await api<Info>(`/users/${id}`);
			reset();
			document.title = `Flixarr · ${info.user.username}`;
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not load them.', 'orange');
			goto('/users', { replaceState: true });
		}
	}
	$effect(() => {
		const id = page.params.id;
		if (session.me?.managesUsers && id) load(id);
	});
	function reset() {
		perms = [...info!.user.perms];
		movieLimit = info!.user.movieLimit;
		seasonLimit = info!.user.seasonLimit;
	}

	const u = $derived(info?.user);
	const can = $derived(!!info?.canChange);
	const isAdmin = $derived(perms.includes('admin'));
	const same = (a: string[], b: string[]) => a.length === b.length && a.every((x) => b.includes(x));
	function roleOf(p: string[]) {
		const rest = p.filter((x) => !MANUAL_PERMS.includes(x));
		for (const r of ROLES) if (info && same(info.roles[r] ?? [], rest)) return r;
		return 'Custom';
	}
	const role = $derived(roleOf(perms));
	const dirty = $derived(!!u && (!same(perms, u.perms) || movieLimit !== u.movieLimit || seasonLimit !== u.seasonLimit));
	const pickRole = (r: string) => (perms = [...info!.roles[r], ...perms.filter((x) => MANUAL_PERMS.includes(x))]);
	const setPerm = (k: string, on: boolean) => (perms = on ? [...perms, k] : perms.filter((x) => x !== k));

	async function save() {
		saving = true;
		try {
			const saved = await api<User>(`/users/${u!.id}`, { method: 'PUT', body: JSON.stringify({ perms, movieLimit, seasonLimit }) });
			info = { ...info!, user: { ...info!.user, ...saved } };
			reset();
			notify(`Saved ${saved.username}.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		} finally {
			saving = false;
		}
	}
	async function remove() {
		removeOpen = false;
		try {
			await api(`/users/${u!.id}`, { method: 'DELETE' });
			notify(`Removed ${u!.username}.`);
			goto('/users');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not remove.', 'orange');
		}
	}
	async function setPassword() {
		saving = true;
		try {
			await api(`/users/${u!.id}/password`, { method: 'PUT', body: JSON.stringify({ password: newPassword }) });
			notify(`New password set for ${u!.username}. They're signed out everywhere.`);
			newPassword = '';
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not set it.', 'orange');
		} finally {
			saving = false;
		}
	}
	const head = 'mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500';
</script>

<main class="mx-auto max-w-[1200px] space-y-6 px-4 pb-32 pt-20 laptop:px-10 laptop:pt-24">
	<a href="/users" class="inline-flex items-center gap-1 text-sm text-gray-400 hover:text-white">← Users</a>
	{#if !info || !u}
		<p class="text-sm text-gray-400">Loading…</p>
	{:else}
		<section class="flex flex-wrap items-center gap-4">
			<Avatar user={u} class="size-16 text-2xl" />
			<div class="min-w-0 flex-1">
				<h1 class="text-2xl font-bold">{u.username}{#if u.id === session.me?.id}<span class="ml-2 text-sm font-normal text-gray-500">You</span>{/if}</h1>
				<div class="flex flex-wrap items-center gap-2 text-sm text-gray-400">
					<span class="rounded-md px-1.5 py-0.5 text-xs ring-1 {ROLE_TONE[roleLabel(u)] ?? ROLE_TONE.User}">{roleLabel(u)}</span>
					{#if u.perms.includes('watchlist') && !u.perms.includes('admin')}<span class="rounded-md bg-gray-800 px-1.5 py-0.5 text-xs text-gray-400 ring-1 ring-gray-700/60">Watchlist</span>{/if}
					<span>{[u.email, u.local ? 'Local account' : 'Plex account', u.owner ? 'Owner' : ''].filter(Boolean).join(' · ')}</span>
				</div>
				<div class="mt-0.5 text-xs text-gray-500">Joined {fmtDate(u.createdAt)} · {u.lastSeen ? `last seen ${ago(u.lastSeen)}` : 'not seen yet'}</div>
			</div>
		</section>

		<section class="grid grid-cols-2 gap-2 tablet:grid-cols-4">
			{#each [['Requests', info.stats.requests], ['Available', info.stats.available], ['In progress', info.stats.inProgress + info.stats.waiting], ['Open issues', info.stats.openIssues]] as [l, n] (l)}
				<div class="card px-4 py-3"><div class="text-2xl font-bold">{n}</div><div class="text-xs text-gray-400">{l}</div></div>
			{/each}
		</section>
		{#if info.requests.length}
			<div class="-mx-4 laptop:mx-0"><Row title="{u.username}'s requests" items={info.requests}>{#snippet item(c)}<Poster item={c} />{/snippet}</Row></div>
		{/if}

		<div class="grid gap-6 laptop:grid-cols-2">
			<div class="space-y-6">
				{#if !can}<p class="rounded-lg bg-gray-800/70 px-3 py-2 text-sm text-gray-300 ring-1 ring-gray-700/60">Only admins can change an admin.</p>{/if}
				<section>
					<div class={head}>Role</div>
					<div class="card p-1">
						<div class="grid grid-cols-4 gap-1 text-sm {isAdmin || !can ? 'opacity-50' : ''}" role="radiogroup" aria-label="Role">
							{#each [...ROLES, 'Custom'] as r (r)}
								<button class="rounded-lg px-2 py-1.5 {r === role ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'} {r === 'Custom' ? 'pointer-events-none' : ''}"
									role="radio" aria-checked={r === role} disabled={isAdmin || !can} onclick={() => r !== 'Custom' && pickRole(r)}>{r}</button>
							{/each}
						</div>
					</div>
					<p class="mt-1.5 text-xs text-gray-500">{u.owner ? 'The owner always stays an admin.' : isAdmin ? 'Admins can do everything. Their role counts again if Admin is turned off.' : 'A role is a preset. Change any of its switches and the role becomes Custom.'}</p>
				</section>
				<section>
					<div class={head}>Permissions</div>
					<div class="card divide-y divide-gray-700/40 px-4">
						{#each PERM_TEXT as [k, label, about] (k)}
							{@const idle = k === 'autoApprove' && !info.approvalRequired}
							<div class="py-3 {idle ? 'opacity-60' : ''}"><Toggle checked={isAdmin || perms.includes(k)} {label} about={idle ? NO_APPROVALS : about} disabled={isAdmin || !can || idle} onchange={(on) => setPerm(k, on)} /></div>
						{/each}
					</div>
				</section>
				<section>
					<div class={head}>Turned on by hand</div>
					<div class="card divide-y divide-gray-700/40 px-4">
						{#each MANUAL_TEXT.filter(([k]) => info!.admin || k !== 'admin') as [k, label, about] (k)}
							<div class="py-3"><Toggle checked={(k === 'watchlist' && isAdmin) || perms.includes(k)} {label} {about} disabled={!can || (k === 'admin' && (u.owner || u.id === session.me?.id)) || (k === 'watchlist' && isAdmin)} onchange={(on) => setPerm(k, on)} /></div>
						{/each}
					</div>
					<p class="mt-1.5 text-xs text-gray-500">Never part of a role: picking a role leaves these as they are.</p>
				</section>
			</div>
			<div class="space-y-6">
				<section>
					<div class={head}>Request limits</div>
					<div class="card grid grid-cols-2 gap-3 px-4 py-3">
						{#each [['lim-movies', 'Movies', 'movie', movieLimit, (n: number) => (movieLimit = n)], ['lim-seasons', 'Series seasons', 'season', seasonLimit, (n: number) => (seasonLimit = n)]] as const as [id, label, unit, value, set] (id)}
							<div>
								<label for={id} class="block text-xs text-gray-400">{label}</label>
								<Select {id} {value} disabled={isAdmin || !can} options={[{ value: 0, label: 'Unlimited' }, ...[1, 3, 5, 10, 20].map((n) => ({ value: n, label: `${n} ${unit}${n === 1 ? '' : 's'} a week` }))]} onchange={set} />
							</div>
						{/each}
					</div>
					<p class="mt-1.5 text-xs text-gray-500">{isAdmin ? 'Admins have no limits.' : 'Counted over the last 7 days. Declined and failed requests are not counted.'}</p>
				</section>
				{#if u.local && info.admin}
					<section>
						<div class={head}>Password</div>
						<div class="card flex gap-2 px-4 py-3">
							<input class="w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60" type="text" autocomplete="new-password" placeholder="New password (8 or more characters)" bind:value={newPassword} />
							<button class="btn-ghost shrink-0 text-sm" onclick={() => (newPassword = makePassword())}>Make one</button>
							<button class="btn shrink-0 px-4 text-sm" disabled={saving || newPassword.length < 8} onclick={setPassword}>Set</button>
						</div>
						<p class="mt-1.5 text-xs text-gray-500">Setting a new password signs them out everywhere.</p>
					</section>
				{/if}
				{#if info.admin && !u.owner && u.id !== session.me?.id}
					<section>
						<div class={head}>Remove</div>
						<div class="card flex items-center justify-between gap-4 px-4 py-3">
							<span class="text-sm text-gray-300">Signs them out and removes their requests from Flixarr. Nothing changes in Plex, Radarr or Sonarr.</span>
							<button class="btn-ghost shrink-0 text-red-300" onclick={() => (removeOpen = true)}><Icon name="trash" class="size-4" />Remove</button>
						</div>
					</section>
				{/if}
			</div>
		</div>
	{/if}
</main>

{#if dirty}
	<div class="fixed inset-x-0 bottom-[calc(4.5rem+env(safe-area-inset-bottom))] z-40 px-3 tablet:bottom-6">
		<div class="mx-auto flex max-w-xl items-center gap-2 rounded-2xl border border-gray-700/60 bg-gray-900/95 p-2.5 shadow-2xl backdrop-blur-xl">
			<span class="px-2 text-sm">Unsaved changes</span>
			<button class="btn-ghost ml-auto text-sm" onclick={reset}>Undo</button>
			<button class="btn px-6 text-sm" disabled={saving} onclick={save}>{saving ? 'Saving…' : 'Save'}</button>
		</div>
	</div>
{/if}

<Modal bind:open={removeOpen} title="Remove {u?.username}?">
	<p class="p-5 text-sm text-gray-300">They're signed out and their requests are removed from Flixarr. Nothing changes in Plex, Radarr or Sonarr. If your Plex server is still shared with them, they can sign in again as a new user.</p>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (removeOpen = false)}>Keep</button>
		<button class="btn px-6" onclick={remove}>Remove</button>
	{/snippet}
</Modal>
