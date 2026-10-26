import { writable, get } from 'svelte/store';
import { backendUrl, pocketbase } from './pocketbase';
export type BootStatus = { os: string; ready: boolean; writes_enabled: boolean; restart_enabled: boolean; boot_next: string | null; boot_order_unchanged: boolean; message?: string; validated_target?: string; job?: {target:string;stage:string;message:string}; targets: Record<string,{available:boolean;entry:string;path:string}> };
export const bootStatuses = writable<Record<string,BootStatus>>({});
const last = new Map<string,number>();
const loading = new Map<string,Promise<void>>();

/** Refresh status for device id (record ID); force bypasses the 15-second client cache. Returns completion; never writes firmware. */
export function refreshBoot(id: string, force = false): Promise<void> {
 if (loading.has(id)) return loading.get(id)!;
 if (!force && Date.now() - (last.get(id) || 0) < 15000) return Promise.resolve();
 const task = (async () => {
  try {
   const response = await fetch(backendUrl + 'api/upsnap/boot/' + encodeURIComponent(id), {headers:{Authorization:get(pocketbase).authStore.token}});
   if (!response.ok) throw new Error('Boot information unavailable.');
   const value:BootStatus = await response.json();
   bootStatuses.update(status=>({...status,[id]:value}));
   last.set(id,Date.now());
  } catch {
   bootStatuses.update(status=>({...status,[id]:{os:'unknown',ready:false,writes_enabled:false,restart_enabled:false,boot_next:null,boot_order_unchanged:false,targets:{},message:'Boot information unavailable.'}}));
  } finally {loading.delete(id);}
 })();
 loading.set(id,task);return task;
}
