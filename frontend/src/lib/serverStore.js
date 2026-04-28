import { writable } from 'svelte/store'

export const serverProcIds = writable({})
export const triggerPublicIP = writable(0)
export const publicIPClosedSig = writable(0)