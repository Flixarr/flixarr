<script lang="ts">
	// Settings → General: Flixarr's address, region and language.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Select from '#lib/components/Select.svelte';
	import { REGIONS, LANGUAGES } from '#lib/locales.ts';

	type General = { region: string; language: string; appUrl: string };
	let general = $state<General | null>(null);
	let appUrl = $state('');
	let savingUrl = $state(false);
	$effect(() => {
		api<General>('/settings/general').then((d) => ((general = d), (appUrl = d.appUrl)));
	});
	async function saveGeneral(change: Partial<General>, msg: string) {
		try {
			general = await api<General>('/settings/general', { method: 'PUT', body: JSON.stringify({ ...general, ...change }) });
			appUrl = general.appUrl;
			notify(msg);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}
	async function saveUrl() {
		savingUrl = true;
		await saveGeneral({ appUrl }, appUrl.trim() ? 'Application URL saved.' : 'Application URL cleared.');
		savingUrl = false;
	}
</script>

{#if general}
	<div>
		<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Flixarr</div>
		<div class="card divide-y divide-gray-700/40 px-4">
			<div class="py-3">
				<label for="gen-url" class="block text-sm">Application URL</label>
				<span class="block text-xs text-gray-500">The address people use to reach Flixarr. Links in notifications point here.</span>
				<div class="mt-2 flex gap-2">
					<input id="gen-url" class="w-full flex-1 rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60" autocomplete="off" spellcheck="false" placeholder="https://flixarr.example.com" bind:value={appUrl} onkeydown={(e) => e.key === 'Enter' && saveUrl()} />
					<button class="btn-ghost shrink-0 px-4 text-sm" disabled={savingUrl || appUrl.trim() === general.appUrl} onclick={saveUrl}>Save</button>
				</div>
				{#if !general.appUrl && !appUrl.trim()}
					<button class="mt-2 text-xs text-primary-light hover:underline" onclick={() => ((appUrl = location.origin), saveUrl())}>Use {location.origin}</button>
				{/if}
			</div>
		</div>
	</div>
	<!-- Region and language are about what Discover and title pages show, not the app's own language (#113). -->
	<div>
		<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Discover</div>
		<div class="card px-4">
			<p class="pt-3 text-xs text-gray-400">What movies and series show: release dates, ratings, upcoming lists, titles and descriptions. Flixarr itself stays in English. People can pick their own in their Profile.</p>
			<div class="grid gap-4 py-3 tablet:grid-cols-2">
				<div>
					<label for="gen-region" class="block text-sm">Region</label>
					<span class="block text-xs text-gray-500">Release dates, age ratings and what counts as upcoming.</span>
					<Select id="gen-region" search value={general.region} options={REGIONS.map(([v, l]) => ({ value: v, label: l }))} onchange={(v) => saveGeneral({ region: v }, `Region set to ${REGIONS.find(([c]) => c === v)?.[1]}.`)} />
				</div>
				<div>
					<label for="gen-lang" class="block text-sm">Language</label>
					<span class="block text-xs text-gray-500">Titles and descriptions, where TMDB has them.</span>
					<Select id="gen-lang" search value={general.language} options={LANGUAGES.map(([v, l]) => ({ value: v, label: l }))} onchange={(v) => saveGeneral({ language: v }, `Language set to ${LANGUAGES.find(([c]) => c === v)?.[1]}.`)} />
				</div>
			</div>
		</div>
	</div>
{/if}
