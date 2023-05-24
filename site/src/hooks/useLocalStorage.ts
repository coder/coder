interface UseLocalStorage {
  saveLocal: (arg0: string, arg1: string) => void
  getLocal: (arg0: string) => string | undefined
  clearLocal: (arg0: string) => void
}

export const useLocalStorage = (): UseLocalStorage => {
  return {
    saveLocal,
    getLocal,
    clearLocal,
  }
}

const saveLocal = (itemKey: string, itemValue: string): void => {
  window.localStorage.setItem(itemKey, itemValue)
}

const getLocal = (itemKey: string): string | undefined => {
  return localStorage.getItem(itemKey) ?? undefined
}

const clearLocal = (itemKey: string): void => {
  localStorage.removeItem(itemKey)
}
