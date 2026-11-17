<script lang="ts">
	// One connection on Settings → Services (Radarr, Sonarr, Maintainerr), every one drawn the same:
	// logo, name and version, a line about how it's doing, its status, then Edit, Open and ⋯ (Refresh now,
	// Copy address, Remove). Not connected: the same row, dimmed, with Connect.
	import type { Snippet } from 'svelte';
	import { notify } from '#lib/toast.svelte.ts';
	import Icon from '#lib/components/Icon.svelte';

	let { logo, name, version = '', about, status, pill = '', web = '', onedit, onremove, onrefresh, children }: {
		logo: string; name: string; version?: string; about: string; pill?: string;
		/** none = not set up; the rest are its status pill */
		status: 'none' | 'checking' | 'ok' | 'issues' | 'down';
		web?: string; onedit: () => void; onremove?: () => void; onrefresh?: () => Promise<void>; children?: Snippet;
	} = $props();

	const PILL = {
		checking: { tone: 'text-gray-400 ring-gray-700/60', dot: 'bg-gray-500' },
		ok: { tone: 'bg-green-500/10 text-green-300 ring-green-500/30', dot: 'bg-green-500' },
		issues: { tone: 'bg-yellow-500/10 text-yellow-300 ring-yellow-500/30', dot: 'bg-yellow-400' },
		down: { tone: 'bg-red-500/10 text-red-300 ring-red-500/30', dot: 'bg-red-500' }
	};

	let menu = $state(false);
	let refreshing = $state(false);
	let el = $state<HTMLElement>();
	$effect(() => {
		if (!menu) return;
		const away = (e: PointerEvent) => !el?.contains(e.target as Node) && (menu = false);
		document.addEventListener('pointerdown', away);
		return () => document.removeEventListener('pointerdown', away);
	});
	async function refresh() {
		refreshing = true;
		try {
			await onrefresh!();
		} finally {
			refreshing = false;
			menu = false;
		}
	}
	async function copy() {
		menu = false;
		try {
			await navigator.clipboard.writeText(web);
			notify('Address copied.');
		} catch {
			notify(web, 'orange');
		}
	}
	const action = 'inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-gray-300 hover:bg-white/5 hover:text-white';
	const menuItem = 'flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-white/5 disabled:opacity-50';
</script>

{#if status === 'none'}
	<div class="flex items-center gap-3 px-4 py-3">
		<img src="/logos/{logo}.svg" alt="" class="size-7 shrink-0 opacity-40 grayscale" />
		<div class="min-w-0 flex-1"><div class="font-medium text-gray-300">{name}</div><div class="text-xs text-gray-500">{about}</div></div>
		<button class="btn-ghost inline-flex shrink-0 items-center gap-1 px-3 py-1 text-xs" onclick={onedit}><Icon name="plus" class="size-3.5" />Connect</button>
	</div>
{:else}
	<div class="flex flex-wrap items-center gap-3 px-4 py-3">
		<img src="/logos/{logo}.svg" alt="" class="size-7 shrink-0" />
		<div class="min-w-0 flex-1">
			<div class="flex items-center gap-2 font-medium">{name}{#if version}<span class="text-[11px] font-normal text-gray-500">{version}</span>{/if}</div>
			{#if children}<div class="text-xs text-gray-500">{@render children()}</div>{/if}
		</div>
		<div class="flex w-full items-center justify-between gap-2 tablet:w-auto">
			<span class="inline-flex shrink-0 items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs ring-1 {PILL[status].tone}"><span class="size-1.5 rounded-full {PILL[status].dot}"></span>{pill}</span>
			<div class="flex shrink-0 items-center gap-1 whitespace-nowrap text-xs">
				<button class={action} onclick={onedit}><Icon name="settings" class="size-3.5" />Edit</button>
				{#if web}<a class={action} href={web} target="_blank" rel="noopener">Open {name}<Icon name="external" class="size-3.5" /></a>{/if}
				<span bind:this={el} class="relative">
					<button class="grid size-7 place-items-center rounded-md hover:bg-white/5 hover:text-white {menu ? 'bg-white/5 text-white' : 'text-gray-400'}" aria-label="More for {name}" aria-expanded={menu} onclick={() => (menu = !menu)}><Icon name="dots" class="size-4" /></button>
					{#if menu}
						<div class="absolute right-0 top-full z-20 mt-1 w-44 overflow-hidden rounded-lg border border-gray-700/60 bg-gray-900 py-1 text-xs shadow-[0_16px_40px_-4px_rgb(0_0_0/0.75),0_4px_12px_rgb(0_0_0/0.5)]">
							{#if onrefresh}<button class="{menuItem} text-gray-300 hover:text-white" disabled={refreshing} onclick={refresh}><Icon name="refresh" class="size-3.5" />{refreshing ? 'Refreshing…' : 'Refresh now'}</button>{/if}
							{#if web}<button class="{menuItem} text-gray-300 hover:text-white" onclick={copy}><Icon name="copy" class="size-3.5" />Copy address</button>{/if}
							{#if onremove}<button class="{menuItem} text-red-300" onclick={() => { menu = false; onremove(); }}><Icon name="trash" class="size-3.5" />Remove</button>{/if}
						</div>
					{/if}
				</span>
			</div>
		</div>
	</div>
{/if}
