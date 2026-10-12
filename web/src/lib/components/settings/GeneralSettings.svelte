<script lang="ts">
	// Settings → General. For now: whether requests wait for an admin.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Toggle from '#lib/components/Toggle.svelte';

	let approval = $state<boolean | null>(null);
	$effect(() => {
		api<{ approvalRequired: boolean }>('/settings/requests').then((d) => (approval = d.approvalRequired));
	});

	async function set(on: boolean) {
		try {
			approval = (await api<{ approvalRequired: boolean }>('/settings/requests', { method: 'PUT', body: JSON.stringify({ approvalRequired: on }) })).approvalRequired;
			notify(on ? 'Requests now wait for an admin to approve them.' : 'Requests are sent straight to Radarr and Sonarr.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}
</script>

{#if approval !== null}
	<div>
		<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Requests</div>
		<div class="card px-4">
			<div class="py-3">
				<Toggle checked={approval} label="Require approval" about="Off: everyone's requests go straight to Radarr and Sonarr. On: requests from people who aren't admins wait until an admin approves them on the Requests page." onchange={set} />
			</div>
		</div>
	</div>
	<p class="text-xs text-gray-500">More general settings (app name, address, region and language) come with the rest of Settings.</p>
{/if}
