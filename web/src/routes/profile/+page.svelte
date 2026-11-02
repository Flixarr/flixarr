<script lang="ts">
	// Your profile: the same page as everyone else's (#193), with your settings in a window.
	import { session } from '#lib/session.svelte.ts';
	import Profile from '#lib/components/Profile.svelte';
</script>

{#if session.me}<Profile id={session.me.id} />{/if}
