<script lang="ts">
	import { deviceCategories } from '$lib/types/inventory';
	import { browser } from '$app/environment';
	import { resolve } from '$app/paths';
	import DeviceCard from '$lib/components/DeviceCard.svelte';
	import PowerDevices from '$lib/components/PowerDevices.svelte';
	import RouterDevices from '$lib/components/RouterDevices.svelte';
	import PendingDeviceRow from '$lib/components/PendingDeviceRow.svelte';
	import { deviceInventory, refreshInventory } from '$lib/stores/inventory';
	import { m } from '$lib/paraglide/messages';
	import { localeStore } from '$lib/stores/locale';
	import { backendUrl, permission, pocketbase } from '$lib/stores/pocketbase';
	import type { Device, Group } from '$lib/types/device';
	import {
		faChevronCircleLeft,
		faChevronCircleRight,
		faPlus,
		faPowerOff,
		faWarning
	} from '@fortawesome/free-solid-svg-icons';
	import { onMount } from 'svelte';
	import Fa from 'svelte-fa';
	import toast from 'svelte-french-toast';

	let computerFailed = $state(false);
	const computers = $derived($deviceInventory.devices.filter((d) => d.driver === 'computer'));
	let devices: Device[] = $state([]);
	let orderBy: 'name' | 'ip' = $state('name');
	let orderExpanded = $state(false);
	let orderByGroups = $state(true);
	let searchQuery = $state('');
	let searchInput: HTMLInputElement | undefined = $state();
	let modalShutdownGroup: HTMLDialogElement | undefined = $state();
	let pendingShutdownGroup: Group | null = $state(null);
	let deviceRequest = 0;
	let filteredDevices: Device[] = $derived([]);
	let devicesWithoutGroups: Device[] = $derived([]);
	let devicesWithGroup: {
		group: Group | null;
		devices: Device[];
	}[] = $derived([]);

	const isMac = browser && /Mac|iPhone|iPad/.test(navigator.userAgent);
	const gridClass = 'device-rows';

	if (browser) {
		const storedOrderBy = localStorage.getItem('orderBy');
		if (storedOrderBy === 'name' || storedOrderBy === 'ip') {
			orderBy = storedOrderBy;
		}
		orderByGroups = localStorage.getItem('orderByGroups') !== 'false';
	}

	$effect(() => {
		if (browser) {
			localStorage.setItem('orderBy', orderBy);
		}
	});
	$effect(() => {
		if (browser) {
			localStorage.setItem('orderByGroups', String(orderByGroups));
		}
	});
	$effect(() => {
		filteredDevices = devices.filter(
			(dev) =>
				computers.some((common) => common.driver_id === dev.id) &&
				(dev.ip.includes(searchQuery.toLowerCase()) ||
					dev.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
					dev.mac.toLowerCase().includes(searchQuery.toLowerCase()) ||
					dev.description.toLowerCase().includes(searchQuery.toLowerCase()))
		);
	});
	$effect(() => {
		devicesWithoutGroups = filteredDevices
			.filter((dev) => dev.groups.length === 0)
			.sort((a, b) => a[orderBy].localeCompare(b[orderBy], $localeStore, { numeric: true }));
	});
	$effect(() => {
		const groups: Record<string, { group: Group; devices: Device[] }> = {};
		filteredDevices.forEach((dev) => {
			if (dev.expand?.groups?.length) {
				for (const g of dev.expand.groups) {
					if (!groups[g.id]) groups[g.id] = { group: g, devices: [] };
					groups[g.id].devices.push(dev);
				}
			}
		});
		devicesWithGroup = Object.values(groups).map((g) => ({
			group: g.group,
			devices: g.devices.sort((a, b) =>
				a[orderBy].localeCompare(b[orderBy], $localeStore, { numeric: true })
			)
		}));
	});

	/** Load stored computer details independently of inventory; silent suppresses toast errors. Returns immediately. */
	function getAllDevices(silent = false) {
		const request = ++deviceRequest;
		$pocketbase
			.collection('devices')
			.getFullList(-1, { sort: 'name', expand: 'ports,groups' })
			.then((resp) => {
				if (request === deviceRequest) {
					devices = resp as Device[];
					computerFailed = false;
				}
			})
			.catch((err) => {
				if (request === deviceRequest) {
					computerFailed = true;
					if (!silent) toast.error(err.message);
				}
			});
	}

	function handleShortcut(event: KeyboardEvent) {
		if ((event.ctrlKey || event.metaKey) && event.key === 'k') {
			event.preventDefault();
			searchInput?.focus();
		}
	}

	function wakeGroup(group: string) {
		fetch(`${backendUrl}api/upsnap/wakegroup/${group}`, {
			headers: {
				Authorization: $pocketbase.authStore.token
			}
		}).catch((err) => {
			toast.error(err.message);
		});
	}

	function askShutdownGroupConfirmation(group: Group) {
		pendingShutdownGroup = group;
		modalShutdownGroup?.showModal();
	}

	function executeShutdownGroup() {
		if (pendingShutdownGroup) {
			fetch(`${backendUrl}api/upsnap/shutdowngroup/${pendingShutdownGroup.id}`, {
				headers: {
					Authorization: $pocketbase.authStore.token
				}
			}).catch((err) => {
				toast.error(err.message);
			});
		}
	}

	onMount(() => {
		orderBy = (localStorage.getItem('orderBy') as 'name' | 'ip') || 'name';
		orderByGroups = localStorage.getItem('orderByGroups') !== 'false';
		getAllDevices();
		let identity = $pocketbase.authStore.record?.id;
		const stopAuth = $pocketbase.authStore.onChange(() => {
			const next = $pocketbase.authStore.isValid ? $pocketbase.authStore.record?.id : undefined;
			if (identity === next) return;
			identity = next;
			++deviceRequest;
			devices = [];
			if ($pocketbase.authStore.isValid) getAllDevices(true);
		});
		const subscriptions = ['devices', 'ports', 'permissions'].map((collection) =>
			$pocketbase.collection(collection).subscribe('*', () => {
				refreshInventory();
				getAllDevices();
			})
		);
		const refreshIfVisible = () => {
			if (document.visibilityState === 'visible') getAllDevices(true);
		};
		const timer = window.setInterval(refreshIfVisible, 30000);
		document.addEventListener('visibilitychange', refreshIfVisible);
		return () => {
			++deviceRequest;
			stopAuth();
			window.clearInterval(timer);
			document.removeEventListener('visibilitychange', refreshIfVisible);
			subscriptions.forEach((subscription) =>
				subscription.then((unsubscribe) => unsubscribe()).catch(() => {})
			);
		};
	});
</script>

<svelte:document onkeydown={handleShortcut} />

{#if !$deviceInventory.loaded || $deviceInventory.devices.length > 0}
	<div class="monitor-header">
		<div>
			<div class="monitor-eyebrow">Home infrastructure</div>
			<h1>Devices</h1>
		</div>
		<div class="monitor-summary" aria-label="Computer status summary">
			<span><strong>{computers.length}</strong> computers</span>
			<span><strong>{computers.filter((d) => d.state === 'online').length}</strong> SSH ready</span>
			<span><strong>{computers.filter((d) => d.state === 'offline').length}</strong> offline</span>
		</div>
	</div>
	<div class="mb-4 flex flex-col justify-between gap-4 md:flex-row">
		<label class="input max-md:w-full">
			<svg class="h-[1em] opacity-50" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"
				><g
					stroke-linejoin="round"
					stroke-linecap="round"
					stroke-width="2.5"
					fill="none"
					stroke="currentColor"
					><circle cx="11" cy="11" r="8"></circle><path d="m21 21-4.3-4.3"></path></g
				></svg
			>
			<input
				type="search"
				class="grow"
				placeholder={m.home_search_placeholder()}
				bind:value={searchQuery}
				bind:this={searchInput}
			/>
			<kbd class="kbd kbd-sm">{isMac ? '⌘' : 'CTRL'}</kbd>
			<kbd class="kbd kbd-sm">K</kbd>
		</label>
		<div class="join ms-auto">
			{#if orderExpanded}
				<button class="btn join-item outline-0!" onclick={() => (orderByGroups = !orderByGroups)}>
					{m.home_order_groups()}
					<input type="checkbox" class="checkbox" checked={orderByGroups} />
				</button>
				<input
					class="btn join-item outline-0!"
					type="radio"
					name="order"
					aria-label={m.home_order_name()}
					bind:group={orderBy}
					value="name"
				/>
				<input
					class="btn join-item outline-0!"
					type="radio"
					name="order"
					aria-label={m.home_order_ip()}
					bind:group={orderBy}
					value="ip"
				/>
			{/if}
			<button
				class="btn join-item tooltip {orderExpanded ? '' : 'rounded-field'}"
				data-tip={m.home_order_tooltip()}
				onclick={() => (orderExpanded = !orderExpanded)}
			>
				<Fa icon={orderExpanded ? faChevronCircleRight : faChevronCircleLeft} size="lg" />
			</button>
		</div>
	</div>
	<h2 class="mb-3 text-lg font-semibold">{deviceCategories.computers}</h2>
	{#if !$deviceInventory.loaded}<p role="status">Loading devices…</p>{/if}
	<div class={gridClass}>
		{#each computers.filter((d) => !devices.some((record) => record.id === d.driver_id) && `${d.name} ${d.address}`
					.toLowerCase()
					.includes(searchQuery.toLowerCase())) as common (common.id)}
			<PendingDeviceRow device={common} failed={computerFailed} />
		{/each}
	</div>
	{#if orderByGroups}
		<div class="space-y-6">
			{#if devicesWithoutGroups.length > 0}
				<div class={gridClass}>
					{#each devicesWithoutGroups as device (device.id)}
						{@const common = computers.find((d) => d.driver_id === device.id)}
						{#if common && devices.some((record) => record.id === device.id)}
							<DeviceCard
								{common}
								bind:device={devices[devices.findIndex((d) => d.id === device.id)]}
							/>
						{/if}
					{/each}
				</div>
			{/if}
			{#each devicesWithGroup.sort( (a, b) => (a.group?.name ?? '').localeCompare( b.group?.name ?? '', $localeStore, { numeric: true } ) ) as { group, devices: groupDevices } (group?.id)}
				<div>
					<h1 class="mb-3 flex flex-wrap items-center gap-3 text-2xl font-bold">
						<span>{group?.name ?? 'Unknown group name'}</span>
						<button
							class="btn btn-sm btn-success btn-soft"
							onclick={() => wakeGroup(group?.id ?? '')}
							><Fa icon={faPowerOff} /> {m.home_wake_group()}</button
						>
						<button
							class="btn btn-sm btn-error btn-soft"
							onclick={() => group && askShutdownGroupConfirmation(group)}
							><Fa icon={faPowerOff} /> {m.home_shutdown_group()}</button
						>
					</h1>
					<div class={gridClass}>
						{#each groupDevices as device (device.id)}
							{@const common = computers.find((d) => d.driver_id === device.id)}
							{#if common && devices.some((record) => record.id === device.id)}
								<DeviceCard
									{common}
									bind:device={devices[devices.findIndex((d) => d.id === device.id)]}
								/>
							{/if}
						{/each}
					</div>
				</div>
			{/each}
		</div>
	{:else}
		{@const sorted = $state
			.snapshot(filteredDevices)
			.sort((a, b) => a[orderBy].localeCompare(b[orderBy], $localeStore, { numeric: true }))}
		<div class={gridClass}>
			{#each sorted as device (device.id)}
				{@const common = computers.find((d) => d.driver_id === device.id)}
				{#if common && devices.some((record) => record.id === device.id)}
					<DeviceCard
						{common}
						bind:device={devices[devices.findIndex((d) => d.id === device.id)]}
					/>
				{/if}
			{/each}
		</div>
	{/if}
{:else}
	<div class="flex justify-center">
		<div role="alert" class="alert alert-horizontal w-fit">
			<Fa icon={faWarning} size="lg" />
			{#if $pocketbase.authStore.isSuperuser || $permission.create}
				<div>
					<h3 class="font-bold">{m.home_no_devices()}</h3>
					<div class="text-xs">{m.home_add_first_device()}</div>
				</div>
				<a href={resolve('/device/new')} class="btn btn-sm"
					><Fa icon={faPlus} />{m.home_add_first_device()}</a
				>
			{:else}
				<span>
					{m.home_grant_permissions()}
				</span>
			{/if}
		</div>
	</div>
{/if}

{#if $deviceInventory.failed}<p role="alert">
		Device inventory unavailable. Previously loaded devices may be stale.
	</p>{/if}
{#if $deviceInventory.unavailableDrivers.length}<p role="status">
		Some device information is unavailable. Existing readings may be stale.
	</p>{/if}
<RouterDevices search={searchQuery} /><PowerDevices search={searchQuery} />

<dialog class="modal" bind:this={modalShutdownGroup}>
	<div class="modal-box">
		<h3 class="text-lg font-bold">
			{m.home_modal_confirm_shutdown_group_title({ group: pendingShutdownGroup?.name ?? '' })}
		</h3>
		<p class="py-4">
			{m.home_modal_confirm_shutdown_group_desc({ group: pendingShutdownGroup?.name ?? '' })}
		</p>
		<div class="modal-action">
			<form method="dialog">
				<button class="btn">{m.buttons_cancel()}</button>
				<button class="btn btn-error" onclick={executeShutdownGroup}>{m.buttons_confirm()}</button>
			</form>
		</div>
	</div>
</dialog>
