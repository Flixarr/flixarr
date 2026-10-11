<script lang="ts">
	// "Sign in with Plex": opens plex.tv in a window, then polls until the PIN is approved.
	import { api } from '#lib/api.ts';
	import type { User } from '#lib/types.ts';
	let { onsignedin, label = 'Sign in with Plex' }: { onsignedin: (u: User) => void; label?: string } = $props();
	let busy = $state(false);
	let error = $state('');

	async function start() {
		error = '';
		busy = true;
		// Open the window right away (inside the click) so browsers don't block it.
		const win = window.open('', 'flixarr-plex', 'width=600,height=720');
		try {
			const pin = await api<{ id: number; authUrl: string }>('/auth/plex/pin', { method: 'POST' });
			if (win) win.location.href = pin.authUrl;
			else location.href = pin.authUrl;
			const until = Date.now() + 5 * 60 * 1000;
			while (Date.now() < until) {
				await new Promise((r) => setTimeout(r, 1500));
				const res = await api<{ done: boolean; user?: User }>('/auth/plex/check', { method: 'POST', body: JSON.stringify({ id: pin.id }) });
				if (res.done && res.user) {
					win?.close();
					onsignedin(res.user);
					return;
				}
			}
			error = 'Sign-in timed out. Try again.';
		} catch (e) {
			win?.close();
			error = e instanceof Error ? e.message : 'Sign-in failed.';
		} finally {
			busy = false;
		}
	}
</script>

<button class="btn w-full py-2.5" onclick={start} disabled={busy}>{busy ? 'Waiting for Plex…' : label}</button>
{#if error}<p class="mt-3 text-center text-sm text-red-400">{error}</p>{/if}
