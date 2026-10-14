<script lang="ts" module>
	export type Option<V> = { value: V; label: string; hint?: string };
</script>

<script lang="ts" generics="T extends string | number">
	// Custom dropdown (never the browser's own). With `search`, it's a combobox: typing filters the list.
	// The list is positioned fixed so it isn't clipped by scrolling modals, and opens upward near the
	// bottom of the screen. Keyboard: arrows, Home/End, Enter, Escape, and type-to-jump.
	import { tick } from 'svelte';
	import Icon from './Icon.svelte';

	let { value = $bindable(), options, id, placeholder = 'Pick one', search = false, disabled = false, size = 'md', class: cls = 'mt-1 w-full', onchange }: {
		value: T; options: Option<T>[]; id?: string; placeholder?: string; search?: boolean; disabled?: boolean; size?: 'sm' | 'md'; class?: string; onchange?: (v: T) => void;
	} = $props();

	let open = $state(false);
	let query = $state('');
	let active = $state(0);
	let button: HTMLButtonElement;
	let list = $state<HTMLUListElement>();
	let input = $state<HTMLInputElement>();
	let pos = $state({ right: 0, top: 0, width: 0, room: 0, up: false, max: 256 });
	const uid = `sel-${Math.random().toString(36).slice(2, 8)}`;

	const current = $derived(options.find((o) => o.value === value));
	const shown = $derived(search && query ? options.filter((o) => (o.label + ' ' + (o.hint ?? '')).toLowerCase().includes(query.toLowerCase())) : options);

	function place() {
		const r = button.getBoundingClientRect();
		const below = innerHeight - r.bottom - 12, above = r.top - 12;
		const up = below < 220 && above > below;
		// Right edges line up; the list is at least as wide as the button and grows to the left for longer options.
		pos = { right: innerWidth - r.right, top: up ? r.top - 6 : r.bottom + 6, width: r.width, room: r.right - 8, up, max: Math.min(320, Math.max(160, up ? above : below)) };
	}

	async function show() {
		if (disabled) return;
		place();
		query = '';
		active = Math.max(0, options.findIndex((o) => o.value === value));
		open = true;
		await tick();
		// Neither may scroll the page: a page scroll closes the list (it would float away from its button).
		// Phones don't jump into the search box: the keyboard would push the page and close the list.
		openedAt = Date.now();
		if (search && !matchMedia('(pointer: coarse)').matches) input?.focus({ preventScroll: true });
		reveal();
	}

	// Scrolls the list (only the list) so the active option shows.
	function reveal() {
		const el = list?.querySelector<HTMLElement>(`[data-i="${active}"]`);
		if (!el || !list) return;
		const top = el.offsetTop, bottom = top + el.offsetHeight;
		if (top < list.scrollTop) list.scrollTop = top;
		else if (bottom > list.scrollTop + list.clientHeight) list.scrollTop = bottom - list.clientHeight;
	}

	function hide(focus = true) {
		open = false;
		if (focus) button?.focus();
	}

	function choose(o: Option<T>) {
		value = o.value;
		onchange?.(o.value);
		hide();
	}

	function move(to: number) {
		if (!shown.length) return;
		active = (to + shown.length) % shown.length;
		reveal();
	}

	let openedAt = 0;
	let typed = '', typedAt = 0;
	function onkey(e: KeyboardEvent) {
		if (!open) {
			if (['ArrowDown', 'ArrowUp', 'Enter', ' '].includes(e.key)) { e.preventDefault(); show(); }
			return;
		}
		if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); hide(); }
		else if (e.key === 'ArrowDown') { e.preventDefault(); move(active + 1); }
		else if (e.key === 'ArrowUp') { e.preventDefault(); move(active - 1); }
		else if (e.key === 'Home') { e.preventDefault(); move(0); }
		else if (e.key === 'End') { e.preventDefault(); move(shown.length - 1); }
		else if (e.key === 'Enter') { e.preventDefault(); if (shown[active]) choose(shown[active]); }
		else if (e.key === 'Tab') hide(false);
		else if (!search && e.key.length === 1) {
			// Type to jump to the first option starting with what was typed.
			const now = Date.now();
			typed = (now - typedAt > 700 ? '' : typed) + e.key.toLowerCase();
			typedAt = now;
			const i = shown.findIndex((o) => o.label.toLowerCase().startsWith(typed));
			if (i >= 0) move(i);
		}
	}

	$effect(() => {
		if (!open) return;
		const away = (e: PointerEvent) => !button.contains(e.target as Node) && !list?.contains(e.target as Node) && hide(false);
		// Scrolling the page or modal moves the button, so the list closes rather than float away.
		// (Ignored right after opening, while the page may still be settling.)
		const scrolled = (e: Event) => Date.now() - openedAt > 400 && !list?.contains(e.target as Node) && hide(false);
		document.addEventListener('pointerdown', away, true);
		addEventListener('scroll', scrolled, true);
		addEventListener('resize', place);
		return () => {
			document.removeEventListener('pointerdown', away, true);
			removeEventListener('scroll', scrolled, true);
			removeEventListener('resize', place);
		};
	});

	// A new filter starts from the top match.
	$effect(() => {
		if (query) active = 0;
	});
</script>

<button bind:this={button} {id} type="button" {disabled} onclick={() => (open ? hide() : show())} onkeydown={onkey}
	role="combobox" aria-haspopup="listbox" aria-expanded={open} aria-controls={uid}
	class="{cls} flex items-center gap-2 rounded-lg border border-gray-700/60 bg-gray-800/70 text-left outline-none transition hover:border-gray-600 focus-visible:ring-2 focus-visible:ring-primary/60 disabled:cursor-not-allowed disabled:opacity-50
		{size === 'sm' ? 'px-2.5 py-1 text-xs' : 'px-3 py-2 text-sm'} {open ? 'border-gray-500' : ''}">
	<span class="min-w-0 flex-1 truncate {current ? '' : 'text-gray-500'}">{current?.label ?? placeholder}</span>
	{#if current?.hint && size === 'md'}<span class="shrink-0 text-xs text-gray-500">{current.hint}</span>{/if}
	<span class="shrink-0 text-gray-400 transition {open ? '-rotate-90' : 'rotate-90'}"><Icon name="chevron" class="size-4" /></span>
</button>

{#if open}
	<div class="fixed z-[60] overflow-hidden rounded-lg border border-gray-700/60 bg-gray-900 shadow-[0_16px_40px_-4px_rgb(0_0_0/0.75),0_4px_12px_rgb(0_0_0/0.5)] {pos.up ? '-translate-y-full' : ''}"
		style="right:{pos.right}px; top:{pos.top}px; min-width:{pos.width}px; width:max-content; max-width:{pos.room}px">
		{#if search}
			<div class="border-b border-gray-700/40 p-2">
				<input bind:this={input} bind:value={query} onkeydown={onkey} placeholder="Type to filter" autocomplete="off" spellcheck="false"
					class="w-full rounded-md bg-gray-800/70 px-2.5 py-1.5 text-sm outline-none placeholder:text-gray-500 focus:ring-1 focus:ring-primary/60" />
			</div>
		{/if}
		<ul bind:this={list} id={uid} role="listbox" class="relative overflow-y-auto overscroll-contain py-1" style="max-height:{pos.max - (search ? 52 : 0)}px">
			{#each shown as o, i (o.value)}
				<li data-i={i} role="option" aria-selected={o.value === value}>
					<button type="button" tabindex="-1" onclick={() => choose(o)} onpointermove={() => (active = i)}
						class="flex w-full items-center gap-2 px-3 py-2 text-left {size === 'sm' ? 'text-xs' : 'text-sm'} {i === active ? 'bg-white/5' : ''} {o.value === value ? 'text-white' : 'text-gray-300'}">
						<span class="min-w-0 flex-1 truncate">{o.label}</span>
						{#if o.hint}<span class="shrink-0 text-xs text-gray-500">{o.hint}</span>{/if}
						<span class="size-4 shrink-0 text-primary-light">{#if o.value === value}<Icon name="check" class="size-4" />{/if}</span>
					</button>
				</li>
			{:else}
				<li class="px-3 py-2 text-sm text-gray-500">Nothing matches.</li>
			{/each}
		</ul>
	</div>
{/if}
