<script lang="ts">
	// Someone in a list: their picture and name, with a small line under it (when, or what they did).
	import type { User } from '#lib/types.ts';
	import Avatar from './Avatar.svelte';
	let { name, avatar = '', sub = '', size = 'size-5 text-[10px]', class: cls = '' }: { name: string; avatar?: string; sub?: string; size?: string; class?: string } = $props();
	const user = $derived({ username: name || '?', avatar } as User);
</script>

<div class="flex min-w-0 items-center gap-2 {cls}">
	<Avatar {user} class={size} />
	<div class="min-w-0">
		<div class="truncate text-xs text-gray-200">{name || 'Someone'}</div>
		{#if sub}<div class="truncate text-[11px] text-gray-500">{sub}</div>{/if}
	</div>
</div>
