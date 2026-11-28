<script lang="ts">
	// The MORE LIKE block of a like search (#291): the title the results are based on, where the Top result card
	// would be, with Play or Request. "Not this one?" lists other titles with the same name (Top Gun 1955, Top Gun:
	// Maverick) and picking one redoes the results for it. The list floats over the page (inset on phones).
	import { href } from '#lib/api.ts';
	import type { LikeSource, Movie, Series, SearchHit } from '#lib/types.ts';
	import Img from './Img.svelte';
	import TiltFrame from './TiltFrame.svelte';
	import RequestButton from './RequestButton.svelte';
	let { src, item, alts, onpick }: { src: LikeSource; item: Movie | Series | null; alts: SearchHit[]; onpick: (h: SearchHit) => void } = $props();
	const kind = $derived(src.type === 'series' ? 'Series' : 'Movie');
	const link = $derived(href({ type: src.type === 'series' ? 'series' : 'movie', id: src.id }));
	let open = $state(false);
	let box = $state<HTMLElement>();
	$effect(() => {
		if (!open) return;
		const close = (e: PointerEvent) => !box?.contains(e.target as Node) && (open = false);
		const esc = (e: KeyboardEvent) => e.key === 'Escape' && (open = false);
		addEventListener('pointerdown', close);
		addEventListener('keydown', esc);
		return () => { removeEventListener('pointerdown', close); removeEventListener('keydown', esc); };
	});
</script>

<div class="px-4 laptop:px-0">
	<div class="relative rounded-2xl ring-1 ring-gray-700/50">
		<div class="absolute inset-0 overflow-hidden rounded-2xl">
			{#if src.backdrop}<Img path={src.backdrop} kind="backdrop" sizes="100vw" class="absolute inset-0 h-full w-full opacity-30" />{/if}
			<div class="absolute inset-0 bg-gradient-to-r from-gray-950 via-gray-950/80 to-transparent"></div>
		</div>
		<div class="relative flex gap-4 p-4 tablet:gap-5 tablet:p-5 laptop:p-7">
			<a href={link} data-top-link class="group w-24 shrink-0 self-start rounded-xl outline-none focus-visible:ring-2 focus-visible:ring-primary tablet:w-32 laptop:w-40">
				<TiltFrame class="aspect-[2/3] rounded-xl bg-gray-800 shadow-2xl ring-1 ring-white/10">
					{#if src.poster}<Img path={src.poster} kind="poster" sizes="160px" class="h-full w-full" />{:else}<div class="grid h-full place-items-center text-3xl text-gray-400">{src.title.charAt(0)}</div>{/if}
				</TiltFrame>
			</a>
			<div class="min-w-0 space-y-2 self-center">
				<div class="text-[11px] font-bold uppercase tracking-wider text-primary">More like</div>
				<a href={link} class="block text-xl font-bold text-white hover:text-primary-light tablet:text-2xl laptop:text-4xl">{src.title}</a>
				<div class="text-sm text-gray-400">{[src.year, kind, src.rating ? `★ ${src.rating}` : ''].filter(Boolean).join(' · ')}</div>
				{#if src.overview}<p class="line-clamp-2 max-w-2xl text-sm text-gray-400">{src.overview}</p>{/if}
				<div class="flex flex-wrap items-center gap-2 pt-1">
					{#if item && item.id === src.id}<RequestButton {item} />{/if}
					<a href={link} class="btn-ghost act">Details</a>
					{#if alts.length}
						<div bind:this={box} class="tablet:relative">
							<button class="ml-1 text-sm text-gray-400 underline decoration-gray-600 underline-offset-2 hover:text-white" aria-expanded={open} onclick={() => (open = !open)}>Not this one?</button>
							{#if open}
								<div class="absolute inset-x-3 top-full z-30 mt-2 overflow-hidden rounded-xl border border-gray-700/60 bg-gray-900 py-1 shadow-2xl shadow-black/60 tablet:inset-x-auto tablet:left-0 tablet:w-80">
									<div class="px-4 py-2 text-[11px] font-bold uppercase tracking-wider text-gray-400">Show titles like</div>
									{#each alts as a (a.type + a.id)}
										<button class="flex w-full items-center gap-3 px-4 py-2 text-left hover:bg-white/5" onclick={() => { open = false; onpick(a); }}>
											{#if a.poster}<Img path={a.poster} kind="poster" sizes="32px" class="h-12 w-8 shrink-0 rounded" />{:else}<span class="h-12 w-8 shrink-0 rounded bg-gray-800"></span>{/if}
											<span class="min-w-0"><span class="block truncate text-sm text-white">{a.title}</span><span class="block text-xs text-gray-400">{[a.year, a.type === 'series' ? 'Series' : 'Movie'].filter(Boolean).join(' · ')}</span></span>
										</button>
									{/each}
								</div>
							{/if}
						</div>
					{/if}
				</div>
			</div>
		</div>
	</div>
</div>
