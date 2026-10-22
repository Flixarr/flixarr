<script lang="ts">
	// On/off switch with a label, a one-line explanation and an optional small tag after the label.
	let { checked, label, about = '', tag = '', disabled = false, onchange }: {
		checked: boolean; label: string; about?: string; tag?: string; disabled?: boolean; onchange: (on: boolean) => void;
	} = $props();
</script>

<label class="flex cursor-pointer items-center justify-between gap-4 {disabled ? 'cursor-not-allowed' : ''}">
	<span><span class="block text-sm">{label}{#if tag}<span class="ml-2 rounded-md bg-gray-800 px-1.5 py-0.5 align-middle text-[10px] text-gray-400 ring-1 ring-gray-700/60">{tag}</span>{/if}</span>{#if about}<span class="block text-xs text-gray-500">{about}</span>{/if}</span>
	<span class="relative inline-flex shrink-0">
		<input type="checkbox" class="peer sr-only" {checked} {disabled} onchange={(e) => onchange((e.currentTarget as HTMLInputElement).checked)} />
		<span class="h-6 w-11 rounded-full bg-gray-700 transition peer-checked:bg-primary peer-focus-visible:ring-2 peer-focus-visible:ring-primary/60 peer-disabled:opacity-50"></span>
		<span class="absolute left-0.5 top-0.5 size-5 rounded-full bg-white shadow transition peer-checked:translate-x-5"></span>
	</span>
</label>
