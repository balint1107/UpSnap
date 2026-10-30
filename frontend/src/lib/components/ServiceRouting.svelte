<script lang="ts">
 import { onMount } from 'svelte';
 import { backendUrl, pocketbase } from '$lib/stores/pocketbase';
 import { refreshServices } from '$lib/stores/services';
 type Route = { id:string; name:string; route:string; canonical:string; access:string };
 let services = $state<Route[]>([]);
 let revision = $state(0);
 let saving = $state('');
 let message = $state('');
 let permitted = $state(false);
 let stopped = false;
 let retry: ReturnType<typeof setTimeout> | undefined;
 /** Load administrator-only route metadata; a denied request keeps this panel hidden. */
 async function load() {
  const response = await fetch(`${backendUrl}api/upsnap/service-routes`, { headers:{Authorization:$pocketbase.authStore.token} });
  if (!response.ok) {
   if (response.status === 503 && !stopped) {
    clearTimeout(retry);
    retry = setTimeout(() => { void load().catch(() => {}); }, 3000);
   }
   return;
  }
  if (stopped) return;
  const result = await response.json();
  services = result.services; revision = result.revision; permitted = true;
 }
 /** Apply one service's bounded prefix/policy with revision protection and report activation failures. */
 async function save(service:Route) {
  saving = service.id; message = '';
  try {
   const response = await fetch(`${backendUrl}api/upsnap/service-routes`, {method:'PUT', headers:{Authorization:$pocketbase.authStore.token,'Content-Type':'application/json'},body:JSON.stringify({id:service.id,route:service.route,access:service.access,revision})});
   const result = await response.json();
   if (!response.ok) throw new Error(result.error || result.message || 'Settings could not be applied');
   services = result.services; revision = result.revision; message = 'Settings applied'; refreshServices();
  } catch(error) {message = error instanceof Error ? error.message : 'Settings could not be applied';}
  finally {saving = '';}
 }
 onMount(() => {
  const unsubscribe = $pocketbase.authStore.onChange(() => {
   if ($pocketbase.authStore.isValid) void load().catch(() => {});
   else permitted = false;
  }, true);
  return () => { stopped = true; clearTimeout(retry); unsubscribe(); };
 });
</script>

{#if permitted}
<details class="routing-settings">
 <summary>Service access and routes</summary>
 <div class="routing-list">
 {#each services as service (service.id)}
 <form onsubmit={(event) => {event.preventDefault(); void save(service);}}>
  <strong>{service.name}</strong>
  <label>Access<select class="select select-sm" bind:value={service.access} aria-label={`${service.name} access`}>
   <option value="authenticated">Anywhere · sign-in required</option>
   <option value="lan_vpn">LAN/VPN · sign-in required</option>
  </select></label>
  <label>Route<input class="input input-sm" aria-label={`${service.name} route`} bind:value={service.route} required maxlength="42" pattern={"/[a-z][a-z0-9\\-]{0,39}/"} /></label>
  <button class="btn btn-sm" disabled={!!saving}>{saving === service.id ? 'Applying…':'Apply'}</button>
 </form>
 {/each}
 </div>
 {#if message}<p role="status">{message}</p>{/if}
</details>
{/if}

<style>
 .routing-settings{border:1px solid #d8e1eb;border-radius:8px;padding:0.8rem 1rem;margin:0.8rem 0 1.2rem;background:white}
 summary{cursor:pointer;font-weight:600}
 .routing-list{display:grid;gap:0.8rem;margin-top:1rem}
 form{display:grid;grid-template-columns:minmax(110px,1fr) minmax(180px,2fr) minmax(120px,1fr) auto;gap:0.75rem;align-items:end}
 strong{align-self:center} label{display:grid;gap:0.25rem;font-size:0.78rem}
 input,select{width:100%;max-width:100%} p{font-size:0.8rem;margin-top:0.75rem}
 @media(max-width:700px){form{grid-template-columns:1fr auto}strong{grid-column:1/-1}label:nth-of-type(1){grid-column:1/-1}}
</style>
