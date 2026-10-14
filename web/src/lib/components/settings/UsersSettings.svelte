<script lang="ts">
	// Settings → Users: who can sign in and what new people start as. Each person's own switches are on
	// the Users page.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Toggle from '#lib/components/Toggle.svelte';
	import Icon from '#lib/components/Icon.svelte';

	type Info = { plexSignIn: boolean; defaultRole: 'User' | 'Trusted'; signIn: 'both' | 'plex' | 'local' };
	const WAYS = [['both', 'Both'], ['plex', 'Plex'], ['local', 'Username']] as const;
	const SAID = { both: 'People can sign in with Plex or a username.', plex: 'People sign in with Plex only.', local: 'People sign in with a username only. Admins can still use Plex.' };
	let info = $state<Info | null>(null);
	$effect(() => {
		api<Info>('/settings/users').then((d) => (info = d));
	});
	async function save(change: Partial<Info>, msg: string) {
		try {
			info = await api<Info>('/settings/users', { method: 'PUT', body: JSON.stringify({ ...info, ...change }) });
			notify(msg);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}
</script>

{#if info}
	<div class="card divide-y divide-gray-700/40 px-4">
		<div class="flex flex-wrap items-center justify-between gap-3 py-3">
			<span><span class="block text-sm">Sign in with</span><span class="block text-xs text-gray-500">Plex, a username and password (local accounts, added on the Users page), or both. With Username only, admins can still sign in with Plex.</span></span>
			<div class="inline-flex gap-1 rounded-lg border border-gray-700/60 bg-gray-800/70 p-0.5 text-sm" role="radiogroup" aria-label="Sign in with">
				{#each WAYS as [v, label] (v)}
					<button class="rounded-md px-3 py-1 {info.signIn === v ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'}" role="radio" aria-checked={info.signIn === v} onclick={() => save({ signIn: v }, SAID[v])}>{label}</button>
				{/each}
			</div>
		</div>
		<div class="py-3 {info.signIn === 'local' ? 'opacity-50' : ''}">
			<Toggle checked={info.plexSignIn} label="Let new Plex users sign in" about="On: anyone your Plex server is shared with can sign in. Off: only people already on the Users page can." onchange={(on) => save({ plexSignIn: on }, on ? 'Anyone your Plex server is shared with can sign in.' : 'Only people on the Users page can sign in.')} />
		</div>
		<div class="flex flex-wrap items-center justify-between gap-3 py-3">
			<span><span class="block text-sm">New people start as</span><span class="block text-xs text-gray-500">User: requests wait for approval when it's on. Trusted: requests go straight through, and they can pick the quality.</span></span>
			<div class="inline-flex gap-1 rounded-lg border border-gray-700/60 bg-gray-800/70 p-0.5 text-sm" role="radiogroup" aria-label="New people start as">
				{#each ['User', 'Trusted'] as r (r)}
					<button class="rounded-md px-3 py-1 {info.defaultRole === r ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'}" role="radio" aria-checked={info.defaultRole === r} onclick={() => save({ defaultRole: r as Info['defaultRole'] }, `New people start as ${r}.`)}>{r}</button>
				{/each}
			</div>
		</div>
	</div>
	<a href="/users" class="card flex items-center gap-3 px-4 py-3 text-sm transition hover:ring-1 hover:ring-primary/50"><Icon name="users" class="size-4 text-gray-400" /><span class="flex-1">People, roles and permissions, request limits, Import Plex users</span><Icon name="chevron" class="size-4 text-gray-500" /></a>
{/if}
