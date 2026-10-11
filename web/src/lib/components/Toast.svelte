<script lang="ts">
	import Icon from './Icon.svelte';
	import { toast } from '#lib/toast.svelte.ts';
</script>

<div class="pointer-events-none fixed inset-x-0 bottom-24 z-[60] flex justify-center px-4 transition-opacity duration-300 tablet:bottom-8 {toast.show ? 'opacity-100' : 'opacity-0'}" role="status">
	<div class="flex items-center gap-2 rounded-xl border border-gray-700/60 bg-gray-900 px-4 py-3 text-sm shadow-2xl">
		<span class={toast.tone === 'green' ? 'text-green-400' : 'text-primary-light'}><Icon name="check" class="size-4" /></span>{toast.text}
	</div>
</div>
