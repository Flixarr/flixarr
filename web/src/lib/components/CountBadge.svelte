<script lang="ts">
	// The red number on a nav item (Requests, Issues): nothing at 0, 99+ past 99.
	let { n, class: cls = '-right-1 -top-1' }: { n: number; class?: string } = $props();
</script>

{#if n}<span class="absolute {cls} grid min-w-4 place-items-center rounded-full bg-red-500 px-1 text-[10px] font-bold leading-4 text-white">{n > 99 ? '99+' : n}</span>{/if}
