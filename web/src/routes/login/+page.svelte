<script lang="ts">
	import { goto } from '$app/navigation';
	import PlexSignIn from '#lib/components/PlexSignIn.svelte';
	import { session } from '#lib/session.svelte.ts';
	import type { User } from '#lib/types.ts';
	import { api } from '#lib/api.ts';

	// Local accounts (no Plex): the username and password an admin gave them.
	let username = $state('');
	let password = $state('');
	let busy = $state(false);
	let error = $state('');
	async function local(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = '';
		try {
			const d = await api<{ user: User }>('/auth/local', { method: 'POST', body: JSON.stringify({ username, password }) });
			done(d.user);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Could not sign in.';
		} finally {
			busy = false;
		}
	}
	// Settings → Users decides which ways show. Username only still lets an admin use Plex (behind a link).
	const mode = $derived(session.status?.signIn ?? 'both');
	let adminPlex = $state(false);
	const field = 'mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60';

	function done(u: User) {
		session.me = u;
		goto('/', { replaceState: true });
	}
</script>

<div class="grid min-h-dvh place-items-center px-4 py-12">
	<div class="w-full max-w-sm text-center">
		<span class="text-logo text-6xl">Flixarr</span>
		<div class="card mt-8 space-y-5 p-6 text-left">
			<div class="text-center"><h1 class="text-xl font-semibold">Welcome back</h1><p class="mt-1 text-sm text-gray-400">Sign in to request movies and series.</p></div>
			{#if mode !== 'local' || adminPlex}<PlexSignIn onsignedin={done} />{/if}
			{#if mode === 'both'}<div class="flex items-center gap-3 text-xs text-gray-500"><span class="h-px flex-1 bg-gray-700/60"></span>or with a username<span class="h-px flex-1 bg-gray-700/60"></span></div>{/if}
			{#if mode !== 'plex'}<form class="space-y-3" onsubmit={local}>
				<div><label for="li-user" class="block text-sm">Username</label><input id="li-user" class={field} autocomplete="username" autocapitalize="off" bind:value={username} /></div>
				<div><label for="li-pass" class="block text-sm">Password</label><input id="li-pass" type="password" class={field} autocomplete="current-password" bind:value={password} /></div>
				{#if error}<p class="text-sm text-red-300">{error}</p>{/if}
				<button class="btn-ghost w-full justify-center" disabled={busy || !username.trim() || !password}>{busy ? 'Signing in…' : 'Sign in'}</button>
			</form>{/if}
			{#if mode === 'local' && !adminPlex}<button class="mx-auto block text-xs text-gray-500 hover:text-gray-300" onclick={() => (adminPlex = true)}>Admin? Sign in with Plex</button>{/if}
		</div>
	</div>
</div>
