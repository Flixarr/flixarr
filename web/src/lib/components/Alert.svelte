<script lang="ts">
	// The one alert box (#284), in the color palette's meanings: yellow = waiting or a heads up, blue = in progress,
	// green = done, red = a problem or not allowed, gray = neutral. Each tone has its own icon unless one is given.
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';

	type Tone = 'yellow' | 'blue' | 'green' | 'red' | 'gray';
	let { tone = 'gray', icon, class: cls = '', children }: { tone?: Tone; icon?: string; class?: string; children: Snippet } = $props();

	// Written out in full so Tailwind builds every class.
	const STYLE: Record<Tone, [string, string, string]> = {
		yellow: ['border-yellow-500/30 bg-yellow-500/10 text-yellow-200', 'text-yellow-400', 'clock'],
		blue: ['border-blue-500/30 bg-blue-500/10 text-blue-200', 'text-blue-400', 'info'],
		green: ['border-green-500/30 bg-green-500/10 text-green-200', 'text-green-400', 'check'],
		red: ['border-red-500/30 bg-red-500/10 text-red-200', 'text-red-400', 'warning'],
		gray: ['border-gray-500/30 bg-gray-500/10 text-gray-200', 'text-gray-400', 'info']
	};
	const s = $derived(STYLE[tone]);
</script>

<div class="flex gap-3 rounded-lg border px-3 py-2.5 text-sm {s[0]} {cls}" role="note">
	<span class="mt-0.5 shrink-0 {s[1]}"><Icon name={icon ?? s[2]} class="size-5" /></span>
	<div class="min-w-0">{@render children()}</div>
</div>
