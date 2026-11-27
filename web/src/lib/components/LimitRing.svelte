<script lang="ts">
	// A weekly request limit as a ring (#278): it empties as requests are used and shows how many are left.
	// Orange normally, yellow at 2 or fewer left, red at none (the color palette, Rule 37).
	let { limit, used, unit, class: cls = '' }: { limit: number; used: number; unit: 'movie' | 'season'; class?: string } = $props();

	const left = $derived(Math.max(0, limit - used));
	const tone = $derived(left === 0 ? 'red' : left <= 2 ? 'yellow' : 'normal');
	const R = 15;
	const C = 2 * Math.PI * R;
</script>

<div class="flex items-center gap-2.5 {cls}">
	<div class="relative size-10 shrink-0">
		<svg viewBox="0 0 40 40" class="size-10 -rotate-90" aria-hidden="true">
			<circle cx="20" cy="20" r={R} fill="none" stroke="currentColor" stroke-width="4" class="text-gray-700" />
			<circle cx="20" cy="20" r={R} fill="none" stroke="currentColor" stroke-width="4" stroke-linecap="round" stroke-dasharray={C} stroke-dashoffset={C * (1 - left / limit)}
				class={tone === 'red' ? 'text-red-400' : tone === 'yellow' ? 'text-yellow-400' : 'text-primary'} />
		</svg>
		<span class="absolute inset-0 grid place-items-center text-sm font-bold {tone === 'red' ? 'text-red-300' : tone === 'yellow' ? 'text-yellow-300' : 'text-white'}">{left}</span>
	</div>
	<div class="text-xs leading-tight {tone === 'red' ? 'text-red-300' : tone === 'yellow' ? 'text-yellow-300' : 'text-gray-400'}">{unit}{left === 1 ? '' : 's'} left<br />this week</div>
</div>
