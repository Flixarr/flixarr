<script lang="ts">
	// On/off switch with a label and a one-line explanation.
	let { checked, label, about = '', disabled = false, onchange }: {
		checked: boolean; label: string; about?: string; disabled?: boolean; onchange: (on: boolean) => void;
	} = $props();
</script>

<label class="flex cursor-pointer items-center justify-between gap-4 {disabled ? 'cursor-not-allowed' : ''}">
	<span><span class="block text-sm">{label}</span>{#if about}<span class="block text-xs text-gray-500">{about}</span>{/if}</span>
	<span class="relative inline-flex shrink-0">
		<input type="checkbox" class="peer sr-only" {checked} {disabled} onchange={(e) => onchange((e.currentTarget as HTMLInputElement).checked)} />
		<span class="h-6 w-11 rounded-full bg-gray-700 transition peer-checked:bg-primary peer-focus-visible:ring-2 peer-focus-visible:ring-primary/60 peer-disabled:opacity-50"></span>
		<span class="absolute left-0.5 top-0.5 size-5 rounded-full bg-white shadow transition peer-checked:translate-x-5"></span>
	</span>
</label>
