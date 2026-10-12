<script lang="ts">
	// First run: the first person to sign in with Plex becomes the admin.
	import { goto } from '$app/navigation';
	import PlexSignIn from '#lib/components/PlexSignIn.svelte';
	import { session, loadSession } from '#lib/session.svelte.ts';
	import type { User } from '#lib/types.ts';

	async function done(u: User) {
		await loadSession();
		session.me = u;
		goto('/welcome', { replaceState: true }); // the rest of setup: Plex server, Radarr/Sonarr
	}
</script>

<div class="grid min-h-dvh place-items-center px-4 py-12">
	<div class="w-full max-w-md text-center">
		<span class="text-logo text-6xl">Flixarr</span>
		<div class="card mt-8 space-y-4 p-6">
			<h1 class="text-xl font-semibold">Welcome to Flixarr!</h1>
			<p class="text-sm text-gray-400">First things first, connect your Plex account. It becomes the admin account for this Flixarr.</p>
			<PlexSignIn onsignedin={done} />
		</div>
		<p class="mt-6 text-xs text-gray-500">Connecting your Plex account lets Flixarr talk to your Plex Media Server. Your Plex password never reaches Flixarr.</p>
	</div>
</div>
