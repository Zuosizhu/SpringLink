export function reindexStore(store, index) {
  const newStore = {}
  for (const k of Object.keys(store)) {
    const ki = parseInt(k)
    if (ki < index) newStore[ki] = store[ki]
    else if (ki > index) newStore[ki - 1] = store[ki]
  }
  return newStore
}
