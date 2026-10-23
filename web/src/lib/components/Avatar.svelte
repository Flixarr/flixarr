<script lang="ts">
	import type { User } from '#lib/types.ts';
	let { user, class: cls = 'size-8 text-sm' }: { user: User; class?: string } = $props();
	let broken = $state(false);
	// Plex avatars load through the server's avatar cache (plex.tv answers each one with an uncached redirect).
	const src = $derived(/^https:\/\/([a-z0-9-]+\.)*plex\.tv\//i.test(user.avatar ?? '') ? '/api/avatar?u=' + encodeURIComponent(user.avatar!) : user.avatar);
</script>

{#if user.avatar && !broken}
	<img {src} alt="" class="{cls} shrink-0 rounded-full object-cover ring-1 ring-white/10" onerror={() => (broken = true)} />
{:else}
	<span class="{cls} grid shrink-0 place-items-center rounded-full bg-primary font-bold text-white">{user.username.charAt(0).toUpperCase()}</span>
{/if}
