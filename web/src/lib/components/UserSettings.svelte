<script lang="ts">
	// Someone's settings, in a window on their profile (#160), for people who manage users: role and switches,
	// request limits, and for admins a new password and Remove. Without admin they can't change admins or the
	// owner (the server checks too).
	import { goto } from '$app/navigation';
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import type { User, UserProfile } from '#lib/types.ts';
	import Icon from './Icon.svelte';
	import Modal from './Modal.svelte';
	import Tooltip from './Tooltip.svelte';
	import Toggle from './Toggle.svelte';
	import Select from './Select.svelte';
	import { ROLES, MANUAL_PERMS, roleLabel, PERM_GROUPS, NO_APPROVALS, makePassword } from '#lib/roles.ts';

	let { info = $bindable(), open: settingsOpen = $bindable(false) }: { info: UserProfile; open?: boolean } = $props();
	// Start from what's saved each time the window opens.
	$effect(() => {
		if (settingsOpen) reset();
	});

	let perms = $state<string[]>([]);
	let movieLimit = $state(0);
	let seasonLimit = $state(0);
	let saving = $state(false);
	let removeOpen = $state(false);
	let newPassword = $state('');
	function closeSettings() {
		reset();
		settingsOpen = false;
	}
	async function saveAndClose() {
		await save();
		if (!dirty) settingsOpen = false;
	}

	function reset() {
		perms = [...info.user.perms];
		movieLimit = info.user.movieLimit;
		seasonLimit = info.user.seasonLimit;
	}

	const u = $derived(info.user);
	const can = $derived(!!info.canChange);
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
			notify(e instanceof Error ? e.message : 'Unable to save these settings', 'orange');
		} finally {
			saving = false;
		}
	}
	async function remove() {
		removeOpen = settingsOpen = false;
		try {
			await api(`/users/${u!.id}`, { method: 'DELETE' });
			notify(`Removed ${u!.username}.`);
			goto('/users');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Unable to remove this user', 'orange');
		}
	}
	async function setPassword() {
		saving = true;
		try {
			await api(`/users/${u!.id}/password`, { method: 'PUT', body: JSON.stringify({ password: newPassword }) });
			notify(`New password set for ${u!.username}. They're signed out everywhere.`);
			newPassword = '';
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Unable to set the password', 'orange');
		} finally {
			saving = false;
		}
	}
	const head = 'mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500';
</script>

<!-- Their settings live in a window, not spread down the profile (#160). -->
<Modal bind:open={settingsOpen} wide title="{u?.username ?? ''}'s settings" sub={u ? roleLabel(u) : ''}>
	{#if u}
		<div class="p-5">
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
					<!-- Grouped by what they're for (#158). Admin is last in Managing; while it's on, the rest show on. -->
					{#each PERM_GROUPS as [group, list] (group)}
						<section>
							<div class={head}>{group}</div>
							<div class="card divide-y divide-gray-700/40 px-4">
								{#each list.filter(([k]) => info!.admin || k !== 'admin') as [k, label, about] (k)}
									{@const idle = k === 'autoApprove' && !info.approvalRequired}
									{@const self = k === 'admin'}
									<div class="py-3 {idle ? 'opacity-60' : ''}">
										<Toggle checked={(!self && isAdmin) || perms.includes(k)} {label} about={idle ? NO_APPROVALS : about}
											disabled={!can || idle || (self ? u.owner || u.id === session.me?.id : isAdmin)} onchange={(on) => setPerm(k, on)} />
									</div>
								{/each}
							</div>
						</section>
					{/each}
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
								<span class="shrink-0"><Tooltip text="New password"><button class="btn-ghost h-full px-3" aria-label="New password" onclick={() => (newPassword = makePassword())}><Icon name="refresh" class="size-4" /></button></Tooltip></span>
								<button class="btn shrink-0 px-4 text-sm" disabled={saving || newPassword.length < 8} onclick={setPassword}>Set</button>
							</div>
							<p class="mt-1.5 text-xs text-gray-500">Setting a new password signs them out everywhere.</p>
						</section>
					{/if}
				</div>
			</div>
		</div>
	{/if}
	{#snippet footer()}
		{#if info?.admin && u && !u.owner && u.id !== session.me?.id}
			<button class="btn-ghost mr-auto text-red-300" onclick={() => (removeOpen = true)}><Icon name="trash" class="size-4" />Remove</button>
		{/if}
		<button class="btn-ghost" onclick={closeSettings}>{can ? 'Cancel' : 'Close'}</button>
		{#if can}<button class="btn px-6" disabled={saving || !dirty} onclick={saveAndClose}>{saving ? 'Saving…' : 'Save'}</button>{/if}
	{/snippet}
</Modal>

<Modal bind:open={removeOpen} title="Remove {u?.username}?">
	<p class="p-5 text-sm text-gray-300">They're signed out and their requests are removed from Flixarr. Nothing changes in Plex, Radarr or Sonarr. If your Plex server is still shared with them, they can sign in again as a new user.</p>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (removeOpen = false)}>Keep</button>
		<button class="btn px-6" onclick={remove}>Remove</button>
	{/snippet}
</Modal>
