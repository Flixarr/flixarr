<script lang="ts" module>
	// Backdrops sit dimmed behind a fade, so phones (often 3× screens) get a smaller one than a full-width photo would need.
	export const BACKDROP_SIZES = '(min-width: 1024px) 100vw, 60vw';
</script>

<script lang="ts">
	// TMDB image that loads well on slow connections:
	// 1. waits until it's near the screen (unless `eager`, for the first thing you see),
	// 2. lets the browser pick the right size for the screen (srcset + sizes),
	// 3. shows a tiny blurred copy first, then fades the full image in once it has fully downloaded.
	import { img } from '#lib/api.ts';
	import { nearScreen } from '#lib/lazy.ts';

	type Kind = 'poster' | 'backdrop' | 'profile' | 'still' | 'logo';
	const WIDTHS: Record<Kind, number[]> = {
		poster: [92, 154, 185, 342, 500, 780],
		backdrop: [300, 780, 1280],
		profile: [45, 185],
		still: [92, 185, 300],
		logo: [92, 154, 185, 300, 500]
	};
	const TINY: Record<Kind, string> = { poster: 'w92', backdrop: 'w300', profile: 'w45', still: 'w92', logo: '' };

	let { path, kind, sizes, alt = '', class: cls = '', imgClass = '', eager = false, onerror }: {
		path: string;
		kind: Kind;
		sizes: string;
		alt?: string;
		class?: string; // wrapper: size and position (the image fills it)
		imgClass?: string; // extra classes on the image itself
		eager?: boolean;
		onerror?: () => void;
	} = $props();

	let near = $state(false);
	let loaded = $state(false);
	let instant = $state(false); // already downloaded when shown: no fade at all
	let faded = $state(false); // the full image has finished fading in, so the blurred copy underneath can go
	let el = $state<HTMLImageElement>();
	const show = () => (near = true);
	const srcset = $derived(WIDTHS[kind].map((w) => `${img(path, 'w' + w)} ${w}w`).join(', '));
	const tiny = $derived(TINY[kind] && img(path, TINY[kind]));
	const largest = $derived(img(path, 'w' + WIDTHS[kind].at(-1)));

	// An image the browser already has can finish before Svelte's onload is attached.
	$effect(() => {
		if (el?.complete && el.naturalWidth) loaded = faded = instant = true;
	});
</script>

{#if kind === 'logo'}
	<!-- Logos are transparent artwork: no blurred stand-in, just a fade-in. -->
	<img bind:this={el} src={largest} srcset={srcset} {sizes} {alt} loading={eager ? 'eager' : 'lazy'} fetchpriority={eager ? 'high' : 'auto'} decoding="async"
		onload={() => (loaded = true)} {onerror} class="{cls} transition-opacity duration-500 {loaded ? 'opacity-100' : 'opacity-0'}" />
{:else}
	<span class="relative block overflow-hidden {cls}" use:nearScreen={show}>
		{#if (near || eager) && tiny && !faded}
			<img src={tiny} alt="" aria-hidden="true" decoding="async" class="absolute inset-0 h-full w-full scale-110 object-cover blur-md" />
		{/if}
		{#if near || eager}
			<img bind:this={el} src={largest} srcset={srcset} {sizes} {alt} decoding="async" fetchpriority={eager ? 'high' : 'auto'}
				onload={() => (loaded = true)} ontransitionend={() => (faded = true)} {onerror}
				class="absolute inset-0 h-full w-full object-cover {instant ? '' : 'transition-opacity duration-500'} {loaded ? 'opacity-100' : 'opacity-0'} {imgClass}" />
		{/if}
	</span>
{/if}
