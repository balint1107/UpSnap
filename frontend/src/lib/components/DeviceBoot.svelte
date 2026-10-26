<script lang="ts">
 import {onMount} from 'svelte';
 import {get} from 'svelte/store';
 import {backendUrl,pocketbase,permission} from '$lib/stores/pocketbase';
 import {bootStatuses,refreshBoot} from '$lib/stores/boot';
 import type {Device} from '$lib/types/device';
 import Fa from 'svelte-fa';
 import {faCheck,faChevronDown} from '@fortawesome/free-solid-svg-icons';
 let {device,compact=false}:{device:Device;compact?:boolean}=$props();
 const status=$derived($bootStatuses[device.id]);
 const canPower=$derived($pocketbase.authStore.isSuperuser || $permission.power?.includes(device.id));
 const name=$derived(status?.os==='windows'?'Windows 11':status?.os==='ubuntu'?'Ubuntu':'OS unknown');
 let busy=$state(false);
 let message=$state('');
 let restartTarget=$state('');
 let selectedOS=$state<'ubuntu'|'windows'|''>('');
 const target=$derived(selectedOS || (status?.os==='windows'?'windows':'ubuntu'));
 let restartDialog=$state<HTMLDialogElement>(null!);
 let osCard=$state<HTMLDetailsElement>(null!);
 const switching=$derived(!!status?.job && !['ready','failed'].includes(status.job.stage));
 const runningOS=$derived(device.status==='online'&&status?.ready&&!switching?status.os:'unknown');

 /** Select fixed OS choice (ubuntu/windows), clear old validation, close the list and focus its header; returns nothing. */
 function selectTarget(choice:'ubuntu'|'windows'){selectedOS=choice;message='';osCard.open=false;osCard.querySelector('summary')?.focus();}

 /** Show confirmation for a fixed OS target; never sends a request until the user confirms. */
 function askRestart(target:string){restartTarget=target;restartDialog.showModal();}

 /** Request the confirmed target's graceful restart, then refresh persisted progress; no automatic retry. */
 async function restart(){
  if(busy||switching||!canPower)return;
  restartDialog.close();busy=true;message='';
  try{
   const response=await fetch(backendUrl+'api/upsnap/boot/'+encodeURIComponent(device.id)+'/restart',{method:'POST',headers:{Authorization:get(pocketbase).authStore.token,'Content-Type':'application/json'},body:JSON.stringify({target:restartTarget})});
   const result=await response.json();if(!response.ok)throw new Error(result.message||'Restart request failed.');
  }catch(error){message=error instanceof Error?error.message:'Restart request failed.';}finally{await refreshBoot(device.id,true);busy=false;}
 }

 /** Validate fixed target (ubuntu/windows) for this device; resolves after updating message/status, never prepares or restarts. */
 async function validate(target:string) {
  if(busy||!canPower)return;
  busy=true;message='';
  try {
   const response=await fetch(backendUrl+'api/upsnap/boot/'+encodeURIComponent(device.id)+'/validate',{method:'POST',headers:{Authorization:get(pocketbase).authStore.token,'Content-Type':'application/json'},body:JSON.stringify({target})});
   const result=await response.json();
   if(!response.ok)throw new Error(result.message||'Target validation failed.');
   message=`${target==='windows'?'Windows 11':'Ubuntu'} loader verified (Boot${result.targets[target].entry}). Boot settings unchanged.`;
   await refreshBoot(device.id,true);
  }catch(error){message=error instanceof Error?error.message:'Target validation failed.';}finally{busy=false;}
 }

 /** Mount without arguments; poll active jobs every three seconds, idle hosts every 30, and return timer cleanup. */
 onMount(function mountBoot(){void refreshBoot(device.id);let tick=0;const timer=setInterval(()=>{tick++;if(!document.hidden&&(switching||tick%10===0))void refreshBoot(device.id,switching);},3000);return()=>clearInterval(timer);});
</script>

{#if compact}
 <span class="os-badge">{name}{switching?' · Restarting…':''}</span>
{:else}
 <section class="boot-panel" aria-label="Operating system for {device.name}">
  <details class="boot-card" bind:this={osCard}>
   <summary class="boot-summary" aria-label="Operating system selection">
    <span class="boot-selected"><span class="boot-check" aria-hidden="true" title={runningOS===target?'Currently running OS':undefined}>{#if runningOS===target}<Fa icon={faCheck} />{/if}</span><strong>{target==='windows'?'Windows 11':'Ubuntu'}</strong><span class="boot-entry">{status?.targets[target]?.available?`Boot${status.targets[target].entry}`:'Not verified'}</span></span>
    <div class="boot-actions">
     <button class="btn btn-sm" aria-label="Validate {target==='windows'?'Windows':'Ubuntu'}" disabled={!canPower||busy||switching||!status?.ready} onclick={(event)=>{event.preventDefault();event.stopPropagation();void validate(target);}} onkeydown={(event)=>event.stopPropagation()}>Validate</button>
     <button class="btn btn-sm" aria-label="Restart into {target==='windows'?'Windows':'Ubuntu'}" disabled={!canPower||busy||switching||!status?.restart_enabled||!!status?.boot_next||!status?.targets[target]?.available} onclick={(event)=>{event.preventDefault();event.stopPropagation();askRestart(target);}} onkeydown={(event)=>event.stopPropagation()}>Restart</button>
    </div>
    <span class="boot-chevron" aria-hidden="true"><Fa icon={faChevronDown} /></span>
   </summary>
   <div class="boot-options" role="group" aria-label="Operating system choices">
    {#each ['ubuntu','windows'] as choice}
     <button type="button" class="boot-option" class:selected={choice===target} aria-pressed={choice===target} title={runningOS===choice?'Currently running OS':undefined} disabled={busy||switching} onclick={()=>selectTarget(choice as 'ubuntu'|'windows')}>
      <span class="boot-check" aria-hidden="true" title={runningOS===choice?'Currently running OS':undefined}>{#if choice===runningOS}<Fa icon={faCheck} />{/if}</span><span>{choice==='windows'?'Windows 11':'Ubuntu'}</span><span class="boot-entry">{status?.targets[choice]?.available?`Boot${status.targets[choice].entry}`:'Not verified'}</span>
     </button>
    {/each}
   </div>
  </details>
  {#if !status?.restart_enabled && !switching}<p class="boot-lock">Boot switching is unavailable.</p>{/if}
  {#if status?.boot_next && !switching}<p class="boot-warning">Pending one-time boot selection: Boot{status.boot_next}. Review before another switch.</p>{/if}
  {#if status?.job}<p role="status">{status.job.message}</p>{/if}
  {#if status?.message}<p>{status.message}</p>{/if}
  {#if message}<p role="status">{message}</p>{/if}
 </section>
 <dialog bind:this={restartDialog} class="modal"><div class="modal-box"><h3 class="text-lg font-bold">Restart into {restartTarget==='windows'?'Windows 11':'Ubuntu'}?</h3><p class="py-4">This gracefully restarts {device.name} and disconnects active sessions. Save your work first.</p><div class="modal-action"><button class="btn" onclick={()=>restartDialog.close()}>Cancel</button><button class="btn btn-primary" onclick={restart}>Restart</button></div></div></dialog>
{/if}
<style>
 .os-badge{display:block;font-size:.65rem;line-height:1rem;font-weight:500;color:var(--monitor-muted);}
 .boot-panel{border-top:1px solid var(--monitor-border);padding-top:1rem;font-size:.8rem;}
 .boot-card{border:1px solid var(--monitor-border);border-radius:6px;}
 .boot-summary{display:grid;grid-template-columns:minmax(0,1fr) auto 1rem;align-items:center;gap:.5rem;padding:.75rem;cursor:pointer;list-style:none;}
 .boot-summary::-webkit-details-marker{display:none;}
 .boot-summary:focus-visible{outline:2px solid currentColor;outline-offset:2px;}
 .boot-chevron{display:flex;justify-content:center;font-size:.65rem;transition:transform .15s ease;}
 .boot-card[open] .boot-chevron{transform:rotate(180deg);}
 .boot-selected{display:flex;align-items:center;gap:.35rem;flex-wrap:wrap;min-width:0;}
 .boot-check{display:inline-flex;align-items:center;justify-content:center;width:.7rem;flex:none;color:#32856b;font-size:.65rem;}
 .boot-entry{color:var(--monitor-muted);font-family:ui-monospace,monospace;font-size:.7rem;white-space:nowrap;}
 .boot-options{display:grid;border-top:1px solid var(--monitor-border);padding:.35rem;gap:.2rem;}
 .boot-option{display:flex;align-items:center;gap:.75rem;width:100%;text-align:left;padding:.6rem;border-radius:4px;cursor:pointer;}
 .boot-option:hover,.boot-option.selected{background:color-mix(in srgb,var(--monitor-border) 40%,transparent);}
 .boot-option:focus-visible{outline:2px solid currentColor;outline-offset:-2px;}
 .boot-option:disabled{opacity:.5;cursor:default;}
 .boot-lock,.boot-panel p{color:var(--monitor-muted);margin-top:.5rem;}
 .boot-actions{display:flex;gap:.5rem;flex-wrap:wrap;}
</style>
