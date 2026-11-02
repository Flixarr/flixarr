<script lang="ts">
	// Someone's profile (#149), opened from Users: people who manage users only. Your own opens as Profile.
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { session } from '#lib/session.svelte.ts';
	import Profile from '#lib/components/Profile.svelte';

	const id = $derived(Number(page.params.id));
	$effect(() => {
		if (!session.me) return;
		if (id === session.me.id) goto('/profile', { replaceState: true });
		else if (!session.me.managesUsers) goto('/', { replaceState: true });
	});
</script>

{#if session.me?.managesUsers && id && id !== session.me.id}<Profile {id} />{/if}
