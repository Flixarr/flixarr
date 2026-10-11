<script lang="ts">
	import { goto } from '$app/navigation';
	import PlexSignIn from '#lib/components/PlexSignIn.svelte';
	import { session } from '#lib/session.svelte.ts';
	import type { User } from '#lib/types.ts';

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
			<PlexSignIn onsignedin={done} />
			<p class="text-center text-xs text-gray-500">Local accounts (username and password) come in a later update.</p>
		</div>
	</div>
</div>
