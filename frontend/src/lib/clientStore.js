import { writable } from 'svelte/store'

export const clientProcIds = writable({})
export const clientAddrs = writable({})
export const triggerImport = writable(false)
