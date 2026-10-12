<script lang="ts">
	// Settings → Backups: daily copies of Flixarr's database (the Backup job), Back up now, how many to
	// keep, download, restore (Flixarr restarts itself) and delete.
	import { api, fmtDate, fmtBytes } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Modal from '#lib/components/Modal.svelte';
	import Select from '#lib/components/Select.svelte';
	import Icon from '#lib/components/Icon.svelte';

	type Backup = { name: string; size: number; created: string; kind: 'scheduled' | 'manual' | 'before-restore' };
	let list = $state<Backup[] | null>(null);
	let keep = $state(7);
	let busy = $state('');
	let target = $state<Backup | null>(null);
	let restoreOpen = $state(false);
	let deleteOpen = $state(false);
	let restarting = $state(false);

	async function load() {
		const d = await api<{ backups: Backup[]; keep: number }>('/backups');
		list = d.backups;
		keep = d.keep;
	}
	$effect(() => {
		load();
	});

	async function now() {
		busy = 'now';
		try {
			await api('/backups', { method: 'POST' });
			await load();
			notify('Backup saved.', 'green');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not back up.', 'orange');
		} finally {
			busy = '';
		}
	}
	async function setKeep(n: number) {
		try {
			const d = await api<{ backups: Backup[]; keep: number }>('/backups/settings', { method: 'PUT', body: JSON.stringify({ keep: n }) });
			keep = d.keep;
			notify(`Keeping the newest ${n} backups.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}
	async function restore() {
		restoreOpen = false;
		busy = 'restore';
		try {
			await api(`/backups/${target!.name}/restore`, { method: 'POST' });
			restarting = true;
			// Wait for Flixarr to come back, then reload everything.
			for (let i = 0; i < 60; i++) {
				await new Promise((r) => setTimeout(r, 1000));
				try {
					await api('/status');
					location.reload();
					return;
				} catch {
					// still restarting
				}
			}
			notify('Flixarr is taking a while to come back. Reload the page in a moment.', 'orange');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not restore.', 'orange');
		} finally {
			busy = '';
		}
	}
	async function remove() {
		deleteOpen = false;
		try {
			await api(`/backups/${target!.name}`, { method: 'DELETE' });
			list = list!.filter((b) => b.name !== target!.name);
			notify('Backup deleted.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not delete.', 'orange');
		}
	}
	const KIND: Record<Backup['kind'], string> = { scheduled: 'Daily', manual: 'Manual', 'before-restore': 'Before a restore' };
	const when = (t: string) => fmtDate(t, { month: 'short', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit' });
</script>

{#snippet heading(t: string)}<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">{t}</div>{/snippet}

{#if restarting}
	<div class="card flex items-center gap-4 p-6"><Icon name="refresh" class="size-6 animate-spin text-primary-light" /><div><div class="font-medium">Restoring</div><p class="text-sm text-gray-400">Flixarr is restarting with the backup. This page reloads when it's back.</p></div></div>
{:else if list}
	<div class="card flex flex-wrap items-center gap-4 p-4">
		<div class="min-w-0 flex-1 text-sm">
			<div class="font-medium">Daily backups</div>
			<div class="text-xs text-gray-400">Settings, users, requests and issues, saved every night by the Backup job (Settings → Jobs) to the <span class="font-mono">backups</span> folder in Flixarr's data folder. The cache isn't included; it refills on its own.</div>
		</div>
		<button class="btn-ghost shrink-0 px-3 py-1.5 text-xs" disabled={!!busy} onclick={now}>{busy === 'now' ? 'Saving…' : 'Back up now'}</button>
	</div>
	<div class="card px-4">
		<div class="flex items-center justify-between gap-4 py-3">
			<span><span class="block text-sm">Keep</span><span class="block text-xs text-gray-500">Older backups are deleted as new ones are made.</span></span>
			<Select value={keep} class="w-40" options={[3, 7, 14, 30].map((n) => ({ value: n, label: `Newest ${n}` }))} onchange={setKeep} />
		</div>
	</div>

	<div>
		{@render heading('Backups')}
		{#if !list.length}
			<div class="card p-4 text-sm text-gray-400">No backups yet. The first one is made tonight, or use Back up now.</div>
		{:else}
			<div class="card divide-y divide-gray-700/40 px-4">
				{#each list as b (b.name)}
					<div class="flex flex-wrap items-center gap-3 py-3">
						<div class="min-w-0 flex-1 text-sm">
							<div>{when(b.created)}</div>
							<div class="text-xs text-gray-500">{KIND[b.kind]} · {fmtBytes(b.size)}</div>
						</div>
						<div class="flex shrink-0 gap-2">
							<a class="btn-ghost px-3 py-1 text-xs" href="/api/backups/{b.name}" download>Download</a>
							<button class="btn-ghost px-3 py-1 text-xs" disabled={!!busy} onclick={() => ((target = b), (restoreOpen = true))}>Restore</button>
							<button class="px-2 text-xs text-gray-500 hover:text-red-300" aria-label="Delete this backup" onclick={() => ((target = b), (deleteOpen = true))}><Icon name="trash" class="size-4" /></button>
						</div>
					</div>
				{/each}
			</div>
		{/if}
		<p class="mt-2 text-xs text-gray-500">API keys and tokens inside a backup are encrypted with the <span class="font-mono">secret.key</span> file in the data folder. Keep that file too if you move Flixarr to another machine; without it they have to be entered again.</p>
	</div>
{/if}

<Modal bind:open={restoreOpen} title="Restore this backup?" sub={target ? when(target.created) : ''}>
	<p class="p-5 text-sm text-gray-300">Everything goes back to how it was then: settings, users, requests and issues. The current data is saved as a backup first, so this can be undone. Flixarr restarts, and everyone may need to sign in again.</p>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (restoreOpen = false)}>Cancel</button>
		<button class="btn px-6" onclick={restore}>Restore</button>
	{/snippet}
</Modal>

<Modal bind:open={deleteOpen} title="Delete this backup?" sub={target ? when(target.created) : ''}>
	<p class="p-5 text-sm text-gray-300">The file is removed from the backups folder.</p>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (deleteOpen = false)}>Keep it</button>
		<button class="btn px-6" onclick={remove}>Delete</button>
	{/snippet}
</Modal>
