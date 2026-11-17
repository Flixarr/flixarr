<script lang="ts">
	// Underlined tabs across the top of a page (Settings → Services, Settings → System). Each tab is a link,
	// so Back and links to a tab work. Scrolls sideways on narrow screens.
	let { tabs, value }: { tabs: { key: string; label: string; href: string }[]; value: string } = $props();
</script>

<div class="no-scrollbar flex gap-5 overflow-x-auto border-b border-gray-700/60 text-sm" role="tablist">
	{#each tabs as t (t.key)}
		<a href={t.href} role="tab" aria-selected={t.key === value} data-sveltekit-noscroll
			class="-mb-px shrink-0 border-b-2 pb-2.5 font-medium transition {t.key === value ? 'border-primary text-white' : 'border-transparent text-gray-400 hover:text-white'}">{t.label}</a>
	{/each}
</div>
