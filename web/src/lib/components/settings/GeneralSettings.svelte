<script lang="ts">
	// Settings → General: whether requests wait for an admin, and whether new Plex users can sign in.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Toggle from '#lib/components/Toggle.svelte';

	let approval = $state<boolean | null>(null);
	let signIn = $state<boolean | null>(null);
	$effect(() => {
		api<{ approvalRequired: boolean }>('/settings/requests').then((d) => (approval = d.approvalRequired));
		api<{ plexSignIn: boolean }>('/settings/users').then((d) => (signIn = d.plexSignIn));
	});

	async function setSignIn(on: boolean) {
		try {
			signIn = (await api<{ plexSignIn: boolean }>('/settings/users', { method: 'PUT', body: JSON.stringify({ plexSignIn: on }) })).plexSignIn;
			notify(on ? 'Anyone your Plex server is shared with can sign in.' : 'Only people on the Users page can sign in.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}

	async function set(on: boolean) {
		try {
			approval = (await api<{ approvalRequired: boolean }>('/settings/requests', { method: 'PUT', body: JSON.stringify({ approvalRequired: on }) })).approvalRequired;
			notify(on ? 'Requests now wait for an admin to approve them.' : 'Requests are sent straight to Radarr and Sonarr.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}
</script>

{#if approval !== null && signIn !== null}
	<div>
		<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Requests</div>
		<div class="card px-4">
			<div class="py-3">
				<Toggle checked={approval} label="Require approval" about="Off: everyone's requests go straight to Radarr and Sonarr. On: requests wait until someone who manages requests approves them on the Requests page. People with Auto-approve (admins and Trusted users) skip the wait." onchange={set} />
			</div>
		</div>
	</div>
	<div>
		<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Users</div>
		<div class="card px-4">
			<div class="py-3">
				<Toggle checked={signIn} label="Let new Plex users sign in" about="On: anyone your Plex server is shared with can sign in, and starts as a User. Off: only people already on the Users page can sign in." onchange={setSignIn} />
			</div>
		</div>
	</div>
	<p class="text-xs text-gray-500">More general settings (app name, address, region and language) come with the rest of Settings.</p>
{/if}
