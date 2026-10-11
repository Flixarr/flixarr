<script lang="ts">
	import Icon from './Icon.svelte';
	let { title, what }: { title: string; what: string } = $props();
</script>

<main class="mx-auto max-w-[1200px] px-4 pb-28 pt-20 laptop:px-10 laptop:pt-24">
	<h1 class="text-2xl font-bold laptop:text-3xl">{title}</h1>
	<div class="card mt-6 flex items-center gap-4 p-6">
		<span class="grid size-12 shrink-0 place-items-center rounded-full bg-primary/15 text-primary-light"><Icon name="clock" class="size-6" /></span>
		<div><div class="font-medium">Coming in the next update</div><p class="text-sm text-gray-400">{what}</p></div>
	</div>
</main>
