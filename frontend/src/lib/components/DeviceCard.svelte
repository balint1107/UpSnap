<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { nextCronDate } from '$lib/helpers/cron';
	import { readPowerResponse } from '$lib/helpers/power';
	import { m } from '$lib/paraglide/messages';
	import { dateFnsLocale } from '$lib/stores/locale';
	import { backendUrl, permission, pocketbase } from '$lib/stores/pocketbase';
	import { type Device } from '$lib/types/device';
	import {
		faBed,
		faCircleArrowDown,
		faCircleArrowUp,
		faLock,
		faPen,
		faArrowsRotate,
		faTerminal,
		faChevronDown
	} from '@fortawesome/free-solid-svg-icons';
	import { formatDistance, parseISO } from 'date-fns';
	import Fa from 'svelte-fa';
	import toast from 'svelte-french-toast';
	import DeviceCardNic from './DeviceCardNic.svelte';
 import DeviceBoot from './DeviceBoot.svelte';
 import {bootStatuses} from '$lib/stores/boot';
	import DeviceMetrics from './DeviceMetrics.svelte';
	import ServiceAvailability from './ServiceAvailability.svelte';
	import { refreshServices, serviceStatus } from '$lib/stores/services';
	import { deviceInventory } from '$lib/stores/inventory';
	import { computerDeviceID } from '$lib/types/inventory';
	import type { HostMetrics } from '$lib/types/metrics';

	let { device = $bindable() }: { device: Device } = $props();
	let expanded = $state(false);
	let metrics: HostMetrics | undefined = $state();
	let metricsFailed = $state(false);
	let refreshing = $state(false);
 const bootPowerUnavailable=$derived(!!device.boot_managed && ((!!$bootStatuses[device.id]?.job && !['ready','failed'].includes($bootStatuses[device.id].job!.stage)) || (device.status==='online' && $bootStatuses[device.id]?.os!=='ubuntu')));
	const canReboot = $derived(
		($pocketbase.authStore.isSuperuser || $permission.power?.includes(device.id)) &&
			!device.boot_managed && (device.reboot_only || device.shutdown_cmd !== '')
	);
	async function refreshDevice() {
		if (refreshing) return;
		refreshing = true;
		try {
			device = await $pocketbase
				.collection('devices')
				.getOne<Device>(device.id, { expand: 'ports,groups' });
			if (device.status === 'online') {
				const response = await fetch(`${backendUrl}api/upsnap/stats/${device.id}`, {
					headers: { Authorization: $pocketbase.authStore.token },
					cache: 'no-store'
				});
				if (!response.ok) throw new Error('Resource readings unavailable');
				const reading = (await response.json()) as HostMetrics;
				metrics = reading.available ? reading : undefined;
				metricsFailed = !reading.available;
			} else {
				metrics = undefined;
				metricsFailed = false;
			}
		} catch (error) {
			toast.error(error instanceof Error ? error.message : 'Could not refresh device information');
		} finally {
			refreshServices();
			refreshing = false;
		}
	}
	const common = $derived($deviceInventory.devices.find((entry) => entry.id === computerDeviceID(device.id)));
	const hostServices = $derived($serviceStatus.services.filter((s) => s.device_id === computerDeviceID(device.id)));
	let modalReboot: HTMLDialogElement;
	let modalRename: HTMLDialogElement;
	let newName = $state('');
	let savingName = $state(false);
	let renameError = $state('');
	const canRename = $derived(
		$pocketbase.authStore.isSuperuser ||
			($permission.read?.includes(device.id) &&
				($permission.power?.includes(device.id) || $permission.update?.includes(device.id)))
	);

	function editName() {
		newName = device.name;
		renameError = '';
		modalRename.showModal();
	}

	async function saveName(event: SubmitEvent) {
		event.preventDefault();
		if (savingName) return;
		savingName = true;
		renameError = '';
		try {
			const response = await fetch(`${backendUrl}api/upsnap/rename/${device.id}`, {
				method: 'POST',
				headers: { Authorization: $pocketbase.authStore.token, 'Content-Type': 'application/json' },
				body: JSON.stringify({ name: newName.trim() })
			});
			const result = await response.json();
			if (!response.ok) throw new Error(result.message || 'Could not save device name.');
			device = { ...device, ...result };
			modalRename.close();
		} catch (error) {
			renameError = error instanceof Error ? error.message : 'Could not save device name.';
		} finally {
			savingName = false;
		}
	}
	let moreButtons = $derived([
		{
			text: m.device_card_btn_more_sleep(),
			icon: faBed,
			onClick: () => sleep(),
			requires:
				($pocketbase.authStore.isSuperuser || $permission.power?.includes(device.id)) &&
				device.status === 'online' &&
				!device.reboot_only &&
				device.sol_enabled
		},
		{
			text: m.device_card_btn_more_edit(),
			icon: faPen,
			onClick: () => goto(resolve(`/device?id=${device.id}`)),
			requires: $pocketbase.authStore.isSuperuser || $permission.update?.includes(device.id)
		}
	]);

	// update device status change
	let now = $state(Date.now());
	let interval: number;
	$effect(() => {
		clearInterval(interval);
		interval = setInterval(() => {
			now = Date.now();
		}, 1000);
	});

	function sleep() {
		fetch(`${backendUrl}api/upsnap/sleep/${device.id}`, {
			headers: {
				Authorization: $pocketbase.authStore.token
			}
		}).catch((err) => {
			toast.error(err.message);
		});
	}

	function reboot() {
		fetch(`${backendUrl}api/upsnap/reboot/${device.id}`, {
			headers: {
				Authorization: $pocketbase.authStore.token
			}
		})
			.then(readPowerResponse)
			.then((result) => {
				device = { ...device, ...result };
			})
			.catch((err) => {
				toast.error(err.message);
			});
	}

	function askRebootConfirmation() {
		if (device.shutdown_confirm) {
			modalReboot.showModal();
		} else {
			reboot();
		}
	}
</script>

<details
	class="server-card device-row bg-base-100"
	bind:open={expanded}
	data-device={device.name}
	data-inventory-id={computerDeviceID(device.id)}
	data-category={common?.category ?? 'computers'}
	data-state={device.status}
>
	<summary class="device-row-summary">
		<span class="row-host-name" title={device.name}>{device.name}{#if device.boot_managed}<DeviceBoot {device} compact />{/if}</span>
		<span class="row-host-ip">{device.ip}</span>
		<span class="host-state"
			>{device.status === 'online'
				? 'SSH ready'
				: device.status === 'pending'
					? 'Changing state'
					: 'Offline'}</span
		>
		<span class="row-service-count"
			>{hostServices.length} {hostServices.length === 1 ? 'service' : 'services'}</span
		>
		<DeviceMetrics {device} compact bind:metrics bind:failed={metricsFailed} />
		<div class="row-power">
			{#if common ? common.capabilities.includes('wake') || common.capabilities.includes('suspend') : !device.reboot_only}
				<DeviceCardNic bind:device compact powerUnavailable={bootPowerUnavailable} />
			{/if}
		</div>
		<div class="row-terminal">
			{#if device.status === 'online' && $pocketbase.authStore.isValid}
				<a class="btn host-power host-icon-button" aria-label="Open terminal for {device.name}" title="Terminal"
					href={resolve('/terminal') + '/?device=' + encodeURIComponent(device.id)}
					onclick={(event) => event.stopPropagation()} onkeydown={(event) => event.stopPropagation()}><Fa icon={faTerminal} /></a>
			{/if}
		</div>
		<span class="row-chevron" aria-hidden="true"><Fa icon={faChevronDown} /></span>
	</summary>
	{#if expanded}
		<div class="card-body p-6">
			<div class="server-heading">
				<div class="server-name">
					{#if device.link !== ''}
						<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
						<a href={device.link} target="_blank">
							<h1 class="link card-title">{device.name}</h1>
						</a>
					{:else}
						<h1 class="card-title">{device.name}</h1>
					{/if}
					{#if canRename}
						<button
							class="btn btn-ghost rename-device"
							aria-label="Edit name for {device.name}"
							title="Edit device name"
							onclick={editName}><Fa icon={faPen} /></button
						>
					{/if}
				</div>
				<span class="host-state"
					>{device.status === 'online'
						? 'SSH ready'
						: device.status === 'pending'
							? 'Changing state'
							: 'Offline'}</span
				>
			</div>
			{#if device.description}
				<p class="server-description">{device.description}</p>
			{/if}
			<div class="card rounded-box w-full">
				<DeviceCardNic bind:device powerUnavailable={bootPowerUnavailable} onReboot={canReboot ? askRebootConfirmation : undefined} />
			</div>
			{#if device.boot_managed}<DeviceBoot {device} />{/if}
			<DeviceMetrics {device} collect={false} {metrics} failed={metricsFailed} />
			<ServiceAvailability deviceId={computerDeviceID(device.id)} embedded />
			{#if device.wake_cron_enabled || device.shutdown_cron_enabled || device.password}
				<div class="mt-1 flex flex-row flex-wrap gap-2">
					{#if device.wake_cron_enabled}
						<div class="tooltip" data-tip={m.device_card_tooltip_wake_cron()}>
							<span class="badge badge-success gap-1 p-3"
								><Fa icon={faCircleArrowUp} />
								{nextCronDate(device.wake_cron)}
							</span>
						</div>
					{/if}
					{#if device.shutdown_cron_enabled}
						<div class="tooltip" data-tip={m.device_card_tooltip_shutdown_cron()}>
							<span class="badge badge-error gap-1 p-3"
								><Fa icon={faCircleArrowDown} />
								{nextCronDate(device.shutdown_cron)}
							</span>
						</div>
					{/if}
					{#if device.password}
						<div class="tooltip" data-tip={m.device_card_tooltip_wake_password()}>
							<span class="badge gap-1 p-3"><Fa icon={faLock} />{m.device_card_password()}</span>
						</div>
					{/if}
				</div>
			{/if}
			<div class="card-actions mt-auto items-center">
				<span
					class="tooltip"
					data-tip="{m.device_card_tooltip_last_status_change()}: {device.updated}"
				>
					State changed
					{formatDistance(parseISO(device.updated), now, {
						includeSeconds: true,
						addSuffix: true,
						locale: $dateFnsLocale
					})}
				</span>
				<button
					class="btn btn-sm btn-square ms-auto"
					disabled={refreshing}
					aria-label="Refresh info for {device.name}"
					title="Refresh info for {device.name}"
					onclick={refreshDevice}
					><Fa icon={faArrowsRotate} /></button
				>
				{#if moreButtons.filter((btn) => btn.requires).length > 0}
					<div class="flex flex-row flex-wrap gap-1">
						{#each moreButtons as btn, i (i)}
							{#if btn.requires}
								<div class="tooltip" data-tip={btn.text}>
									<button
										class="btn btn-sm btn-circle"
										aria-label="{btn.text} {device.name}"
										onclick={btn.onClick}
									>
										<Fa icon={btn.icon} />
									</button>
								</div>
							{/if}
						{/each}
					</div>
				{/if}
			</div>
		</div>
	{/if}
</details>

<dialog
	class="modal"
	bind:this={modalRename}
	aria-label="Edit device name"
	oncancel={(event) => {
		if (savingName) event.preventDefault();
	}}
>
	<form class="modal-box" onsubmit={saveName}>
		<h2 class="text-lg font-bold">Edit device name</h2>
		<label class="form-control mt-4 block">
			<span class="mb-2 block">Device name</span>
			<input
				class="input input-bordered w-full"
				bind:value={newName}
				required
				maxlength="100"
				disabled={savingName}
			/>
		</label>
		{#if renameError}<p class="text-error mt-2" role="alert">{renameError}</p>{/if}
		<div class="modal-action">
			<button type="button" class="btn" disabled={savingName} onclick={() => modalRename.close()}
				>Cancel</button
			>
			<button type="submit" class="btn btn-primary" disabled={savingName || !newName.trim()}
				>{savingName ? 'Saving…' : 'Save'}</button
			>
		</div>
	</form>
</dialog>

<dialog class="modal" bind:this={modalReboot}>
	<div class="modal-box">
		<h3 class="text-lg font-bold">
			Reboot {device.name}?
		</h3>
		<p class="py-4">
			{device.reboot_only
				? 'Restart this host. The dashboard and other services on it will be briefly unavailable.'
				: 'Restart this host by suspending it and sending Wake-on-LAN.'}
		</p>
		<div class="modal-action">
			<form method="dialog">
				<button class="btn">{m.buttons_cancel()}</button>
				<button class="btn btn-success" onclick={reboot}>{m.buttons_confirm()}</button>
			</form>
		</div>
	</div>
</dialog>
