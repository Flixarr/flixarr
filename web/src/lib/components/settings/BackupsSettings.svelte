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
	// Offsite: each backup also copied to another folder and/or S3-compatible storage.
	type S3 = { endpoint: string; region: string; bucket: string; prefix: string; accessKey: string; secretKey?: string };
	type Offsite = { folder: string; s3: S3; secretKeySet: boolean; last: { at: string; error?: string } };
	let off = $state<Offsite | null>(null);
	let offOpen = $state(false);
	let secret = $state('');
	const input = 'mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60';

	async function load() {
		const d = await api<{ backups: Backup[]; keep: number; offsite: Offsite }>('/backups');
		list = d.backups;
		keep = d.keep;
		off = d.offsite;
	}
	async function saveOffsite() {
		busy = 'offsite';
		try {
			const d = await api<{ backups: Backup[]; keep: number; offsite: Offsite }>('/backups/offsite', { method: 'PUT', body: JSON.stringify({ folder: off!.folder, s3: { ...off!.s3, secretKey: secret } }) });
			off = d.offsite;
			secret = '';
			notify(off.folder || off.s3.endpoint ? 'Saved. The newest backup was copied there.' : 'Offsite copies are off.', 'green');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
			load();
		} finally {
			busy = '';
		}
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

	{#if off}
		<div>
			{@render heading('Offsite copies')}
			<div class="card divide-y divide-gray-700/40 px-4 text-sm">
				<!-- Collapsed until opened (#108): the form is long and rarely changed. -->
				<button class="flex w-full items-center gap-3 py-3 text-left" aria-expanded={offOpen} onclick={() => (offOpen = !offOpen)}>
					<span class="min-w-0 flex-1">
						<span class="block">{[off.folder && 'A folder', off.s3.endpoint && 'S3 storage'].filter(Boolean).join(' and ') || 'Off'}</span>
						<span class="block text-xs {off.last.error ? 'text-red-300' : 'text-gray-500'}">{off.folder || off.s3.endpoint ? (off.last.at ? `Last copy ${when(off.last.at)}${off.last.error ? `: ${off.last.error}` : ', worked'}` : 'Nothing copied yet.') : 'Copy each backup to another drive or to S3-compatible storage.'}</span>
					</span>
					<Icon name="chevron" class="size-4 shrink-0 text-gray-500 transition {offOpen ? 'rotate-90' : ''}" />
				</button>
				{#if offOpen}
				<p class="py-3 text-xs text-gray-400">A backup on the same disk doesn't help if the disk dies. Each backup can also be copied to another folder (a different drive, mounted into Flixarr's container) and to S3-compatible storage (Amazon S3, Backblaze B2, Cloudflare R2, MinIO…). Each place keeps the same number of copies as above.</p>
				<div class="py-3"><label for="o-folder" class="block">Folder</label><input id="o-folder" class={input} bind:value={off.folder} placeholder="/offsite-backups" autocomplete="off" spellcheck="false" /><p class="mt-1 text-xs text-gray-500">A full path inside the container. Leave empty to not use one.</p></div>
				<div class="grid gap-3 py-3 tablet:grid-cols-2">
					<div class="tablet:col-span-2"><label for="o-ep" class="block">S3 endpoint</label><input id="o-ep" class={input} bind:value={off.s3.endpoint} placeholder="https://s3.us-east-1.amazonaws.com" autocomplete="off" spellcheck="false" /></div>
					<div><label for="o-b" class="block">Bucket</label><input id="o-b" class={input} bind:value={off.s3.bucket} autocomplete="off" spellcheck="false" /></div>
					<div><label for="o-r" class="block">Region</label><input id="o-r" class={input} bind:value={off.s3.region} placeholder="us-east-1 (R2: auto)" autocomplete="off" spellcheck="false" /></div>
					<div><label for="o-p" class="block">Folder in the bucket <span class="text-gray-500">(optional)</span></label><input id="o-p" class={input} bind:value={off.s3.prefix} placeholder="flixarr" autocomplete="off" spellcheck="false" /></div>
					<div><label for="o-a" class="block">Access key</label><input id="o-a" class={input} bind:value={off.s3.accessKey} autocomplete="off" spellcheck="false" /></div>
					<div class="tablet:col-span-2"><label for="o-s" class="block">Secret key</label><input id="o-s" class={input} type="password" bind:value={secret} placeholder={off.secretKeySet ? 'Saved. Type a new one to replace it.' : ''} autocomplete="new-password" /></div>
				</div>
				<div class="flex flex-wrap items-center justify-between gap-3 py-3">
					<span class="text-xs {off.last.error ? 'text-red-300' : 'text-gray-500'}">{off.last.at ? `Last copy ${when(off.last.at)}${off.last.error ? `: ${off.last.error}` : ', worked'}` : 'Nothing copied yet.'}</span>
					<button class="btn px-5 py-1.5 text-sm" disabled={!!busy} onclick={saveOffsite}>{busy === 'offsite' ? 'Saving and copying…' : 'Save'}</button>
				</div>
				{/if}
			</div>
			<p class="mt-2 text-xs text-gray-500">Keep a copy of <span class="font-mono">secret.key</span> with them too; it isn't in the backups.</p>
		</div>
	{/if}
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
