<script lang="ts">
	import { onMount } from 'svelte';
	import { pocketbase } from '$lib/stores/pocketbase';
	import { enrollPasskey, passkeyRequest, passkeyError } from '$lib/helpers/passkeys';
	import toast from 'svelte-french-toast';
	let enabled = $state(false);
	let busy = $state(false);
	let label = $state('Apple Passwords');
	let keys = $state<{ id: string; label: string; last_used: string }[]>([]);
	async function refresh() {
		keys = await passkeyRequest('', $pocketbase.authStore.token);
	}
	onMount(async () => {
		try {
			enabled = (await passkeyRequest('/config')).enabled;
			if (enabled && $pocketbase.authStore.record?.collectionName === 'users') await refresh();
		} catch (error) {
			toast.error(passkeyError(error));
		}
	});
	async function add() {
		busy = true;
		try {
			await enrollPasskey($pocketbase, label);
			await refresh();
			toast.success('Passkey added');
		} catch (error) {
			toast.error(passkeyError(error));
		} finally {
			busy = false;
		}
	}
	async function remove(id: string) {
		busy = true;
		try {
			await passkeyRequest(`/${id}`, $pocketbase.authStore.token, 'DELETE');
			await refresh();
		} catch (error) {
			toast.error(passkeyError(error));
		} finally {
			busy = false;
		}
	}
</script>

{#if enabled}
	<section class="card bg-base-200 mb-6 shadow-sm" aria-label="Passkeys">
		<div class="card-body gap-3">
			<h2 class="card-title">Passkeys</h2>
			{#if $pocketbase.authStore.record?.collectionName === 'users'}
				<p>
					Sign in with Touch ID, Face ID, or your device PIN. Save your passkey in Apple Passwords
					when prompted.
				</p>
				<div class="flex flex-wrap items-center gap-2">
					<input class="input" aria-label="Passkey label" bind:value={label} maxlength="80" />
					<button class="btn btn-outline" disabled={busy || keys.length >= 10} onclick={add}
						>{busy ? 'Please wait…' : 'Add passkey'}</button
					>
				</div>
				{#each keys as key (key.id)}
					<div class="border-base-300 flex items-center justify-between gap-3 border-t pt-2">
						<span
							>{key.label}<small class="ml-2 opacity-60"
								>{key.last_used
									? `Last used ${new Date(key.last_used).toLocaleString()}`
									: 'Not used yet'}</small
							></span
						>
						<button
							class="btn btn-sm btn-ghost"
							disabled={busy}
							onclick={() => remove(key.id)}
							aria-label={`Remove ${key.label}`}>Remove</button
						>
					</div>
				{:else}<p class="text-sm opacity-70">
						No passkeys registered. Keep your password for recovery.
					</p>{/each}
			{:else}<p>
					Passkeys are available for regular user accounts. Superuser access keeps password login
					for recovery.
				</p>{/if}
		</div>
	</section>
{/if}
