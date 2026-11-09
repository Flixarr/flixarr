<script lang="ts">
	// One number on a server's card: what it is, its value, and a line of detail. A link or a button when
	// there's somewhere to go.
	import type { Snippet } from 'svelte';

	let { label, value, sub, tone = '', href, onclick, expanded, children }: {
		label: string; value: string; sub?: string; tone?: string; href?: string; onclick?: () => void; expanded?: boolean; children?: Snippet;
	} = $props();

	const shell = 'block min-w-0 rounded-lg bg-gray-900/50 p-3 text-left ring-1 ring-gray-700/40';
	const active = 'transition hover:bg-gray-900/80 hover:ring-gray-600/60';
</script>

{#snippet body()}
	<span class="block text-[11px] font-medium text-gray-500">{label}</span>
	<span class="mt-0.5 block truncate text-sm font-semibold tabular-nums {tone || 'text-gray-100'}">{value}</span>
	{#if sub}<span class="mt-0.5 block truncate text-[11px] text-gray-500">{sub}</span>{/if}
	{#if children}{@render children()}{/if}
{/snippet}

{#if href}
	<a {href} class="{shell} {active}">{@render body()}</a>
{:else if onclick}
	<button class="{shell} {active} w-full" aria-expanded={expanded} {onclick}>{@render body()}</button>
{:else}
	<div class={shell}>{@render body()}</div>
{/if}
