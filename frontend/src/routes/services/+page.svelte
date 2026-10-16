<script lang="ts">
	import ServiceAvailability from '$lib/components/ServiceAvailability.svelte';
</script>

<div class="service-page"><ServiceAvailability /></div>

<style>
	.service-page :global(.services-section) {
		margin-top: 0;
		border-top: 0;
		padding-top: 1rem;
	}
</style>
