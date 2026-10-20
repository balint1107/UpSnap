<script lang="ts">
	import { goto } from '$app/navigation';
	import { asset, resolve } from '$app/paths';
	import { page } from '$app/state';
	import Navbar from '$lib/components/Navbar.svelte';
	import Transition from '$lib/components/Transition.svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { localeStore } from '$lib/stores/locale';
	import { backendUrl, permission, pocketbase } from '$lib/stores/pocketbase';
	import { settingsPriv, settingsPub } from '$lib/stores/settings';
	import type { Permission } from '$lib/types/permission';
	import type { SettingsPrivate, SettingsPublic } from '$lib/types/settings';
	import { onMount } from 'svelte';
	import toast, { Toaster, type ToastOptions } from 'svelte-french-toast';
	import '../app.css';

	let { children } = $props();
	let authIsValid = $state(false);
	let manifestHref = $state(
		asset('/server-mark.svg').replace(/[^/]*$/, 'api/upsnap/manifest.webmanifest')
	);
	$effect(() => {
		// An absolute client URL keeps the manifest stable across SPA redirects.
		const iconURL = new URL(asset('/server-mark.svg'), page.url);
		manifestHref = new URL('api/upsnap/manifest.webmanifest', iconURL).href;
	});

	const toastOptions: ToastOptions = {
		duration: 5000
	};

	// list of RTL languages, more can be added if needed
	const RTL_LANGS = ['ar-SA'];

	function setHTMLdir(lang: string) {
		if (RTL_LANGS.includes(lang)) {
			document.documentElement.setAttribute('dir', 'rtl');
		} else {
			document.documentElement.setAttribute('dir', 'ltr');
		}
	}

	$effect(() => {
		setHTMLdir($localeStore);
	});

	onMount(async () => {
		$pocketbase.authStore.onChange(() => {
			authIsValid = $pocketbase.authStore.isValid;

			// load user permissions
			if ($pocketbase.authStore.record?.collectionName === 'users') {
				$pocketbase
					.collection('permissions')
					.getFirstListItem(`user.id = '${$pocketbase.authStore.record.id}'`)
					.then((data) => {
						permission.set(data as Permission);
					})
					.catch(() => {
						return;
					});

				$pocketbase.collection('permissions').subscribe('*', (event) => {
					permission.set(event.record as Permission);
					toast.success(m.toasts_permissions_updated_personal());
				});
			}
		});

		// set settingsPub store on load
		if (!$settingsPub) {
			const res = await $pocketbase.collection('settings_public').getFirstListItem('');
			settingsPub.set(res as SettingsPublic);
		}

		// redirect to welcome page if setup is not completed
		if ($settingsPub.setup_completed === false) {
			$pocketbase.authStore.clear();
			if (page.url.pathname !== '/welcome/') {
				goto(resolve('/welcome'));
			}
			return;
		}

		// refresh auth token
		if ($pocketbase.authStore.isSuperuser) {
			await $pocketbase
				.collection('_superusers')
				.authRefresh()
				.then(async () => {
					// set settingsPriv store on load
					if (!$settingsPriv) {
						const res = await $pocketbase.collection('settings_private').getFirstListItem('');
						settingsPriv.set(res as SettingsPrivate);
					}
				})
				.catch((err) => {
					// clear the store only on invalidated/expired token
					const status = err?.status << 0;
					if (status == 401 || status == 403) {
						$pocketbase.authStore.clear();
						goto(resolve('/login'));
					} else {
						console.log('NOT CLEARED');
					}
				});
		} else {
			await $pocketbase
				.collection('users')
				.authRefresh()
				.catch((err) => {
					// clear the store only on invalidated/expired token
					const status = err?.status << 0;
					if (status == 401 || status == 403) {
						$pocketbase.authStore.clear();
						goto(resolve('/login'));
					}
				});
		}
	});
</script>

<svelte:head>
	<!-- Keep the API manifest relative to the same app directory as static icons. -->
	<link
		rel="manifest"
		href={manifestHref}
		crossorigin="use-credentials"
	/>
	<link
		rel="shortcut icon"
		href={$settingsPub?.id && $settingsPub?.favicon
			? `${backendUrl}api/files/settings_public/${$settingsPub?.id}/${$settingsPub?.favicon}`
			: asset('/server-mark.svg')}
	/>
	<link
		rel="apple-touch-icon"
		sizes="180x180"
		href={$settingsPub?.id && $settingsPub?.favicon
			? `${backendUrl}api/files/settings_public/${$settingsPub.id}/${$settingsPub.favicon}`
			: asset('/apple-touch-icon.png') + '?v=server-monitor-1'}
	/>
	{#if $settingsPub === undefined}
		<title>UpSnap</title>
	{:else}
		<title>{$settingsPub.website_title === '' ? 'UpSnap' : $settingsPub.website_title}</title>
	{/if}
</svelte:head>

{#if authIsValid && !page.url.pathname.startsWith(resolve('/welcome'))}
	<Navbar />
{/if}

<Toaster position="bottom-center" {toastOptions} />

<Transition url={page.url}>
	<div class="container mx-auto mb-4 p-2">
		{@render children()}
	</div>
</Transition>
